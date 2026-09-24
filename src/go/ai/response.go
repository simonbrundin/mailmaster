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

// parseResponse parses AI response text into a Response struct.
// First tries structured format (ÅTGÄRD:/MOTIVERING:), then falls back to free-text.
func parseResponse(content string) *Response {
	if structured := parseStructuredFormat(content); structured != nil {
		return structured
	}
	return parseFreeTextFallback(content)
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
			action := strings.TrimSpace(strings.TrimPrefix(line, "ÅTGÄRD:"))
			action = strings.TrimSpace(strings.TrimPrefix(action, "ACTION:"))
			resp.Action = normalizeAction(action)
			resp.Confidence = confidenceForAction(resp.Action)
			changed = true
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
				resp.ActionArgs[prefix.name] = strings.TrimSpace(strings.TrimPrefix(line, prefix.key))
			}
		}
	}

	// If action is uncertain, check if the motivation contains actionable content
	if resp.Action == "uncertain" {
		if parsed := parseMotivationForAction(resp.Motivation); parsed != nil {
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

// extractReplyFromContent extracts the reply text from content that may contain "Svara:" or similar.
func extractReplyFromContent(content string) string {
	// Look for patterns like "Svara: text" or "Reply: text"
	patterns := []string{"svara:", "reply:", "svara :", "reply :"}
	for _, pattern := range patterns {
		if idx := strings.Index(strings.ToLower(content), pattern); idx >= 0 {
			// Get text after the pattern
			replyText := strings.TrimSpace(content[idx+len(pattern):])
			// Clean up common suffixes
			replyText = strings.Split(replyText, "")[0] // Stop at first newline
			// If it starts with a quote, find the end quote
			if strings.HasPrefix(replyText, "\"") {
				replyText = strings.TrimPrefix(replyText, "\"")
				if idx := strings.Index(replyText, "\""); idx > 0 {
					replyText = replyText[:idx]
				}
			}
			// Otherwise take text until common separators
			for _, sep := range []string{" och ", " .", "!", "?"} {
				if idx := strings.Index(replyText, sep); idx > 5 {
					replyText = replyText[:idx]
					break
				}
			}
			replyText = strings.TrimSpace(replyText)
			if len(replyText) > 0 && len(replyText) < 500 {
				return replyText
			}
		}
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
