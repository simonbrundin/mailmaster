package ai

import "testing"

func TestParseStructuredMultiActionKeepsReplyText(t *testing.T) {
	content := `ÅTGÄRD: reply, archive
SVAR: Jag hanterar det skitsnappt och återkommer så snart jag kan!
MOTIVERING: Ämnet innehåller ordet "Viktigt", så mejlet ska besvaras och arkiveras.`

	resp := parseResponse(content)

	if resp.Action != "reply" {
		t.Fatalf("Expected action 'reply', got %q", resp.Action)
	}

	reply, _ := resp.ActionArgs["reply"].(string)
	want := "Jag hanterar det skitsnappt och återkommer så snart jag kan!"
	if reply != want {
		t.Errorf("Expected reply text %q, got %q", want, reply)
	}

	actions, ok := resp.ActionArgs["actions"].([]string)
	if !ok {
		t.Fatalf("Expected 'actions' argument, got %#v", resp.ActionArgs["actions"])
	}
	if len(actions) != 2 || actions[0] != "reply" || actions[1] != "archive" {
		t.Errorf("Expected actions [reply archive], got %v", actions)
	}
}

func TestParseStructuredStripsQuotesFromReply(t *testing.T) {
	content := `ÅTGÄRD: reply
SVAR: "Tack för ditt mejl Simon, din stora spammare!"`

	resp := parseResponse(content)

	reply, _ := resp.ActionArgs["reply"].(string)
	want := "Tack för ditt mejl Simon, din stora spammare!"
	if reply != want {
		t.Errorf("Expected reply text %q, got %q", want, reply)
	}
}

func TestParseStructuredReplyWithoutActionLine(t *testing.T) {
	content := `SVAR: Hej och välkommen
MOTIVERING: Mejlet ska besvaras.`

	resp := parseResponse(content)

	if resp.Action != "reply" {
		t.Fatalf("Expected action 'reply', got %q", resp.Action)
	}
	if reply, _ := resp.ActionArgs["reply"].(string); reply != "Hej och välkommen" {
		t.Errorf("Expected reply text %q, got %q", "Hej och välkommen", reply)
	}
}

func TestExtractReplyFromContentKeepsFullText(t *testing.T) {
	got := extractReplyFromContent("Svara: Tack för ditt mejl Simon, din stora spammare!")
	want := "Tack för ditt mejl Simon, din stora spammare!"
	if got != want {
		t.Errorf("Expected %q, got %q", want, got)
	}
}

func TestExtractReplyFromContentIgnoresSingleQuotedKeyword(t *testing.T) {
	// The motivation quotes a keyword - that is not a reply text.
	got := extractReplyFromContent(`Ämnet innehåller ordet "Viktigt", så mejlet ska besvaras och arkiveras.`)
	if got != "" {
		t.Errorf("Expected empty reply text, got %q", got)
	}
}

func TestParseActions(t *testing.T) {
	actions := parseActions("reply, archive")
	if len(actions) != 2 || actions[0] != "reply" || actions[1] != "archive" {
		t.Errorf("Expected [reply archive], got %v", actions)
	}

	if got := parseActions("arkivera"); len(got) != 1 || got[0] != "archive" {
		t.Errorf("Expected [archive], got %v", got)
	}
}

func TestParseStructuredReadsRuleName(t *testing.T) {
	content := `ÅTGÄRD: reply, archive
SVAR: Jag hanterar det snabbt!
MOTIVERING: Ämnet innehåller ordet "viktigt"
REGEL: Ämne innehåller viktigt`

	resp := parseResponse(content)

	if resp.Rule != "Ämne innehåller viktigt" {
		t.Errorf("Expected rule %q, got %q", "Ämne innehåller viktigt", resp.Rule)
	}
}

func TestParseRuleIgnoresMissingRuleLine(t *testing.T) {
	if got := ParseRule("ÅTGÄRD: read\nMOTIVERING: inget"); got != "" {
		t.Errorf("Expected empty rule, got %q", got)
	}
}
