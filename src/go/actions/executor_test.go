package actions

import (
	"strings"
	"testing"

	"mailagent/ai"
	"mailagent/rules"
)

func TestExtractReplyTextUsesActionArg(t *testing.T) {
	e := &Executor{}
	response := &rules.Response{
		Action:     "reply",
		Motivation: `Ämnet innehåller ordet "Viktigt", så mejlet ska besvaras.`,
		ActionArgs: map[string]interface{}{
			"reply": "Jag hanterar det skitsnabbt och återkommer så snart jag kan!",
		},
	}

	got := e.extractReplyText(response)
	want := "Jag hanterar det skitsnabbt och återkommer så snart jag kan!"
	if got != want {
		t.Errorf("Expected %q, got %q", want, got)
	}
}

func TestExtractReplyTextIgnoresQuotedKeyword(t *testing.T) {
	e := &Executor{}
	response := &rules.Response{
		Action:     "reply",
		Motivation: `Ämnet innehåller ordet "Viktigt", så mejlet ska besvaras och arkiveras.`,
	}

	if got := e.extractReplyText(response); got != "" {
		t.Errorf("Expected empty reply text, got %q", got)
	}
}

func TestExtractReplyTextStripsQuotes(t *testing.T) {
	e := &Executor{}
	response := &rules.Response{
		Action:     "reply",
		ActionArgs: map[string]interface{}{"reply": `"Tack för ditt mejl Simon, din stora spammare!"`},
	}

	got := e.extractReplyText(response)
	want := "Tack för ditt mejl Simon, din stora spammare!"
	if got != want {
		t.Errorf("Expected %q, got %q", want, got)
	}
}

func TestCollectActionsUsesExplicitActionList(t *testing.T) {
	e := &Executor{}
	response := &rules.Response{
		Action:     "reply",
		Motivation: "Mejlet ska besvaras och arkiveras.",
		ActionArgs: map[string]interface{}{
			"actions": []string{"reply", "archive"},
		},
	}

	actions := e.collectActions(response)
	if !sliceContains(actions, "reply") || !sliceContains(actions, "archive") {
		t.Errorf("Expected reply and archive, got %v", actions)
	}
}

// TestReplyTextFromRealAIOutput walks the whole chain: raw AI output (as the
// model produces it for the rules in rules.md) -> parsed response -> reply
// text that will be sent.
func TestReplyTextFromRealAIOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "ämne innehåller viktigt",
			raw: `ÅTGÄRD: reply, archive
SVAR: Jag hanterar det skitsnabbt och återkommer så snart jag kan!
MOTIVERING: Ämnet innehåller ordet "Viktigt", så mejlet ska besvaras och arkiveras.`,
			want: "Jag hanterar det skitsnabbt och återkommer så snart jag kan!",
		},
		{
			name: "mejl från Simon",
			raw: `ÅTGÄRD: reply, archive
SVAR: Tack för ditt mejl Simon, din stora spammare!
MOTIVERING: Avsändaren är simonbrundin@gmail.com.`,
			want: "Tack för ditt mejl Simon, din stora spammare!",
		},
		{
			name: "minnesmejl",
			raw: `ÅTGÄRD: forward, archive
TILL: simonbrundin@gmail.com
SVAR: Nu jäklar ska vi boosta minnet!
MOTIVERING: Mejlet handlar om att förbättra minnet.`,
			want: "Nu jäklar ska vi boosta minnet!",
		},
	}

	e := &Executor{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := ai.ParseResponse(tt.raw)
			response := &rules.Response{
				Action:     parsed.Action,
				Motivation: parsed.Motivation,
				Confidence: parsed.Confidence,
				ActionArgs: parsed.ActionArgs,
			}

			got := e.extractReplyText(response)
			if got != tt.want {
				t.Errorf("Expected reply text %q, got %q", tt.want, got)
			}
		})
	}
}

func TestQuotedFromMotivation(t *testing.T) {
	motivation := `Mejlet handlar om att förbättra minnet och ska därför vidarebefordras med titeln "Nu jäklar ska vi boosta minnet!" samt arkiveras.`

	got := quotedFromMotivation(motivation)
	want := "Nu jäklar ska vi boosta minnet!"
	if got != want {
		t.Errorf("Expected %q, got %q", want, got)
	}

	if got := quotedFromMotivation(`Ämnet innehåller ordet "Viktigt".`); got != "" {
		t.Errorf("Expected empty note for single keyword, got %q", got)
	}
}

// TestReplyMessageBodyMatchesRuleText verifies the body of the mail that is
// actually sent: it must be the text from the rule, nothing else.
func TestReplyMessageBodyMatchesRuleText(t *testing.T) {
	raw := `ÅTGÄRD: reply, archive
SVAR: Jag hanterar det skitsnabbt och återkommer så snart jag kan!
MOTIVERING: Ämnet innehåller ordet "Viktigt", så mejlet ska besvaras och arkiveras.`

	parsed := ai.ParseResponse(raw)
	e := &Executor{}
	text := e.extractReplyText(&rules.Response{
		Action:     parsed.Action,
		Motivation: parsed.Motivation,
		ActionArgs: parsed.ActionArgs,
	})

	msg := formatMessage("michael@brundins.se", "simonbrundin@gmail.com", "Re: Viktigt", text)

	want := "Subject: Re: Viktigt\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nJag hanterar det skitsnabbt och återkommer så snart jag kan!"
	if !strings.HasSuffix(msg, want) {
		t.Errorf("Expected message to end with %q, got %q", want, msg)
	}
}
