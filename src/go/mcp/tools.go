package mcp

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"mailagent/rules"
)

// getTools returns the list of available MCP tools with their schemas.
func (s *Server) getTools() []Tool {
	rulesDesc := s.buildRulesDescription()

	return []Tool{
		{
			Name:        "list_emails",
			Description: "List emails in a folder. Returns email subjects, senders, and UIDs. " + rulesDesc,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder": map[string]interface{}{
						"type":        "string",
						"description": "Folder name (e.g., 'INBOX', 'INBOX.Simon')",
						"default":     "INBOX",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of emails to return",
						"default":     10,
					},
				},
			},
		},
		{
			Name:        "read_email",
			Description: "Read the full content of an email including body. " + rulesDesc,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder": map[string]interface{}{
						"type":        "string",
						"description": "Folder name containing the email",
					},
					"uid": map[string]interface{}{
						"type":        "integer",
						"description": "Email UID from list_emails",
					},
				},
				"required": []string{"folder", "uid"},
			},
		},
		{
			Name:        "archive_email",
			Description: "Archive an email by moving it to the Archive folder. " + rulesDesc,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder": map[string]interface{}{
						"type":        "string",
						"description": "Folder name containing the email",
					},
					"uid": map[string]interface{}{
						"type":        "integer",
						"description": "Email UID to archive",
					},
				},
				"required": []string{"folder", "uid"},
			},
		},
		{
			Name:        "reply_to_email",
			Description: "Send a reply to an email. The original sender will receive your reply. " + rulesDesc,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder": map[string]interface{}{
						"type":        "string",
						"description": "Folder name containing the email to reply to",
					},
					"uid": map[string]interface{}{
						"type":        "integer",
						"description": "Email UID to reply to",
					},
					"message": map[string]interface{}{
						"type":        "string",
						"description": "Your reply message text",
					},
				},
				"required": []string{"folder", "uid", "message"},
			},
		},
		{
			Name:        "forward_email",
			Description: "Forward an email to another email address. " + rulesDesc,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder": map[string]interface{}{
						"type":        "string",
						"description": "Folder name containing the email to forward",
					},
					"uid": map[string]interface{}{
						"type":        "integer",
						"description": "Email UID to forward",
					},
					"to": map[string]interface{}{
						"type":        "string",
						"description": "Email address to forward to",
					},
					"note": map[string]interface{}{
						"type":        "string",
						"description": "Optional note to add before the forwarded content",
					},
				},
				"required": []string{"folder", "uid", "to"},
			},
		},
		{
			Name:        "move_email",
			Description: "Move an email to a different folder. " + rulesDesc,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder": map[string]interface{}{
						"type":        "string",
						"description": "Current folder containing the email",
					},
					"uid": map[string]interface{}{
						"type":        "integer",
						"description": "Email UID to move",
					},
					"destination": map[string]interface{}{
						"type":        "string",
						"description": "Destination folder name",
					},
				},
				"required": []string{"folder", "uid", "destination"},
			},
		},
		{
			Name:        "process_emails_with_ai",
			Description: "Process emails using AI to decide actions. Reads emails, sends them to AI with rules, and executes the recommended actions. Returns a summary of what was done. " + rulesDesc,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"folder": map[string]interface{}{
						"type":        "string",
						"description": "Folder to process emails from",
						"default":     "INBOX",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of emails to process",
						"default":     5,
					},
				},
			},
		},
	}
}

// buildRulesDescription generates a description of loaded rules for tool documentation.
func (s *Server) buildRulesDescription() string {
	if s.ruleSet == nil {
		return "No rules loaded."
	}

	var sb strings.Builder
	sb.WriteString("RULES: ")

	for i, rule := range s.ruleSet.Rules {
		sb.WriteString(fmt.Sprintf("Rule %d (%s): ", i+1, rule.Name))
		if rule.Condition.Raw != "" {
			sb.WriteString(fmt.Sprintf("When %s, ", rule.Condition.Raw))
		}
		for j, action := range rule.Actions {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(action.Type)
		}
		sb.WriteString(". ")
	}

	return strings.TrimSpace(sb.String())
}

// --- Tool Implementations ---

func (s *Server) toolListEmails(args map[string]interface{}) (ContentBlock, error) {
	if err := s.ensureConnection(); err != nil {
		return ContentBlock{}, err
	}

	folder := getStringArg(args, "folder", "INBOX")
	limit := getIntArg(args, "limit", 10)

	emails, err := s.imapClient.ListEmails(folder, uint32(limit))
	if err != nil {
		return ContentBlock{}, fmt.Errorf("failed to list emails: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d emails in %s:\n\n", len(emails), folder))
	for _, email := range emails {
		sb.WriteString(fmt.Sprintf("UID: %d | From: %s | Subject: %s | Date: %s",
			email.UID, email.From, email.Subject, email.Date.Format("2006-01-02 15:04")))
		if email.HasAttachment {
			sb.WriteString(" [Has attachment]")
		}
		sb.WriteString("\n")
	}

	return ContentBlock{Type: "text", Text: sb.String()}, nil
}

func (s *Server) toolReadEmail(args map[string]interface{}) (ContentBlock, error) {
	if err := s.ensureConnection(); err != nil {
		return ContentBlock{}, err
	}

	folder := getStringArg(args, "folder", "")
	uid := getIntArg(args, "uid", 0)

	if folder == "" || uid == 0 {
		return ContentBlock{}, fmt.Errorf("folder and uid are required")
	}

	email, err := s.imapClient.ReadEmail(folder, uint32(uid))
	if err != nil {
		return ContentBlock{}, fmt.Errorf("failed to read email: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\nDate: %s\nUID: %d\n",
		email.From, email.To, email.Subject, email.Date.Format("2006-01-02 15:04"), email.UID))
	if email.HasAttachment {
		sb.WriteString("Has attachment: Yes\n")
	}
	sb.WriteString("\n--- Body ---\n\n")
	sb.WriteString(email.Body)

	return ContentBlock{Type: "text", Text: sb.String()}, nil
}

func (s *Server) toolArchiveEmail(args map[string]interface{}) (ContentBlock, error) {
	if err := s.ensureConnection(); err != nil {
		return ContentBlock{}, err
	}

	folder := getStringArg(args, "folder", "")
	uid := getIntArg(args, "uid", 0)

	if folder == "" || uid == 0 {
		return ContentBlock{}, fmt.Errorf("folder and uid are required")
	}

	if err := s.imapClient.ArchiveEmail(folder, uint32(uid)); err != nil {
		return ContentBlock{}, fmt.Errorf("failed to archive email: %w", err)
	}

	return ContentBlock{Type: "text", Text: fmt.Sprintf("Email UID %d archived successfully", uid)}, nil
}

func (s *Server) toolReplyToEmail(args map[string]interface{}) (ContentBlock, error) {
	if err := s.ensureConnection(); err != nil {
		return ContentBlock{}, err
	}

	folder := getStringArg(args, "folder", "")
	uid := getIntArg(args, "uid", 0)
	message := getStringArg(args, "message", "")

	if folder == "" || uid == 0 || message == "" {
		return ContentBlock{}, fmt.Errorf("folder, uid, and message are required")
	}

	email, err := s.imapClient.ReadEmail(folder, uint32(uid))
	if err != nil {
		return ContentBlock{}, fmt.Errorf("failed to read email: %w", err)
	}

	to := rules.ExtractEmailAddress(email.From)
	if to == "" {
		return ContentBlock{}, fmt.Errorf("could not extract recipient from email")
	}

	if err := s.sendEmail(to, "Re: "+email.Subject, message); err != nil {
		return ContentBlock{}, fmt.Errorf("failed to send reply: %w", err)
	}

	return ContentBlock{Type: "text", Text: fmt.Sprintf("Reply sent to %s", to)}, nil
}

func (s *Server) toolForwardEmail(args map[string]interface{}) (ContentBlock, error) {
	if err := s.ensureConnection(); err != nil {
		return ContentBlock{}, err
	}

	folder := getStringArg(args, "folder", "")
	uid := getIntArg(args, "uid", 0)
	to := getStringArg(args, "to", "")
	note := getStringArg(args, "note", "Forwarded email")

	if folder == "" || uid == 0 || to == "" {
		return ContentBlock{}, fmt.Errorf("folder, uid, and to are required")
	}

	email, err := s.imapClient.ReadEmail(folder, uint32(uid))
	if err != nil {
		return ContentBlock{}, fmt.Errorf("failed to read email: %w", err)
	}

	var body strings.Builder
	body.WriteString(note + "\n\n")
	body.WriteString("---\n")
	body.WriteString(fmt.Sprintf("From: %s\nDate: %s\nSubject: %s\n---\n\n",
		email.From, email.Date.Format("2006-01-02 15:04"), email.Subject))
	body.WriteString(email.Body)

	if err := s.sendEmail(to, "Fwd: "+email.Subject, body.String()); err != nil {
		return ContentBlock{}, fmt.Errorf("failed to forward email: %w", err)
	}

	return ContentBlock{Type: "text", Text: fmt.Sprintf("Email forwarded to %s", to)}, nil
}

func (s *Server) toolMoveEmail(args map[string]interface{}) (ContentBlock, error) {
	if err := s.ensureConnection(); err != nil {
		return ContentBlock{}, err
	}

	folder := getStringArg(args, "folder", "")
	uid := getIntArg(args, "uid", 0)
	destination := getStringArg(args, "destination", "")

	if folder == "" || uid == 0 || destination == "" {
		return ContentBlock{}, fmt.Errorf("folder, uid, and destination are required")
	}

	if err := s.imapClient.MoveEmail(folder, destination, uint32(uid)); err != nil {
		return ContentBlock{}, fmt.Errorf("failed to move email: %w", err)
	}

	return ContentBlock{Type: "text", Text: fmt.Sprintf("Email UID %d moved to %s", uid, destination)}, nil
}

func (s *Server) toolProcessEmailsWithAI(args map[string]interface{}) (ContentBlock, error) {
	if err := s.ensureConnection(); err != nil {
		return ContentBlock{}, err
	}

	folder := getStringArg(args, "folder", "INBOX")
	limit := getIntArg(args, "limit", 5)

	emails, err := s.imapClient.ListEmails(folder, uint32(limit))
	if err != nil {
		return ContentBlock{}, fmt.Errorf("failed to list emails: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Processing %d emails in %s...\n\n", len(emails), folder))

	for _, emailSummary := range emails {
		email, err := s.imapClient.ReadEmail(folder, emailSummary.UID)
		if err != nil {
			sb.WriteString(fmt.Sprintf("UID %d: Error reading - %v\n", emailSummary.UID, err))
			continue
		}

		prompt := s.ruleSet.BuildPromptForAI(email)

		aiResp, err := s.aiProvider.Process(context.Background(), prompt)
		if err != nil {
			sb.WriteString(fmt.Sprintf("UID %d (%s): AI error - %v\n", email.UID, email.Subject, err))
			continue
		}

		ruleResp := &rules.Response{
			Action:     aiResp.Action,
			Motivation: aiResp.Motivation,
			Confidence: aiResp.Confidence,
			ActionArgs: aiResp.ActionArgs,
		}

		if err := s.executor.Execute(email, ruleResp); err != nil {
			sb.WriteString(fmt.Sprintf("UID %d (%s): Action '%s' failed - %v\n",
				email.UID, email.Subject, aiResp.Action, err))
			continue
		}

		sb.WriteString(fmt.Sprintf("UID %d (%s): %s (%.0f%%) - %s\n",
			email.UID, email.Subject, aiResp.Action, aiResp.Confidence*100, aiResp.Motivation))
	}

	return ContentBlock{Type: "text", Text: sb.String()}, nil
}

// sendEmail sends an email via SMTP.
func (s *Server) sendEmail(to, subject, body string) error {
	if s.account == nil {
		return fmt.Errorf("not connected")
	}

	smtpServer := s.account.GetSMTPServer()
	smtpPort := s.account.GetSMTPPort()
	from := s.account.Username
	password, _ := s.account.GetPassword()

	addr := fmt.Sprintf("%s:%d", smtpServer, smtpPort)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	auth := smtp.PlainAuth("", from, password, smtpServer)
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

// Helper functions

func getStringArg(args map[string]interface{}, key, defaultVal string) string {
	if val, ok := args[key].(string); ok {
		return val
	}
	return defaultVal
}

func getIntArg(args map[string]interface{}, key string, defaultVal int) int {
	if val, ok := args[key].(float64); ok {
		return int(val)
	}
	if val, ok := args[key].(int); ok {
		return val
	}
	return defaultVal
}
