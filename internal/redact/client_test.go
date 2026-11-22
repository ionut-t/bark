package redact

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
)

// recorder is an llm.LLM that keeps what it was sent.
type recorder struct {
	mu               sync.Mutex
	systems, prompts []string
}

func (r *recorder) record(system, prompt string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.systems = append(r.systems, system)
	r.prompts = append(r.prompts, prompt)
}

func (r *recorder) Generate(_ context.Context, system, prompt string) (llm.Response, error) {
	r.record(system, prompt)
	return llm.Response{Content: "ok"}, nil
}

func (r *recorder) Stream(_ context.Context, system, prompt string) (<-chan llm.Response, <-chan error) {
	r.record(system, prompt)
	out, errs := make(chan llm.Response, 1), make(chan error)
	out <- llm.Response{Content: "ok"}
	close(out)
	close(errs)
	return out, errs
}

func TestClientMasksWhatItSends(t *testing.T) {
	rec := &recorder{}
	var warn bytes.Buffer
	c := Wrap(rec, &warn)

	diff := "+++ b/config/prod.env\n@@ -0,0 +1,2 @@\n+AWS_ACCESS_KEY_ID=" + awsKeyID + "\n+GITHUB_TOKEN=" + ghToken + "\n"
	if _, err := c.Generate(context.Background(), "rules: "+anthropicKey, diff); err != nil {
		t.Fatal(err)
	}
	out, _ := c.Stream(context.Background(), "system", "again: "+awsKeyID)
	for range out {
	}

	for _, s := range append(rec.systems, rec.prompts...) {
		for _, secret := range []string{awsKeyID, ghToken, anthropicKey} {
			if strings.Contains(s, secret) {
				t.Errorf("sent %q, which holds a secret", s)
			}
		}
	}
	if !strings.Contains(rec.prompts[0], "AWS_ACCESS_KEY_ID=[REDACTED:aws-access-key-id]") {
		t.Errorf("prompt = %q, want the key masked", rec.prompts[0])
	}

	got := warn.String()
	want := "bark: warning: masked 3 secret(s) before sending them to the model provider: " +
		"anthropic-api-key, aws-access-key-id (config/prod.env), github-token (config/prod.env)\n"
	if got != want {
		t.Errorf("warning =\n%q\nwant\n%q", got, want)
	}
}

func TestClientWarnsOnlyAboutNewSecrets(t *testing.T) {
	rec := &recorder{}
	var warn bytes.Buffer
	c := Wrap(rec, &warn)
	ctx := context.Background()

	_, _ = c.Generate(ctx, "", "a "+awsKeyID)
	_, _ = c.Generate(ctx, "", "b "+awsKeyID) // the check sees the same key
	if n := strings.Count(warn.String(), "\n"); n != 1 {
		t.Errorf("warned %d times about one key:\n%s", n, warn.String())
	}
	_, _ = c.Generate(ctx, "", "c "+awsKeyID+" "+googleKey)
	if !strings.HasSuffix(warn.String(), "masked 1 secret(s) before sending them to the model provider: google-api-key\n") {
		t.Errorf("warning = %q, want only the new key", warn.String())
	}
}

func TestClientWithoutSecretsIsQuiet(t *testing.T) {
	rec := &recorder{}
	var warn bytes.Buffer
	c := Wrap(rec, &warn)
	_, _ = c.Generate(context.Background(), "system", "+func main() {}\n")
	if warn.Len() != 0 {
		t.Errorf("warning = %q, want none", warn.String())
	}
	if rec.prompts[0] != "+func main() {}\n" {
		t.Errorf("prompt = %q, want it unchanged", rec.prompts[0])
	}
}

// A prompt with masked secrets ends with a note saying what the
// placeholders are, so the model doesn't take them for text in the code.
func TestClientExplainsPlaceholders(t *testing.T) {
	rec := &recorder{}
	c := Wrap(rec, nil)
	ctx := context.Background()

	_, _ = c.Generate(ctx, "system", "+key := \""+awsKeyID+"\"\n")
	assert.Equal(t, "system", rec.systems[0], "the system prompt stays the same, so it can be cached")
	assert.True(t, strings.HasPrefix(rec.prompts[0], "+key := \"[REDACTED:aws-access-key-id]\"\n"))
	assert.True(t, strings.HasSuffix(rec.prompts[0], "\n\n"+Note), "prompt = %q", rec.prompts[0])

	// A secret in the system prompt (house rules, say) gets the note too.
	_, _ = c.Generate(ctx, "rules "+awsKeyID, "+x := 1\n")
	assert.Equal(t, "+x := 1\n\n"+Note, rec.prompts[1])

	// Without a secret, the prompt is sent as it is.
	_, _ = c.Generate(ctx, "system", "+x := 1\n")
	assert.Equal(t, "+x := 1\n", rec.prompts[2])
}

// Sent and logged prompts are the same (see debuglog).
func TestPrompts(t *testing.T) {
	system, prompt, matches := Prompts("rules", "+key := \""+awsKeyID+"\"")
	assert.Equal(t, "rules", system)
	assert.Equal(t, "+key := \"[REDACTED:aws-access-key-id]\"\n\n"+Note, prompt)
	assert.Len(t, matches, 1)
}
