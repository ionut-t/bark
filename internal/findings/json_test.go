package findings

import (
	"encoding/json"
	"testing"

	"github.com/ionut-t/bark/v2/internal/prices"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A report keeps every finding, the set-aside ones marked, so a script or
// an agent can tell what the check kept from what it doubted.
func TestNewReport(t *testing.T) {
	kept := Finding{File: "a.go", Line: 2, EndLine: 3, Severity: High, Category: "bug", Title: "Nil map", Body: "m is nil.",
		Original: "var m map[string]int", Suggestion: "m := map[string]int{}", Remark: "Illogical."}
	doubted := Finding{File: "b.go", Line: 5, Severity: Medium, Category: "style", Title: "Long name", Body: "Too long.", Doubt: "It's a preference."}
	unchecked := Finding{File: "c.go", Severity: Low, Category: "docs", Title: "Typo", Body: "Teh.", Unchecked: "it failed.", Repaired: true}
	repeat := Finding{File: "d.go", Line: 1, Severity: High, Category: "bug", Title: "Leak", Body: "Leaks.", RaisedBefore: "Leak in d.go"}
	dismissed := Finding{File: "e.go", Line: 1, Severity: Low, Category: "style", Title: "Nit", Body: "Nit.", Dismissed: "won't fix"}

	r := NewReport([]Event{
		{Intro: "Let's see."},
		{Finding: &kept},
		{Text: "stray output"},
		{Finding: &doubted},
		{Finding: &unchecked},
		{Finding: &repeat},
		{Finding: &dismissed},
		{Text: "more"},
		{Verdict: "Fix the map."},
		{Text: TruncatedNote},
	}, 4)

	assert.Equal(t, ReportVersion, r.Version)
	assert.Equal(t, "Let's see.", r.Intro)
	assert.Equal(t, "Fix the map.", r.Verdict)
	assert.Equal(t, "stray output\nmore", r.Text, "the truncated note is a flag, not text")
	assert.True(t, r.Truncated)
	assert.Equal(t, 4, r.Checked)
	require.Len(t, r.Findings, 5)
	assert.Equal(t, ReportFinding{File: "a.go", Line: 2, EndLine: 3, Severity: "high", Category: "bug", Title: "Nil map", Body: "m is nil.",
		Original: "var m map[string]int", Suggestion: "m := map[string]int{}", Remark: "Illogical."}, r.Findings[0])
	assert.Equal(t, ReportFinding{File: "b.go", Line: 5, Severity: "medium", Category: "style", Title: "Long name", Body: "Too long.",
		SetAside: true, Doubt: "It's a preference."}, r.Findings[1])
	assert.Equal(t, ReportFinding{File: "c.go", Severity: "low", Category: "docs", Title: "Typo", Body: "Teh.",
		Unchecked: "it failed.", Repaired: true}, r.Findings[2], "an unchecked finding isn't set aside")
	assert.True(t, r.Findings[3].SetAside)
	assert.Equal(t, "Leak in d.go", r.Findings[3].RaisedBefore)
	assert.True(t, r.Findings[4].SetAside)
	assert.Equal(t, "won't fix", r.Findings[4].Dismissed)
}

// The JSON has a fixed shape: findings is a list even when empty, and
// setAside is always there, so readers needn't treat a missing key as
// false.
func TestReportJSON(t *testing.T) {
	b, err := json.Marshal(NewReport(nil, 0))
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":1,"findings":[],"checked":0}`, string(b))

	f := Finding{File: "a.go", Severity: Low, Category: "docs", Title: "Typo", Body: "Teh."}
	r := NewReport([]Event{{Finding: &f}}, 0)
	r.Partial = "reviewed 2 of 3 files"
	r.Note = "nothing new"
	r.Cost = &prices.Cost{USD: 0.004, Calls: 2, Steps: []prices.Step{{Name: "review", USD: 0.003}, {Name: "check", USD: 0.001}}, PricesDate: "2026-11-09"}
	b, err = json.Marshal(r)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":1,"findings":[{"file":"a.go","severity":"low","category":"docs","title":"Typo","body":"Teh.","setAside":false}],
		"checked":0,"partial":"reviewed 2 of 3 files","note":"nothing new",
		"cost":{"usd":0.004,"calls":2,"steps":[{"name":"review","usd":0.003},{"name":"check","usd":0.001}],"pricesDate":"2026-11-09"}}`, string(b))
}
