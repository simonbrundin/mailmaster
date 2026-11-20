package mcp

import "testing"

func TestDescribeMatchReportsRuleAndCondition(t *testing.T) {
	item := analysisItem{
		Rule:      "Ämne innehåller viktigt",
		Condition: "Ämne eller titel innehåller ordet \"viktigt\"",
	}

	got := describeMatch(item)
	want := "Mailet matchar regeln \"Ämne innehåller viktigt\". Villkor: Ämne eller titel innehåller ordet \"viktigt\""
	if got != want {
		t.Errorf("Expected %q, got %q", want, got)
	}
}

func TestDescribeMatchWithoutRule(t *testing.T) {
	item := analysisItem{}

	got := describeMatch(item)
	want := "Ingen regel matchade det här mailet - ingen åtgärd rekommenderas."
	if got != want {
		t.Errorf("Expected %q, got %q", want, got)
	}
}

func TestDescribeMatchWithActionsButNoRule(t *testing.T) {
	item := analysisItem{Actions: []suggestedAction{{Type: "archive"}}}

	got := describeMatch(item)
	want := "Ingen regel matchade tydligt, men AI:t föreslår en åtgärd baserat på mailets innehåll."
	if got != want {
		t.Errorf("Expected %q, got %q", want, got)
	}
}

func TestDescribeAction(t *testing.T) {
	args := map[string]interface{}{
		"reply":  "Tack för ditt mejl!",
		"to":     "simon@example.com",
		"folder": "INBOX.Archive",
	}

	tests := []struct {
		action string
		want   string
	}{
		{"reply", "Svara: Tack för ditt mejl!"},
		{"forward", "Vidarebefordra till simon@example.com"},
		{"move", "Flytta till INBOX.Archive"},
		{"archive", "Arkivera mailet"},
		{"read", "Markera mailet som läst"},
	}

	for _, tt := range tests {
		if got := describeAction(tt.action, args); got != tt.want {
			t.Errorf("action %q: expected %q, got %q", tt.action, tt.want, got)
		}
	}
}

func TestExecArgsKeepsOnlyUsableArguments(t *testing.T) {
	args := map[string]interface{}{
		"reply":   "Hej",
		"to":      "a@b.se",
		"actions": []string{"reply"},
		"number":  42,
	}

	got := execArgs(args)
	if len(got) != 2 {
		t.Fatalf("Expected 2 arguments, got %d (%#v)", len(got), got)
	}
	if got["reply"] != "Hej" || got["to"] != "a@b.se" {
		t.Errorf("Unexpected arguments: %#v", got)
	}
}

func TestAccountFromPath(t *testing.T) {
	got := accountFromPath("/api/v1/accounts/Michael%20Brundins/analyze")
	if got != "Michael%20Brundins" {
		t.Errorf("Expected account name, got %q", got)
	}
}
