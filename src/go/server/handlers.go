package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"mailagent/actions"
	"mailagent/ai"
	"mailagent/config"
	"mailagent/imap"
	"mailagent/rules"

	"github.com/go-chi/chi/v5"
)

// --- Request/Response DTOs ---

type AccountInfo struct {
	Name      string `json:"name"`
	Username  string `json:"username"`
	Server    string `json:"server"`
	Folder    string `json:"folder"`
	UseOAuth2 bool   `json:"use_oauth2"`
	Connected bool   `json:"connected"`
}

type EmailInfo struct {
	UID           uint32 `json:"uid"`
	Subject       string `json:"subject"`
	From          string `json:"from"`
	To            string `json:"to"`
	Date          string `json:"date"`
	HasAttachment bool   `json:"has_attachment"`
	Folder        string `json:"folder"`
}

type ProcessRequest struct {
	Folder string `json:"folder"`
	Limit  uint   `json:"limit"`
}

type ProcessResult struct {
	UID        uint32 `json:"uid"`
	Subject    string `json:"subject"`
	Action     string `json:"action,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Motivation string `json:"motivation,omitempty"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

type ActionRequest struct {
	Action     string            `json:"action"`
	Motivation string            `json:"motivation,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
}

type MoveRequest struct {
	Destination string `json:"destination"`
}

// emailToInfo converts an EmailContext to EmailInfo DTO
func emailToInfo(email *rules.EmailContext, folder string) EmailInfo {
	return EmailInfo{
		UID:           email.UID,
		Subject:       email.Subject,
		From:          email.From,
		To:            email.To,
		Date:          email.Date.Format(time.RFC3339),
		HasAttachment: email.HasAttachment,
		Folder:        folder,
	}
}

// --- Health Check ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":      "healthy",
		"accounts":    len(s.cfg.Mail),
		"connections": len(s.connections),
	}

	// Check if we have any active connections
	hasConnections := false
	for name := range s.connections {
		hasConnections = true
		_ = name
		break
	}

	if !hasConnections && len(s.cfg.Mail) > 0 {
		status["status"] = "ready"
	}

	jsonResponse(w, http.StatusOK, status)
}

// --- Accounts ---

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := make([]AccountInfo, 0, len(s.cfg.Mail))
	for _, acc := range s.cfg.Mail {
		accounts = append(accounts, AccountInfo{
			Name:       acc.Name,
			Username:   acc.Username,
			Server:     acc.Server,
			Folder:     acc.Folder,
			UseOAuth2:  acc.UseOAuth2,
			Connected:  false,
		})
	}

	// Mark connected accounts
	for name := range s.connections {
		for i := range accounts {
			if accounts[i].Name == name {
				accounts[i].Connected = true
			}
		}
	}

	jsonResponse(w, http.StatusOK, accounts)
}

// --- Folders ---

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	acc := r.Context().Value("account").(*config.MailAccount)

	client, err := s.getConnection(acc)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	folders, err := client.ListFolders()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list folders: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"account": acc.Name,
		"folders": folders,
	})
}

// --- Folder Stats ---

func (s *Server) handleCountEmails(w http.ResponseWriter, r *http.Request) {
	acc := r.Context().Value("account").(*config.MailAccount)
	folder := chi.URLParam(r, "folder")

	client, err := s.getConnection(acc)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	total, unread, err := client.CountEmails(folder)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to count emails: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"account": acc.Name,
		"folder":  folder,
		"total":   total,
		"unread":  unread,
		"read":    total - unread,
	})
}

// --- Emails ---

func (s *Server) handleListEmails(w http.ResponseWriter, r *http.Request) {
	acc := r.Context().Value("account").(*config.MailAccount)
	folder := chi.URLParam(r, "folder")

	limitStr := r.URL.Query().Get("limit")
	limit := uint32(20)
	if limitStr != "" {
		if l, err := strconv.ParseUint(limitStr, 10, 32); err == nil {
			limit = uint32(l)
		}
	}

	client, err := s.getConnection(acc)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	emails, err := client.ListEmails(folder, limit)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list emails: %v", err))
		return
	}

	// Convert to API response format
	emailList := make([]EmailInfo, 0, len(emails))
	for _, e := range emails {
		emailList = append(emailList, emailToInfo(e, folder))
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"account": acc.Name,
		"folder":  folder,
		"count":   len(emailList),
		"emails":  emailList,
	})
}

func (s *Server) handleGetEmail(w http.ResponseWriter, r *http.Request) {
	acc := r.Context().Value("account").(*config.MailAccount)
	folder := chi.URLParam(r, "folder")

	uidStr := chi.URLParam(r, "uid")
	uid, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid UID")
		return
	}

	client, err := s.getConnection(acc)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	email, err := client.ReadEmail(folder, uint32(uid))
	if err != nil {
		errorResponse(w, http.StatusNotFound, fmt.Sprintf("Email not found: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"account": acc.Name,
		"email":   emailToInfo(email, email.Folder),
	})
}

// --- Process Emails ---

func (s *Server) handleProcessEmails(w http.ResponseWriter, r *http.Request) {
	acc := r.Context().Value("account").(*config.MailAccount)
	req, err := s.parseProcessRequest(r, acc)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	client, err := s.getConnection(acc)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	executor := s.executors[acc.Name]
	emails, err := client.ListEmails(req.Folder, uint32(req.Limit))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list emails: %v", err))
		return
	}

	ctx := context.Background()
	results := s.processEmailBatch(ctx, client, executor, req.Folder, emails)

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"account":   acc.Name,
		"folder":    req.Folder,
		"processed": len(results),
		"results":   results,
	})
}

func (s *Server) parseProcessRequest(r *http.Request, acc *config.MailAccount) (ProcessRequest, error) {
	var req ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = ProcessRequest{Folder: acc.Folder, Limit: 10}
	}

	if req.Folder == "" {
		req.Folder = acc.Folder
	}
	if req.Limit == 0 {
		req.Limit = 10
	}
	return req, nil
}

func (s *Server) processEmailBatch(ctx context.Context, client *imap.Client, executor *actions.Executor, folder string, emails []*rules.EmailContext) []ProcessResult {
	results := make([]ProcessResult, 0, len(emails))

	for _, email := range emails {
		result := s.processSingleEmail(ctx, client, executor, folder, email)
		results = append(results, result)
	}

	return results
}

func (s *Server) processSingleEmail(ctx context.Context, client *imap.Client, executor *actions.Executor, folder string, email *rules.EmailContext) ProcessResult {
	// Read full email
	fullEmail, err := client.ReadEmail(folder, email.UID)
	if err != nil {
		return ProcessResult{UID: email.UID, Subject: email.Subject, Error: fmt.Sprintf("Failed to read email: %v", err)}
	}

	// Get AI decision
	aiResp, err := s.getAIDecision(ctx, fullEmail)
	if err != nil {
		return ProcessResult{UID: email.UID, Subject: email.Subject, Error: fmt.Sprintf("AI failed: %v", err)}
	}

	// Execute action
	if err := s.executeEmailAction(executor, fullEmail, aiResp); err != nil {
		return s.actionResult(email, aiResp, fmt.Sprintf("Action failed: %v", err))
	}

	return s.successResult(email, aiResp)
}

func (s *Server) getAIDecision(ctx context.Context, email *rules.EmailContext) (*ai.Response, error) {
	prompt := s.ruleSet.BuildPromptForAI(email)
	return s.aiProvider.Process(ctx, prompt)
}

func (s *Server) executeEmailAction(executor *actions.Executor, email *rules.EmailContext, aiResp *ai.Response) error {
	ruleResp := &rules.Response{
		Action:     aiResp.Action,
		Motivation: aiResp.Motivation,
		Confidence: aiResp.Confidence,
		ActionArgs: aiResp.ActionArgs,
	}
	return executor.Execute(email, ruleResp)
}

func (s *Server) actionResult(email *rules.EmailContext, aiResp *ai.Response, errMsg string) ProcessResult {
	return ProcessResult{
		UID:        email.UID,
		Subject:    email.Subject,
		Action:     aiResp.Action,
		Confidence: fmt.Sprintf("%.0f%%", aiResp.Confidence*100),
		Motivation: aiResp.Motivation,
		Error:      errMsg,
	}
}

func (s *Server) successResult(email *rules.EmailContext, aiResp *ai.Response) ProcessResult {
	return ProcessResult{
		UID:        email.UID,
		Subject:    email.Subject,
		Action:     aiResp.Action,
		Confidence: fmt.Sprintf("%.0f%%", aiResp.Confidence*100),
		Motivation: aiResp.Motivation,
		Success:    true,
	}
}

// --- Execute Action ---

func (s *Server) handleExecuteAction(w http.ResponseWriter, r *http.Request) {
	acc := r.Context().Value("account").(*config.MailAccount)
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = acc.Folder
	}

	uidStr := chi.URLParam(r, "uid")
	uid, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid UID")
		return
	}

	var req ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	client, err := s.getConnection(acc)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	executor := s.executors[acc.Name]

	// Read the email
	email, err := client.ReadEmail(folder, uint32(uid))
	if err != nil {
		errorResponse(w, http.StatusNotFound, fmt.Sprintf("Email not found: %v", err))
		return
	}

	// Convert string args to interface args
	args := make(map[string]interface{}, len(req.Args))
	for k, v := range req.Args {
		args[k] = v
	}

	// Execute the requested action
	ruleResp := &rules.Response{
		Action:     req.Action,
		Motivation: req.Motivation,
		ActionArgs: args,
	}

	if err := executor.Execute(email, ruleResp); err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Action failed: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"account": acc.Name,
		"uid":     uid,
		"folder":  folder,
		"action":  req.Action,
		"success": true,
	})
}

// --- Move Email ---

func (s *Server) handleMoveEmail(w http.ResponseWriter, r *http.Request) {
	acc := r.Context().Value("account").(*config.MailAccount)
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = acc.Folder
	}

	uidStr := chi.URLParam(r, "uid")
	uid, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid UID")
		return
	}

	var req MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Destination == "" {
		errorResponse(w, http.StatusBadRequest, "destination is required")
		return
	}

	client, err := s.getConnection(acc)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to connect: %v", err))
		return
	}

	if err := client.MoveEmail(folder, req.Destination, uint32(uid)); err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to move email: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"account":     acc.Name,
		"uid":         uid,
		"from_folder": folder,
		"to_folder":   req.Destination,
		"success":     true,
	})
}

// --- Rules ---

type RulesResponse struct {
	Content string `json:"content"`
}

func (s *Server) handleGetRules(w http.ResponseWriter, r *http.Request) {
	content, err := os.ReadFile("rules.md")
	if err != nil {
		// Try with config path
		content, err = os.ReadFile(s.cfg.Rules.File)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to read rules: %v", err))
			return
		}
	}

	jsonResponse(w, http.StatusOK, RulesResponse{
		Content: string(content),
	})
}

type UpdateRulesRequest struct {
	Content string `json:"content"`
}

func (s *Server) handleUpdateRules(w http.ResponseWriter, r *http.Request) {
	var req UpdateRulesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Content == "" {
		errorResponse(w, http.StatusBadRequest, "Content cannot be empty")
		return
	}

	// Determine file path
	rulesPath := "rules.md"
	if s.cfg.Rules.File != "" {
		rulesPath = s.cfg.Rules.File
	}

	// Write to file
	if err := os.WriteFile(rulesPath, []byte(req.Content), 0644); err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write rules: %v", err))
		return
	}

	// Reload rules in the rule set
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Rules saved but failed to reload: %v", err))
		return
	}

	ruleSet, err := rules.ParseMarkdown(string(content))
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Rules saved but failed to parse: %v\n\nContent:\n%s", err, string(content)[:min(len(content), 500)]))
		return
	}

	// Update the server's rule set
	s.ruleSet = ruleSet

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Rules updated and reloaded successfully. Parsed %d rules.", len(ruleSet.Rules)),
		"rulesCount": len(ruleSet.Rules),
		"systemPromptLength": len(ruleSet.SystemPrompt),
	})
}
