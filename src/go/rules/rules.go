package rules

import (
	"fmt"
	"strings"
	"time"
)

// Rule represents a mail processing rule
type Rule struct {
	Name      string
	Priority  int
	Condition Condition
	Actions   []Action
}

// Condition defines when a rule applies
type Condition struct {
	Raw string // Original markdown condition text
}

// Action represents what to do when a rule matches
type Action struct {
	Type  string
	Args  map[string]interface{}
}

// Response represents the AI's decision for an email
type Response struct {
	Action     string
	Motivation string
	Confidence float64
	ActionArgs map[string]interface{}
}

// RuleSet holds all parsed rules and instructions
type RuleSet struct {
	SystemPrompt  string
	RawRules      string // The raw rules text (markdown)
	Rules         []Rule
	UnknownAction string
}

// ParseMarkdown parses a markdown rules file into a RuleSet
func ParseMarkdown(content string) (*RuleSet, error) {
	rs := &RuleSet{
		Rules: make([]Rule, 0),
	}

	lines := strings.Split(content, "\n")
	currentSection := ""
	currentRule := (*Rule)(nil)
	var rulesText strings.Builder

	for _, line := range lines {
		// Track current section
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "##") {
			currentSection = strings.ToLower(strings.TrimSpace(line))
			continue
		}

		// System instruction section
		if currentSection == "# ai mail agent - regler" || currentSection == "# mailmaster - regler" || currentSection == "# system instruction" {
			if !strings.HasPrefix(line, "##") && line != "" && !strings.HasPrefix(line, "#") {
				rs.SystemPrompt += line + "\n"
			}
		}

		// Collect rules text for AI (everything after ## Regler section)
		if strings.Contains(strings.ToLower(line), "## regler") || strings.Contains(strings.ToLower(line), "## rules") {
			rulesText.WriteString(line + "\n")
			continue
		}
		if currentSection == "# regler" || currentSection == "# rules" || rulesText.Len() > 0 {
			rulesText.WriteString(line + "\n")
		}

		// New rule (## Regel: or ## Rule:)
		if strings.HasPrefix(line, "## Regel:") || strings.HasPrefix(line, "## Rule:") {
			if currentRule != nil {
				rs.Rules = append(rs.Rules, *currentRule)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "## Regel:"))
			name = strings.TrimSpace(strings.TrimPrefix(name, "## Rule:"))
			currentRule = &Rule{
				Name:      name,
				Priority:  len(rs.Rules),
				Condition: Condition{},
				Actions:   make([]Action, 0),
			}
		}

		// Parse condition
		if currentRule != nil {
			line = strings.TrimSpace(line)

			// Condition line
			if strings.HasPrefix(line, "Villkor:") || strings.HasPrefix(line, "Condition:") {
				cond := strings.TrimSpace(strings.TrimPrefix(line, "Villkor:"))
				cond = strings.TrimSpace(strings.TrimPrefix(cond, "Condition:"))
				currentRule.Condition.Raw = cond
			}

			// Action line
			if strings.HasPrefix(line, "Åtgärd:") || strings.HasPrefix(line, "Action:") {
				action := strings.TrimSpace(strings.TrimPrefix(line, "Åtgärd:"))
				action = strings.TrimSpace(strings.TrimPrefix(action, "Action:"))
				parseAction(currentRule, action)
			}

			// Indented action lines (continuation)
			if strings.HasPrefix(line, "- ") {
				parseAction(currentRule, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			}
		}
	}

	if currentRule != nil {
		rs.Rules = append(rs.Rules, *currentRule)
	}

	// Store raw rules text for AI
	rs.RawRules = rulesText.String()

	return rs, nil
}

func parseAction(rule *Rule, action string) {
	action = strings.TrimSpace(action)
	if action == "" {
		return
	}

	// reply with "Svara med texten: X" or "Reply: X"
	if strings.Contains(action, "Svara med texten:") {
		msg := strings.TrimSpace(strings.TrimPrefix(action, "Svara med texten:"))
		rule.Actions = append(rule.Actions, Action{Type: "reply", Args: map[string]interface{}{"message": msg}})
		return
	}

	// Also check for "Svara:" prefix
	if strings.HasPrefix(action, "Svara:") {
		msg := strings.TrimSpace(strings.TrimPrefix(action, "Svara:"))
		rule.Actions = append(rule.Actions, Action{Type: "reply", Args: map[string]interface{}{"message": msg}})
		return
	}

	// reply (plain)
	if action == "reply" || action == "svara" {
		rule.Actions = append(rule.Actions, Action{Type: "reply"})
		return
	}

	// archive
	if strings.Contains(action, "arkivera") || action == "archive" {
		rule.Actions = append(rule.Actions, Action{Type: "archive"})
		return
	}

	// read
	if action == "läsa" || action == "read" || action == "Läsa" {
		rule.Actions = append(rule.Actions, Action{Type: "read"})
		return
	}

	// forward
	if strings.Contains(action, "vidarebefordra") || strings.Contains(action, "forward") {
		rule.Actions = append(rule.Actions, Action{Type: "forward"})
		return
	}

	// move
	if strings.Contains(action, "flytta") || strings.Contains(action, "move") {
		rule.Actions = append(rule.Actions, Action{Type: "move"})
		return
	}
}

// BuildPromptForAI builds the full prompt to send to the AI
func (rs *RuleSet) BuildPromptForAI(email *EmailContext) string {
	var sb strings.Builder

	// System prompt
	sb.WriteString(strings.TrimSpace(rs.SystemPrompt))
	sb.WriteString("\n\n")

	// Raw rules text - AI understands Swedish directly!
	sb.WriteString("## REGLER\n\n")
	if rs.RawRules != "" {
		sb.WriteString(rs.RawRules)
	} else {
		// Fallback to parsed rules if raw text is empty
		for i, rule := range rs.Rules {
			sb.WriteString(fmt.Sprintf("### Regel %d: %s\n", i+1, rule.Name))
			if rule.Condition.Raw != "" {
				sb.WriteString(fmt.Sprintf("Villkor: %s\n", rule.Condition.Raw))
			}
			for _, action := range rule.Actions {
				sb.WriteString(fmt.Sprintf("- %s\n", formatAction(action)))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n## MAIL ATT HANTERA\n\n")
	sb.WriteString(fmt.Sprintf("Ämne: %s\n", email.Subject))
	sb.WriteString(fmt.Sprintf("Från: %s\n", email.From))
	sb.WriteString(fmt.Sprintf("Datum: %s\n", email.Date.Format("2006-01-02 15:04")))
	if email.HasAttachment {
		sb.WriteString("Bilagor: Ja\n")
	}
	sb.WriteString("\nInnehåll:\n")
	sb.WriteString(email.Body)
	sb.WriteString("\n")

	sb.WriteString("\n## SVAR\n\n")
	sb.WriteString("Analysera mailet mot reglerna och bestäm åtgärd(er).\n")
	sb.WriteString("Svara ENBART i följande format:\n")
	sb.WriteString("ÅTGÄRD: [reply, archive, read, move, forward]\n")
	sb.WriteString("MOTIVERING: [Kortfattad förklaring]\n")
	sb.WriteString("Om ÅTGÄRD är 'reply', inkludera svarstexten i MOTIVERING, t.ex.:\n")
	sb.WriteString("  ÅTGÄRD: reply, archive\n")
	sb.WriteString("  MOTIVERING: Regeln säger svara. Svara: Tack för ditt mail!\n")
	sb.WriteString("Om ingen regel matchar: ÅTGÄRD: uncertain\n")

	return sb.String()
}

func formatAction(a Action) string {
	switch a.Type {
	case "reply":
		if msg, ok := a.Args["message"].(string); ok {
			return "Svara: " + msg
		}
	case "archive":
		return "Arkivera mailet"
	case "read":
		return "Läs mailet"
	}
	return a.Type
}

// EmailContext holds the email data to process
type EmailContext struct {
	Subject       string
	From          string
	To            string
	Date          time.Time
	Body          string
	HasAttachment bool
	UID           uint32
	Folder        string
}
