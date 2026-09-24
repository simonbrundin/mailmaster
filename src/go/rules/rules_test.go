package rules

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseMarkdown(t *testing.T) {
	content := `# AI Mail Agent - Regler

Du är en intelligent mail-assistent. Analysera inkommande mail och bestäm vilken åtgärd som passar bäst.

## Regel: Nyhetsbrev
Villkor: subject contains "nyhetsbrev" OR from contains "newsletter"
Åtgärd: read
Prioritet: 1

## Regel: Faktura
Villkor: attachment exists OR subject contains "faktura"
Åtgärd: archive
- add_label: "Billing"
Prioritet: 2

## Regel: VIP-avsändare
Villkor: from in [boss@company.com, ceo@example.com]
Åtgärd: read
Prioritet: 10
`

	rs, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}

	if rs.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}

	if len(rs.Rules) != 3 {
		t.Errorf("Expected 3 rules, got %d", len(rs.Rules))
	}

	// Rules are parsed in file order
	if rs.Rules[0].Name != "Nyhetsbrev" {
		t.Errorf("Expected rule name 'Nyhetsbrev', got '%s'", rs.Rules[0].Name)
	}

	// Check Nyhetsbrev rule has a condition
	if rs.Rules[0].Condition.Raw == "" {
		t.Error("Expected Nyhetsbrev rule to have a condition")
	}

	// Check Faktura rule
	if rs.Rules[1].Name != "Faktura" {
		t.Errorf("Expected rule name 'Faktura', got '%s'", rs.Rules[1].Name)
	}

	// Check Faktura has archive action
	if len(rs.Rules[1].Actions) == 0 || rs.Rules[1].Actions[0].Type != "archive" {
		t.Errorf("Expected Faktura rule to have 'archive' action")
	}
}

func TestParseMarkdownWithActions(t *testing.T) {
	content := `# AI Mail Agent - Regler

Du är en intelligent mail-assistent.

## Regel: Svara på kund
Villkor: subject contains "order"
Åtgärd: reply

## Regel: Arkivera spam
Villkor: subject contains "spam"
Åtgärd: archive
`

	rs, err := ParseMarkdown(content)
	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}

	if len(rs.Rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(rs.Rules))
	}

	// First rule: reply action
	if rs.Rules[0].Actions[0].Type != "reply" {
		t.Errorf("Expected first rule to have 'reply' action, got '%s'", rs.Rules[0].Actions[0].Type)
	}

	// Second rule: archive action
	if rs.Rules[1].Actions[0].Type != "archive" {
		t.Errorf("Expected second rule to have 'archive' action, got '%s'", rs.Rules[1].Actions[0].Type)
	}
}

func TestBuildPromptForAI(t *testing.T) {
	content := `# AI Mail Agent - Regler
Du är en assistent.

## Regel: Test
Villkor: subject contains "test"
Åtgärd: read
`

	rs, _ := ParseMarkdown(content)

	email := &EmailContext{
		Subject:       "Test mail",
		From:          "test@example.com",
		Date:          time.Now(),
		Body:          "Hello World",
		HasAttachment: true,
	}

	prompt := rs.BuildPromptForAI(email)
	
	if !strings.Contains(prompt, "Test mail") {
		t.Error("Prompt should contain email subject")
	}
	
	if !strings.Contains(prompt, "test@example.com") {
		t.Error("Prompt should contain email from")
	}
	
	if !strings.Contains(prompt, "Bilagor: Ja") {
		t.Error("Prompt should indicate attachments")
	}
	
	if !strings.Contains(prompt, "ÅTGÄRD:") {
		t.Error("Prompt should contain action request")
	}
}

func TestLoadFromFile(t *testing.T) {
	content := `# AI Mail Agent - Regler
Test prompt.

## Regel: Archive Spam
Villkor: subject contains "spam"
Åtgärd: archive
`
	
	tmpfile, err := os.CreateTemp("", "rules*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	rs, err := ParseMarkdown(string(data))
	if err != nil {
		t.Fatalf("ParseMarkdown failed: %v", err)
	}

	if len(rs.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rs.Rules))
	}
}

func TestExtractEmailAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user@example.com", "user@example.com"},
		{"John Doe <john@example.com>", "john@example.com"},
		{"", ""},
		{"No email here", ""},
	}

	for _, tt := range tests {
		result := ExtractEmailAddress(tt.input)
		if result != tt.expected {
			t.Errorf("ExtractEmailAddress(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
