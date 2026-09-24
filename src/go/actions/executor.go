package actions

import (
	"fmt"
	"strings"

	"mailagent/config"
	"mailagent/imap"
	"mailagent/rules"
)

// Executor handles AI decision execution
type Executor struct {
	imapClient *imap.Client
	cfg        *config.FallbackConfig
	smtpPass   string
}

// NewExecutor creates a new action executor
func NewExecutor(imapClient *imap.Client, cfg *config.FallbackConfig) *Executor {
	return &Executor{
		imapClient: imapClient,
		cfg:        cfg,
	}
}

// SetSMTPCredentials stores the SMTP password/token for outgoing mail
func (e *Executor) SetSMTPCredentials(pass string) {
	e.smtpPass = pass
}

// Execute runs all recommended actions for an email.
// Supports multiple actions (e.g., "reply AND archive") by scanning the
// motivation text in addition to the primary action.
func (e *Executor) Execute(email *rules.EmailContext, response *rules.Response) error {
	actions := e.collectActions(response)
	for _, action := range actions {
		if err := e.executeSingleAction(email, response, action); err != nil {
			return err
		}
	}
	return nil
}

// collectActions returns all actions found in the AI response, including
// implicit actions mentioned in the motivation text.
func (e *Executor) collectActions(response *rules.Response) []string {
	actions := []string{response.Action}
	motivation := strings.ToLower(response.Motivation)

	for action, present := range e.collectImplicitActions(motivation) {
		if present && !sliceContains(actions, action) {
			actions = append(actions, action)
		}
	}

	return actions
}

func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// collectImplicitActions returns a map of action types detected in the motivation text.
func (e *Executor) collectImplicitActions(motivation string) map[string]bool {
	return map[string]bool{
		"archive": strings.Contains(motivation, "arkiv"),
		"forward": strings.Contains(motivation, "vidarebefordr"),
		"move":    strings.Contains(motivation, "flytta"),
		"reply":   strings.Contains(motivation, "svara") || strings.Contains(motivation, "svar") || strings.Contains(motivation, "reply") || strings.Contains(motivation, "replay"),
	}
}

// executeSingleAction performs one named action on an email.
func (e *Executor) executeSingleAction(email *rules.EmailContext, response *rules.Response, action string) error {
	switch action {
	case "read":
		return e.markRead(email)

	case "archive":
		return e.archive(email)

	case "move":
		folder := getActionArg(response, "folder")
		if folder == "" {
			folder = e.defaultArchiveFolder()
		}
		return e.move(email, folder)

	case "forward":
		to := getActionArg(response, "to")
		if to == "" {
			to = e.cfg.NotifyEmail
		}
		return e.forward(email, to, getActionArg(response, "reply"))

	case "reply":
		return e.reply(email, response)

	case "uncertain", "":
		return e.handleUncertain(email, response)

	default:
		return e.handleUncertain(email, response)
	}
}

// --- Read ---

func (e *Executor) markRead(email *rules.EmailContext) error {
	if email.UID == 0 || email.Folder == "" {
		return nil
	}
	return e.imapClient.MarkAsRead(email.Folder, email.UID)
}

// --- Archive ---

func (e *Executor) archive(email *rules.EmailContext) error {
	if email.UID == 0 || email.Folder == "" {
		return fmt.Errorf("no UID available for archive action")
	}
	return e.imapClient.ArchiveEmail(email.Folder, email.UID)
}

// --- Move ---

func (e *Executor) move(email *rules.EmailContext, folder string) error {
	if email.UID == 0 || email.Folder == "" {
		return fmt.Errorf("no UID available for move action")
	}
	return e.imapClient.MoveEmail(email.Folder, folder, email.UID)
}

func (e *Executor) defaultArchiveFolder() string {
	if acc := e.imapClient.GetAccount(); acc != nil && acc.Server == "imap.one.com" {
		return "INBOX.Archive"
	}
	return "Archive"
}

// --- Reply ---

func (e *Executor) reply(email *rules.EmailContext, response *rules.Response) error {
	replyText := e.extractReplyText(response)
	if replyText == "" {
		return fmt.Errorf("no reply text found in AI response")
	}

	to := rules.ExtractEmailAddress(email.From)
	if to == "" {
		return fmt.Errorf("could not extract recipient from email")
	}

	return e.sendEmail(to, "Re: "+email.Subject, replyText)
}

func (e *Executor) extractReplyText(response *rules.Response) string {
	// 1. Explicit action arg
	if text := getActionArg(response, "reply"); text != "" {
		return text
	}

	// 2. "Svara:" / "Reply:" prefix in motivation
	motivation := response.Motivation
	for _, prefix := range []string{"Svara:", "Reply:", "SVARA:", "REPLY:"} {
		if idx := strings.Index(strings.ToLower(motivation), strings.ToLower(prefix)); idx >= 0 {
			return strings.TrimSpace(motivation[idx+len(prefix):])
		}
	}

	// 3. Quoted text in motivation (most reliable)
	if idx := strings.Index(motivation, "\""); idx >= 0 {
		if end := strings.Index(motivation[idx+1:], "\""); end > 0 {
			return motivation[idx+1 : idx+1+end]
		}
	}

	// 4. If motivation contains action verbs after the reply, strip them
	// e.g. "Tack för ditt mail!" och arkivera mailet
	stripPatterns := []string{
		" och arkivera",
		" och flytta",
		" och vidarebefordra",
		" och move",
		" och archive",
		" och forward",
	}
	for _, pattern := range stripPatterns {
		if idx := strings.Index(motivation, pattern); idx > 0 {
			return strings.TrimSpace(motivation[:idx])
		}
	}

	return motivation
}

// --- Forward ---

func (e *Executor) forward(email *rules.EmailContext, to, note string) error {
	if note == "" {
		note = "Vidarebefordrat mail"
	}

	var body strings.Builder
	body.WriteString("─────────────────────────────────────\n")
	body.WriteString(fmt.Sprintf("Vidarebefordrat från: %s\n", email.From))
	body.WriteString(fmt.Sprintf("Datum: %s\n", email.Date.Format("2006-01-02 15:04")))
	body.WriteString(fmt.Sprintf("Ämne: %s\n", email.Subject))
	body.WriteString("─────────────────────────────────────\n\n")
	body.WriteString(email.Body)

	msg := fmt.Sprintf("%s\n\n%s", note, body.String())
	return e.sendEmail(to, "Fwd: "+email.Subject, msg)
}

// --- Uncertain / Fallback ---

func (e *Executor) handleUncertain(email *rules.EmailContext, response *rules.Response) error {
	if !e.cfg.Enabled || e.cfg.NotifyEmail == "" {
		return nil // Silently skip when fallback is disabled
	}

	body := e.buildFallbackEmail(email, &AIResponse{
		Action:     response.Action,
		Motivation: response.Motivation,
		Confidence: response.Confidence,
	})

	return e.sendEmail(e.cfg.NotifyEmail, "[MailAgent] Osäker på hur mail ska hanteras", body)
}

func (e *Executor) buildFallbackEmail(email *rules.EmailContext, aiResp *AIResponse) string {
	var sb strings.Builder

	sb.WriteString("Hej,\n\n")
	sb.WriteString("MailAgent kunde inte avgöra hur följande mail ska hanteras:\n\n")
	sb.WriteString("─────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("Från: %s\n", email.From))
	sb.WriteString(fmt.Sprintf("Till: %s\n", email.To))
	sb.WriteString(fmt.Sprintf("Ämne: %s\n", email.Subject))
	sb.WriteString(fmt.Sprintf("Datum: %s\n", email.Date.Format("2006-01-02 15:04")))
	sb.WriteString("─────────────────────────────────────\n\n")

	if email.Body != "" {
		sb.WriteString("Innehåll:\n")
		sb.WriteString(email.Body)
		sb.WriteString("\n\n")
	}

	sb.WriteString("─────────────────────────────────────\n")
	sb.WriteString("AI-analys:\n")
	sb.WriteString(fmt.Sprintf("Föreslagen åtgärd: %s\n", aiResp.Action))
	sb.WriteString(fmt.Sprintf("Motivering: %s\n", aiResp.Motivation))
	sb.WriteString(fmt.Sprintf("Konfidens: %.0f%%\n", aiResp.Confidence*100))
	sb.WriteString("─────────────────────────────────────\n\n")
	sb.WriteString("Vänligen hantera detta mail manuellt eller uppdatera reglerna.\n\n")
	sb.WriteString("Med vänliga hälsningar,\n")
	sb.WriteString("MailAgent\n")

	return sb.String()
}

// --- SMTP ---

func (e *Executor) smtpConfig() smtpConfig {
	acc := e.imapClient.GetAccount()
	if acc == nil {
		return smtpConfig{}
	}
	return smtpConfig{
		server:    acc.GetSMTPServer(),
		port:      acc.GetSMTPPort(),
		username:  acc.Username,
		password:  e.smtpPass,
		fromEmail: acc.Username,
		useOAuth2: acc.UseOAuth2,
	}
}

func (e *Executor) sendEmail(to, subject, body string) error {
	cfg := e.smtpConfig()
	if cfg.server == "" || cfg.fromEmail == "" {
		return nil // SMTP not configured — skip silently
	}
	msg := formatMessage(cfg.fromEmail, to, subject, body)
	return newSMTPClient(cfg).send(to, []byte(msg))
}

// --- Helpers ---

func getActionArg(response *rules.Response, key string) string {
	if response.ActionArgs == nil {
		return ""
	}
	if val, ok := response.ActionArgs[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// AIResponse is used for uncertain/error handling from AI
type AIResponse struct {
	Action     string
	Motivation string
	Confidence float64
}

// HandleUncertain is kept for backward compatibility — delegates to handleUncertain
func (e *Executor) HandleUncertain(email *rules.EmailContext, aiResp *AIResponse) error {
	return e.handleUncertain(email, &rules.Response{
		Action:     aiResp.Action,
		Motivation: aiResp.Motivation,
		Confidence: aiResp.Confidence,
	})
}
