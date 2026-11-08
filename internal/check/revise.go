package check

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/ionut-t/bark/v2/internal/debuglog"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/llm"
)

//go:embed revise.md
var reviseSystem string

// ReviseSystem returns the system prompt for revising the intro and verdict
// in the voice of persona.
func ReviseSystem(persona string) string {
	return reviseSystem + persona
}

// NeedsRevision reports whether events, after the check, have an intro or
// verdict that may rest on findings the check doubted.
func NeedsRevision(events []findings.Event) bool {
	doubted, persona := false, false
	for _, e := range events {
		doubted = doubted || (e.Finding != nil && e.Finding.Doubt != "")
		persona = persona || e.Intro != "" || e.Verdict != ""
	}
	return doubted && persona
}

// RevisePrompt builds the user prompt for Revise: the review's intro and
// verdict, the findings that remain and those the check set aside.
func RevisePrompt(events []findings.Event) string {
	var b strings.Builder
	var kept, doubted strings.Builder
	for _, e := range events {
		switch {
		case e.Intro != "":
			fmt.Fprintf(&b, "## Original intro\n\n%s\n\n", e.Intro)
		case e.Verdict != "":
			fmt.Fprintf(&b, "## Original verdict\n\n%s\n\n", e.Verdict)
		case e.Finding != nil && e.Finding.Doubt != "":
			fmt.Fprintf(&doubted, "- %s\n", summary(*e.Finding))
		case e.Finding != nil:
			fmt.Fprintf(&kept, "- %s: %s\n", summary(*e.Finding), oneLine(e.Finding.Body))
		}
	}
	b.WriteString("## Findings that remain\n\n")
	if kept.Len() == 0 {
		b.WriteString("None.\n")
	}
	b.WriteString(kept.String())
	b.WriteString("\n## Findings set aside\n\n")
	b.WriteString(doubted.String())
	return b.String()
}

func summary(f findings.Finding) string {
	s := fmt.Sprintf("[%s] %s", strings.ToUpper(string(f.Severity)), f.Title)
	if f.File != "" {
		s += " (" + f.File + ")"
	}
	return s
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Revise rewrites the intro and verdict in events in the persona's voice so
// they don't rest on findings the check doubted. It makes one small call
// (the persona, the intro and verdict, and the findings' titles; no code)
// and only when NeedsRevision. Each of the intro and verdict is replaced
// only if the model wrote a new one, so on any failure events are returned
// as they were, with the error.
func Revise(ctx context.Context, client llm.LLM, persona string, events []findings.Event, startLog func(system, prompt string) *debuglog.Log) ([]findings.Event, error) {
	if !NeedsRevision(events) || strings.TrimSpace(persona) == "" {
		return events, nil
	}

	system, prompt := ReviseSystem(persona), RevisePrompt(events)
	var log *debuglog.Log
	if startLog != nil {
		log = startLog(system, prompt)
	}
	resp, err := client.Generate(ctx, system, prompt)
	log.Write(resp.Content, resp.Usage)
	log.Finish(err)
	if err != nil {
		return events, fmt.Errorf("error revising the verdict: %w", err)
	}

	var intro, verdict string
	for _, e := range findings.Parse(resp.Content) {
		if e.Intro != "" {
			intro = e.Intro
		}
		if e.Verdict != "" {
			verdict = e.Verdict
		}
	}
	if intro == "" && verdict == "" {
		return events, fmt.Errorf("error revising the verdict: no intro or verdict in the response")
	}

	out := make([]findings.Event, len(events))
	for i, e := range events {
		switch {
		case e.Intro != "" && intro != "":
			e.Intro = intro
		case e.Verdict != "" && verdict != "":
			e.Verdict = verdict
		}
		out[i] = e
	}
	return out, nil
}
