package ai

import (
	"regexp"
	"strings"
)

// confidenceForAction returns a confidence score based on the action type.
func confidenceForAction(action string) float64 {
	switch action {
	case "read", "archive":
		return 0.9
	case "move", "forward", "reply":
		return 0.85
	case "uncertain":
		return 0.3
	default:
		return 0.7
	}
}

// normalizeAction maps various Swedish/English action strings to canonical action names.
func normalizeAction(action string) string {
	action = strings.TrimSpace(action)
	switch strings.ToLower(action) {
	case "läs", "läsa", "read":
		return "read"
	case "arkivera", "archive":
		return "archive"
	case "flytta", "move":
		return "move"
	case "vidarebefordra", "forward":
		return "forward"
	case "svara", "reply":
		return "reply"
	case "osäker", "uncertain", "unknown":
		return "uncertain"
	case "ignorera", "ignore", "skip":
		return "skip"
	default:
		return "uncertain"
	}
}

// parseActions splits an action field such as "reply, archive" into a list of
// canonical actions. The order in the field is preserved.
func parseActions(action string) []string {
	parts := strings.FieldsFunc(action, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})

	actions := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := normalizeAction(part)
		if normalized == "" {
			continue
		}
		actions = append(actions, normalized)
	}
	return actions
}

// stripQuotes removes a single pair of surrounding double quotes.
func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

// ParseResponse parses raw AI output into a Response. Exported so that tests
// and tooling use the same parsing as the provider.
func ParseResponse(content string) *Response {
	return parseResponse(content)
}

// parseResponse parses AI response text into a Response struct.
// First tries structured format (ÅTGÄRD:/MOTIVERING:), then falls back to free-text.
func parseResponse(content string) *Response {
	if structured := parseStructuredFormat(content); structured != nil {
		if structured.Rule == "" {
			structured.Rule = ParseRule(content)
		}
		return structured
	}
	resp := parseFreeTextFallback(content)
	resp.Rule = ParseRule(content)
	return resp
}

// parseStructuredFormat parses key:value format like "ÅTGÄRD: read".
func parseStructuredFormat(content string) *Response {
	resp := &Response{
		ActionArgs: make(map[string]interface{}),
	}
	changed := false

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "*")
		if line == "" {
			continue
		}

		// Action line
		if strings.HasPrefix(line, "ÅTGÄRD:") || strings.HasPrefix(line, "ACTION:") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "ÅTGÄRD:"))
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "ACTION:"))
			actions := parseActions(raw)
			if len(actions) > 0 {
				resp.Action = actions[0]
				if len(actions) > 1 {
					resp.ActionArgs["actions"] = actions
				}
				resp.Confidence = confidenceForAction(resp.Action)
				changed = true
			}
		}

		// Rule line - the name of the rule the mail matched
		if strings.HasPrefix(line, "REGEL:") || strings.HasPrefix(line, "RULE:") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "REGEL:"))
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "RULE:"))
			resp.Rule = strings.Trim(raw, "\"")
		}

		// Motivation line
		if strings.HasPrefix(line, "MOTIVERING:") || strings.HasPrefix(line, "MOTIVATION:") {
			resp.Motivation = strings.TrimSpace(strings.TrimPrefix(line, "MOTIVERING:"))
			resp.Motivation = strings.TrimSpace(strings.TrimPrefix(resp.Motivation, "MOTIVATION:"))
		}

		// Action args
		for _, prefix := range []struct{ key, name string }{
			{"MAPP:", "folder"},
			{"FOLDER:", "folder"},
			{"TILL:", "to"},
			{"TO:", "to"},
			{"SVAR:", "reply"},
			{"REPLY:", "reply"},
		} {
			if strings.HasPrefix(line, prefix.key) {
				value := strings.TrimSpace(strings.TrimPrefix(line, prefix.key))
				if prefix.name == "reply" {
					value = stripQuotes(value)
				}
				resp.ActionArgs[prefix.name] = value
			}
		}
	}

	// If the action field was not understood, fall back on what the structured
	// fields or the motivation tell us — but never throw away a reply text or
	// a recipient that the AI already gave us.
	if resp.Action == "uncertain" {
		if reply, _ := resp.ActionArgs["reply"].(string); strings.TrimSpace(reply) != "" {
			resp.Action = "reply"
			resp.Confidence = 0.85
		} else if to, _ := resp.ActionArgs["to"].(string); strings.TrimSpace(to) != "" {
			resp.Action = "forward"
			resp.Confidence = 0.85
		} else if parsed := parseMotivationForAction(resp.Motivation); parsed != nil {
			return parsed
		}
	}

	if !changed || resp.Action == "" {
		return nil
	}
	if resp.Motivation == "" {
		resp.Motivation = content
	}
	return resp
}

// parseMotivationForAction checks the motivation text for actionable content.
func parseMotivationForAction(motivation string) *Response {
	if motivation == "" {
		return nil
	}

	lower := strings.ToLower(motivation)

	// Check for reply
	if strings.Contains(lower, "svara") || strings.Contains(lower, "reply") {
		replyText := extractReplyFromContent(motivation)
		return &Response{
			Action:     "reply",
			Motivation: motivation,
			Confidence: 0.85,
			ActionArgs: map[string]interface{}{"reply": replyText},
		}
	}

	// Check for forward - MUST have email address
	if strings.Contains(lower, "vidarebefordr") || strings.Contains(lower, "forward") {
		// Try to extract email address from motivation
		emailRegex := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
		matches := emailRegex.FindAllString(motivation, -1)
		
		if len(matches) == 0 {
			// No email found - return uncertain so user knows action couldn't be completed
			return &Response{
				Action:     "uncertain",
				Motivation: motivation + "\n[FEL: Forward kräver en email-adress med TILL: exempel@domän.se]",
				Confidence: 0.3,
			}
		}
		
		return &Response{
			Action:     "forward",
			Motivation: motivation,
			Confidence: 0.85,
			ActionArgs: map[string]interface{}{"to": matches[0]},
		}
	}

	// Check for archive
	if strings.Contains(lower, "arkiv") || strings.Contains(lower, "archiv") {
		return &Response{
			Action:     "archive",
			Motivation: motivation,
			Confidence: 0.9,
		}
	}

	// Check for forward
	if strings.Contains(lower, "vidarebefordr") || strings.Contains(lower, "forward") {
		return &Response{
			Action:     "forward",
			Motivation: motivation,
			Confidence: 0.85,
		}
	}

	// Check for move
	if strings.Contains(lower, "flytta") || strings.Contains(lower, "move") {
		return &Response{
			Action:     "move",
			Motivation: motivation,
			Confidence: 0.85,
		}
	}

	return nil
}

// parseFreeTextFallback tries to extract action from free-form text.
func parseFreeTextFallback(content string) *Response {
	resp := &Response{
		ActionArgs: make(map[string]interface{}),
		Confidence: 0.7,
	}

	lower := strings.ToLower(content)

	// Check for reply FIRST (before archive since "svara" is most common)
	if strings.Contains(lower, "svara") || strings.Contains(lower, "reply") {
		resp.Action = "reply"
		// Try to extract the actual reply text from the content
		if replyText := extractReplyFromContent(content); replyText != "" {
			resp.ActionArgs["reply"] = replyText
			resp.Motivation = replyText
		} else {
			resp.Motivation = "AI föreslog att svara på mailet"
		}
		return resp
	}

	// Try to find archive
	if strings.Contains(lower, "arkiv") || strings.Contains(lower, "archiv") {
		resp.Action = "archive"
		resp.Motivation = "AI föreslog att arkivera mailet"
		return resp
	}

	// Try to find forward
	if strings.Contains(lower, "vidarebefordr") || strings.Contains(lower, "forward") {
		resp.Action = "forward"
		resp.Motivation = "AI föreslog att vidarebefordra mailet"
		return resp
	}

	// Try to find move
	if strings.Contains(lower, "flytta") || strings.Contains(lower, "move") {
		resp.Action = "move"
		resp.Motivation = "AI föreslog att flytta mailet"
		return resp
	}

	// Try to find read
	if strings.Contains(lower, "läsa") || strings.Contains(lower, "read") {
		resp.Action = "read"
		resp.Motivation = "AI föreslog att läsa mailet"
		return resp
	}

	// Default to uncertain
	resp.Action = "uncertain"
	resp.Motivation = content
	resp.Confidence = 0.3
	return resp
}

// ParseRule extracts the rule name from a raw AI answer. Used as a fallback
// when the structured REGEL: field is missing.
func ParseRule(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "*"))
		if strings.HasPrefix(line, "REGEL:") || strings.HasPrefix(line, "RULE:") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "REGEL:"))
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "RULE:"))
			return strings.Trim(raw, "\"")
		}
	}
	return ""
}

// extractReplyFromContent extracts the reply text from content that may contain "Svara:" or similar.
// An explicit "Svara:"/"Reply:" line wins and is returned verbatim (only quotes
// and trailing whitespace are removed). Otherwise a quoted sentence is used.
func extractReplyFromContent(content string) string {
	// Look for patterns like "Svara: text" or "Reply: text"
	patterns := []string{"svara:", "reply:", "svar:", "svara :", "reply :", "svar :"}
	for _, pattern := range patterns {
		if idx := strings.Index(strings.ToLower(content), pattern); idx >= 0 {
			// Get text after the pattern
			replyText := strings.TrimSpace(content[idx+len(pattern):])
			// Stop at first newline
			replyText = strings.Split(replyText, "\n")[0]
			replyText = strings.TrimSpace(replyText)
			// If it starts with a quote, find the end quote
			if strings.HasPrefix(replyText, "\"") {
				replyText = strings.TrimPrefix(replyText, "\"")
				if idx := strings.Index(replyText, "\""); idx > 0 {
					replyText = replyText[:idx]
				}
			}
			replyText = strings.TrimSpace(replyText)
			if len(replyText) > 0 && len(replyText) < 500 {
				return replyText
			}
		}
	}

	// No explicit label - fall back on a quoted sentence, but only if it looks
	// like a sentence and not a single keyword quoted in the motivation.
	if quoted := extractQuotedText(content); len(strings.Fields(quoted)) >= 2 {
		return quoted
	}
	return ""
}

// extractQuotedText extracts text within double quotes using regex.
func extractQuotedText(content string) string {
	re := regexp.MustCompile(`"([^"]+)"`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
