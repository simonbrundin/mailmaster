package rules

import "strings"

// ExtractEmailAddress parses "Name <email@host.com>" or plain "email@host.com".
func ExtractEmailAddress(from string) string {
	if from == "" {
		return ""
	}
	if idx := strings.Index(from, "<"); idx >= 0 {
		if end := strings.Index(from[idx+1:], ">"); end > 0 {
			return strings.TrimSpace(from[idx+1 : idx+1+end])
		}
	}
	if strings.Contains(from, "@") {
		return strings.TrimSpace(from)
	}
	return ""
}
