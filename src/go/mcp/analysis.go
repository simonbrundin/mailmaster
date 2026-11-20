package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mailagent/rules"
)

// suggestedAction is one action the AI proposes for an email.
type suggestedAction struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// analysisItem is the AI's proposal for one email. Nothing is executed until
// the user approves it in the UI.
type analysisItem struct {
	UID         uint32            `json:"uid"`
	Subject     string            `json:"subject"`
	From        string            `json:"from"`
	Folder      string            `json:"folder"`
	Matched     bool              `json:"matched"`
	Rule        string            `json:"rule"`
	Condition   string            `json:"condition"`
	Description string            `json:"description"`
	Motivation  string            `json:"motivation"`
	Confidence  string            `json:"confidence"`
	Actions     []suggestedAction `json:"actions"`
	Error       string            `json:"error,omitempty"`

	// execArgs holds the arguments needed to run the actions (reply text,
	// forward recipient, destination folder). Kept server side so the client
	// cannot change what an approved action does.
	execArgs map[string]interface{} `json:"-"`
}

// httpAnalyzeEmails lets the AI analyse the emails in a folder without
// executing anything. The result is stored so a later approval can be
// executed with exactly the proposed arguments.
func (s *Server) httpAnalyzeEmails(w http.ResponseWriter, r *http.Request, path string) {
	account := accountFromPath(path)

	var req struct {
		Folder string `json:"folder"`
		Limit  int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, 400, err.Error())
		return
	}

	session, err := s.ensureConnectionFor(account)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	if s.aiProvider == nil || s.ruleSet == nil {
		writeError(w, 500, "AI provider or rules not configured")
		return
	}

	folder := req.Folder
	if folder == "" {
		folder = s.defaultFolder(session.account)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	emails, err := s.listEmails(session, folder, uint32(limit))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	items := make([]analysisItem, 0, len(emails))
	stored := make(map[uint32]analysisItem, len(emails))
	for _, e := range emails {
		item := s.analyzeEmail(r, session, folder, e)
		items = append(items, item)
		stored[e.UID] = item
	}

	s.storeAnalyses(account, folder, stored)

	writeJSON(w, 200, map[string]interface{}{
		"folder":  folder,
		"results": items,
	})
}

// analyzeEmail runs the AI on one email and builds the proposal shown in the UI.
func (s *Server) analyzeEmail(r *http.Request, session *accountSession, folder string, e *rules.EmailContext) analysisItem {
	item := analysisItem{
		UID:     e.UID,
		Subject: e.Subject,
		From:    e.From,
		Folder:  folder,
		Actions: []suggestedAction{},
	}

	full, err := session.imapClient.ReadEmail(folder, e.UID)
	if err != nil {
		item.Error = fmt.Sprintf("kunde inte läsa mailet: %v", err)
		item.Description = "Mailet kunde inte läsas och kunde därför inte analyseras."
		return item
	}
	item.Subject = full.Subject
	item.From = full.From

	aiResp, err := s.aiProvider.Process(r.Context(), s.ruleSet.BuildPromptForAI(full))
	if err != nil {
		item.Error = fmt.Sprintf("AI-fel: %v", err)
		item.Description = "Analysen misslyckades - AI:t svarade inte."
		return item
	}

	item.Motivation = aiResp.Motivation
	item.Confidence = fmt.Sprintf("%.0f%%", aiResp.Confidence*100)
	item.execArgs = execArgs(aiResp.ActionArgs)

	// Which rule did the AI match?
	ruleName := strings.TrimSpace(aiResp.Rule)
	if strings.EqualFold(ruleName, "ingen") || strings.EqualFold(ruleName, "none") {
		ruleName = ""
	}
	if ruleName != "" {
		item.Rule = ruleName
		if rule := s.ruleSet.FindRule(ruleName); rule != nil {
			item.Rule = rule.Name
			item.Condition = rule.Condition.Raw
		}
	} else if rule := s.ruleSet.FindRuleInText(aiResp.Motivation); rule != nil {
		// The AI mentioned a rule in the motivation instead of the REGEL field.
		item.Rule = rule.Name
		item.Condition = rule.Condition.Raw
	}

	// Which actions does the AI propose?
	for _, action := range session.executor.ProposedActions(&rules.Response{
		Action:     aiResp.Action,
		Motivation: aiResp.Motivation,
		ActionArgs: aiResp.ActionArgs,
	}) {
		if action == "" || action == "uncertain" || action == "skip" {
			continue
		}
		item.Actions = append(item.Actions, suggestedAction{
			Type:        action,
			Description: describeAction(action, aiResp.ActionArgs),
		})
	}

	item.Matched = item.Rule != "" || len(item.Actions) > 0
	item.Description = describeMatch(item)

	return item
}

// describeMatch builds the human readable text that explains if the email
// matched a rule and why.
func describeMatch(item analysisItem) string {
	if item.Rule != "" {
		if item.Condition != "" {
			return fmt.Sprintf("Mailet matchar regeln %q. Villkor: %s", item.Rule, item.Condition)
		}
		return fmt.Sprintf("Mailet matchar regeln %q.", item.Rule)
	}
	if len(item.Actions) == 0 {
		return "Ingen regel matchade det här mailet - ingen åtgärd rekommenderas."
	}
	return "Ingen regel matchade tydligt, men AI:t föreslår en åtgärd baserat på mailets innehåll."
}

// describeAction describes a single proposed action for the UI.
func describeAction(action string, args map[string]interface{}) string {
	switch action {
	case "reply":
		if text := stringArg(args, "reply"); text != "" {
			return fmt.Sprintf("Svara: %s", text)
		}
		return "Svara på mailet"
	case "forward":
		if to := stringArg(args, "to"); to != "" {
			return fmt.Sprintf("Vidarebefordra till %s", to)
		}
		return "Vidarebefordra mailet (mottagare saknas)"
	case "move":
		if folder := stringArg(args, "folder"); folder != "" {
			return fmt.Sprintf("Flytta till %s", folder)
		}
		return "Flytta mailet till en annan mapp"
	case "archive":
		return "Arkivera mailet"
	case "read":
		return "Markera mailet som läst"
	default:
		return action
	}
}

// execArgs picks the arguments needed to execute the proposed actions.
func execArgs(args map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for _, key := range []string{"reply", "to", "folder"} {
		if val := stringArg(args, key); val != "" {
			out[key] = val
		}
	}
	return out
}

func stringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	if val, ok := args[key].(string); ok {
		return strings.TrimSpace(val)
	}
	return ""
}

// httpDecideEmail applies the user's decision for an analysed email.
// decision is either "approve" (run the proposed actions) or "deny".
func (s *Server) httpDecideEmail(w http.ResponseWriter, r *http.Request, path string) {
	account := accountFromPath(path)

	var req struct {
		Folder   string `json:"folder"`
		UID      uint32 `json:"uid"`
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, err.Error())
		return
	}

	folder := req.Folder
	if folder == "" {
		// The account config is enough to know the default folder, so a
		// denial does not need a working connection.
		accountCfg, err := s.accountByName(account)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		folder = s.defaultFolder(accountCfg)
	}

	switch strings.ToLower(req.Decision) {
	case "deny", "neka":
		writeJSON(w, 200, map[string]interface{}{
			"success": true,
			"message": "Åtgärden avböjdes - inget gjordes med mailet.",
		})
		return
	case "approve", "ok":
	default:
		writeError(w, 400, "decision must be 'approve' or 'deny'")
		return
	}

	session, err := s.ensureConnectionFor(account)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	item, ok := s.analysisFor(account, folder, req.UID)
	if !ok {
		writeError(w, 404, "analysen finns inte längre - kör 'Processa mejl' igen")
		return
	}
	if len(item.Actions) == 0 {
		writeJSON(w, 200, map[string]interface{}{
			"success": false,
			"message": "Det finns ingen åtgärd att godkänna för det här mailet.",
		})
		return
	}

	full, err := session.imapClient.ReadEmail(folder, req.UID)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	actions := make([]string, 0, len(item.Actions))
	for _, a := range item.Actions {
		actions = append(actions, a.Type)
	}

	resp := &rules.Response{
		Action:     actions[0],
		Motivation: item.Motivation,
		ActionArgs: item.execArgs,
	}

	if err := session.executor.ExecuteActions(full, resp, actions); err != nil {
		writeJSON(w, 200, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Kunde inte utföra åtgärden: %v", err),
		})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Åtgärden utfördes: %s", strings.Join(actions, ", ")),
	})
}

// accountFromPath extracts the account name from /api/v1/accounts/{name}/...
func accountFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "accounts" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
