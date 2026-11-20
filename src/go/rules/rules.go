package rules

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// orderedListItem matches action lines written as "1. Svara med: ...".
var orderedListItem = regexp.MustCompile(`^\d+[.)]\s+(.*)$`)

// ruleHeading matches rule headings, e.g. "## Regel: Nyhetsbrev" or
// "### Regeln: Ämne innehåller viktigt".
var ruleHeading = regexp.MustCompile(`^#{2,4}\s+(?:Regel|Regeln|Rule):\s*(.*)$`)

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
	Type string
	Args map[string]interface{}
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
	rulesSection := false
	var rulesText strings.Builder

	for _, line := range lines {
		// Track current section
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "##") {
			currentSection = strings.ToLower(strings.TrimSpace(line))
			continue
		}

		// System instruction section: the intro before the rules section.
		if !rulesSection && currentSection != "# regler" && currentSection != "# rules" && strings.HasPrefix(currentSection, "# ") {
			if !strings.HasPrefix(line, "#") && line != "" {
				rs.SystemPrompt += line + "\n"
			}
		}

		// Collect rules text for AI (everything after ## Regler section)
		if strings.Contains(strings.ToLower(line), "## regler") || strings.Contains(strings.ToLower(line), "## rules") {
			rulesSection = true
			rulesText.WriteString(line + "\n")
			continue
		}
		if currentSection == "# regler" || currentSection == "# rules" || rulesText.Len() > 0 {
			rulesText.WriteString(line + "\n")
		}

		// New rule (## Regel: or ## Rule:)
		if m := ruleHeading.FindStringSubmatch(line); m != nil {
			if currentRule != nil {
				rs.Rules = append(rs.Rules, *currentRule)
			}
			currentRule = &Rule{
				Name:      strings.TrimSpace(m[1]),
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

			// Numbered action lines, e.g. "1. Svara med: "..."
			if m := orderedListItem.FindStringSubmatch(line); m != nil {
				parseAction(currentRule, strings.TrimSpace(m[1]))
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

	// reply with "Svara med texten: X", "Svara med: X" or "Svara: X"
	for _, prefix := range []string{"Svara med texten:", "Svara med:", "Svara:", "Reply:", "reply with:"} {
		if strings.Contains(action, prefix) {
			msg := strings.TrimSpace(strings.TrimPrefix(action, prefix))
			msg = stripQuotes(msg)
			rule.Actions = append(rule.Actions, Action{Type: "reply", Args: map[string]interface{}{"message": msg}})
			return
		}
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

// FindRule looks up a rule by name. The match is case-insensitive and falls
// back on a partial match, so the AI can answer with a shortened rule name.
func (rs *RuleSet) FindRule(name string) *Rule {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	lower := strings.ToLower(name)
	for i := range rs.Rules {
		if strings.ToLower(rs.Rules[i].Name) == lower {
			return &rs.Rules[i]
		}
	}
	for i := range rs.Rules {
		ruleName := strings.ToLower(rs.Rules[i].Name)
		if ruleName != "" && (strings.Contains(ruleName, lower) || strings.Contains(lower, ruleName)) {
			return &rs.Rules[i]
		}
	}
	return nil
}

// FindRuleInText looks for a rule name inside a free text, e.g. the AI's
// motivation. It returns the first rule whose name is mentioned in the text.
func (rs *RuleSet) FindRuleInText(text string) *Rule {
	lower := strings.ToLower(text)
	if strings.TrimSpace(lower) == "" {
		return nil
	}
	for i := range rs.Rules {
		name := strings.ToLower(rs.Rules[i].Name)
		if name != "" && strings.Contains(lower, name) {
			return &rs.Rules[i]
		}
	}
	return nil
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
	sb.WriteString("Svara ENBART i följande format (varje del på egen rad, utan extra text):\n")
	sb.WriteString("ÅTGÄRD: [reply, archive, read, move, forward]\n")
	sb.WriteString("TILL: [email-ADRESS för forward, MÅSTE vara i formatet name@domain.se]\n")
	sb.WriteString("SVAR: [svarstext för reply]\n")
	sb.WriteString("MOTIVERING: [Kortfattad förklaring]\n")
	sb.WriteString("REGEL: [namnet på regeln som matchade, eller \"ingen\" om ingen regel matchade]\n")
	sb.WriteString("\nKorrekta exempel:\n")
	sb.WriteString("ÅTGÄRD: reply, archive\n")
	sb.WriteString("SVAR: Tack för ditt mail!\n")
	sb.WriteString("MOTIVERING: Regeln säger svara och arkivera.\n")
	sb.WriteString("REGEL: Nyhetsbrev\n")
	sb.WriteString("\nÅTGÄRD: forward, archive\n")
	sb.WriteString("TILL: simon.brundin@gmail.com\n")
	sb.WriteString("MOTIVERING: Vidarebefordra till Simon.\n")
	sb.WriteString("\nFEL: Om forward anges utan TILL: med giltig email-adress kommer åtgärden INTE utföras!\n")

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

// stripQuotes removes a single pair of surrounding double quotes.
func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
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
