package redact

import (
	"strings"
	"testing"
)

// Fake secrets, built from parts so secret scanners (GitHub push
// protection, Bark's own masking when it reviews this file) don't match the
// source.
var (
	awsKeyID     = "AKIA" + "IOSFODNN7EXAMPLE"
	awsSecret    = "wJalrXUtnFEMI/K7MDENG/" + "bPxRfiCYEXAMPLEKEY"
	ghToken      = "ghp" + "_" + strings.Repeat("a1B2c3D4e5", 4)[:36]
	ghPAT        = "github" + "_pat_" + "11ABCDEFG0" + strings.Repeat("xY9_", 18)
	openAIKey    = "sk-" + "proj-" + strings.Repeat("Ab3_-", 10)
	openAILegacy = "sk-" + "Ab3Cd4Ef5Gh6Ij7Kl8Mn" + "T3Blbk" + "FJ" + "Op9Qr0St1Uv2Wx3Yz4Ab"
	anthropicKey = "sk-" + "ant-api03-" + strings.Repeat("Zz9_-", 20)
	googleKey    = "AI" + "za" + "SyA1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6Q"
	googleSecret = "GOC" + "SPX-" + "a1B2c3D4e5F6g7H8i9J0k1L2m3N4"
	stripeLive   = "sk_" + "live_" + "51AbCdEfGh" + strings.Repeat("Ij0Kl1Mn2", 10)
	stripeTest   = "sk_" + "test_" + "51DummyKeyForBarkTesting0000000000000000000000000000"
	stripeRK     = "rk_" + "live_" + strings.Repeat("Zy9Xw8", 8)
	stripeHook   = "whs" + "ec_" + "DummyWebhookSecretForBarkTesting00000"
	slackHook    = "https://hooks.slack.com/services/" + "T00000000/B00000000/" + strings.Repeat("X", 24)
	slackBot     = "xo" + "xb-" + "123456789012-1234567890123-" + strings.Repeat("aB3", 8)
	dbPassword   = "Sup3rS3cret" + "Passw0rd!"
	pemBegin     = "-----BEGIN " + "RSA PRIVATE KEY-----"
	pemEnd       = "-----END " + "RSA PRIVATE KEY-----"
	pemBody1     = "MIIEowIBAAKCAQEA7bq3W0xYz1Ab2Cd3Ef4Gh5Ij6Kl7Mn8Op9Qr0St"
	pemBody2     = "u1Vw2Xy3Za4Bc5De6Fg7Hi8Jk9Lm0No1Pq2Rs3Tu4Vw5Xy6Za7Bc8De9"
)

func TestMaskKinds(t *testing.T) {
	tests := []struct {
		name, in, kind, secret string
	}{
		{"aws access key id", `key := "` + awsKeyID + `"`, "aws-access-key-id", awsKeyID},
		{"aws secret key in env", "AWS_SECRET_ACCESS_KEY=" + awsSecret, "aws-secret-access-key", awsSecret},
		{"aws secret key in json", `"SecretAccessKey": "` + awsSecret + `",`, "aws-secret-access-key", awsSecret},
		{"github token", "token: " + ghToken, "github-token", ghToken},
		{"github fine-grained token", "GH_TOKEN=" + ghPAT, "github-token", ghPAT},
		{"openai project key", `OPENAI_API_KEY="` + openAIKey + `"`, "openai-api-key", openAIKey},
		{"openai legacy key", "key = '" + openAILegacy + "'", "openai-api-key", openAILegacy},
		{"anthropic key", "ANTHROPIC_API_KEY=" + anthropicKey, "anthropic-api-key", anthropicKey},
		{"google api key", `apiKey: "` + googleKey + `"`, "google-api-key", googleKey},
		{"google oauth secret", `"client_secret": "` + googleSecret + `"`, "google-oauth-client-secret", googleSecret},
		{"stripe live key", "STRIPE_SECRET_KEY=" + stripeLive, "stripe-secret-key", stripeLive},
		{"stripe test key", "secretKey: '" + stripeTest + "',", "stripe-secret-key", stripeTest},
		{"stripe restricted key", `"` + stripeRK + `"`, "stripe-secret-key", stripeRK},
		{"stripe webhook secret", "webhookSecret: '" + stripeHook + "',", "stripe-webhook-secret", stripeHook},
		{"slack webhook", "'" + slackHook + "',", "slack-webhook-url", slackHook},
		{"slack bot token", "SLACK_BOT_TOKEN=" + slackBot, "slack-token", slackBot},
		{"postgres password", "url: 'postgres://shop_admin:" + dbPassword + "@db.internal.example.com:5432/shop',", "database-password", dbPassword},
		{"postgresql password", "DATABASE_URL=postgresql://app:" + dbPassword + "@localhost/app", "database-password", dbPassword},
		{"mysql password", `dsn = "mysql://root:` + dbPassword + `@tcp/app"`, "database-password", dbPassword},
		{"mongodb srv password", `"mongodb+srv://admin:` + dbPassword + `@cluster0.example.net/db"`, "database-password", dbPassword},
		{"redis password", "REDIS_URL=rediss://default:" + dbPassword + "@cache:6380", "database-password", dbPassword},
		{"amqp password", "amqp://guest:" + dbPassword + "@rabbit:5672/", "database-password", dbPassword},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matches := Mask(tt.in)
			if strings.Contains(got, tt.secret) {
				t.Fatalf("Mask(%q) = %q, still holds the secret", tt.in, got)
			}
			want := strings.Replace(tt.in, tt.secret, "[REDACTED:"+tt.kind+"]", 1)
			if got != want {
				t.Errorf("Mask(%q) = %q, want %q", tt.in, got, want)
			}
			if len(matches) != 1 || matches[0].Kind != tt.kind {
				t.Errorf("matches = %+v, want one %s", matches, tt.kind)
			}
		})
	}
}

func TestMaskLeavesLookalikes(t *testing.T) {
	for _, in := range []string{
		"AKIA is the prefix of an AWS access key id",
		"AKIAIOSFODNN7EXAMPL",         // one short
		"xAKIAIOSFODNN7EXAMPLEx",      // inside a longer word
		"ghp_short",                   // too short for a token
		"sk-ant-",                     // just the prefix
		"risk-proj-planning-for-2026", // "sk-proj-" inside a word
		"task-admin-dashboard",
		"AIzaShort",
		"sha := \"0123456789abcdef0123456789abcdef01234567\"", // 40 hex chars, no AWS name
		"secret := strings.Repeat(\"a\", 40)",
		"// -----BEGIN PUBLIC KEY-----",
		"-----BEGIN CERTIFICATE-----",
		"const pemHeader = \"-----BEGIN\"",
		"[REDACTED:github-token]",
		"pk_live_51AbCdEfGhIj0Kl1Mn2Ij0Kl1Mn2Ij0", // publishable, not secret
		"sk_test_short",
		"task_live_session_handler_for_the_dashboard_x", // "sk_live_" inside a word
		"https://hooks.slack.com/services/",
		"postgres://localhost:5432/app",               // no credentials
		"postgres://app@localhost/app",                // a user, no password
		"postgres://app:${DB_PASSWORD}@localhost/app", // filled in at runtime
		"postgres://app:$DB_PASSWORD@localhost/app",
		"postgres://user:<password>@host/db",
		"postgres://user:password@host/db", // the docs' placeholder
		"mysql://user:****@host/db",
		"https://user:pass@example.com/", // not a database URL
	} {
		if got, matches := Mask(in); got != in || len(matches) != 0 {
			t.Errorf("Mask(%q) = %q, %+v; want it unchanged", in, got, matches)
		}
	}
}

func TestMaskPrivateKeyInNumberedDiff(t *testing.T) {
	in := strings.Join([]string{
		"diff --git a/deploy/id_rsa b/deploy/id_rsa",
		"+++ b/deploy/id_rsa",
		"@@ -0,0 +1,4 @@",
		"     1 +" + pemBegin,
		"     2 +" + pemBody1,
		"     3 +" + pemBody2,
		"     4 +" + pemEnd,
		"diff --git a/main.go b/main.go",
		"+++ b/main.go",
		"@@ -1,1 +1,1 @@",
		"     1 +func ParseConfigurationFromEnvironment() {}",
	}, "\n")
	got, matches := Mask(in)
	want := strings.Join([]string{
		"diff --git a/deploy/id_rsa b/deploy/id_rsa",
		"+++ b/deploy/id_rsa",
		"@@ -0,0 +1,4 @@",
		"     1 +[REDACTED:private-key]",
		"     2 +[REDACTED:private-key]",
		"     3 +[REDACTED:private-key]",
		"     4 +[REDACTED:private-key]",
		"diff --git a/main.go b/main.go",
		"+++ b/main.go",
		"@@ -1,1 +1,1 @@",
		"     1 +func ParseConfigurationFromEnvironment() {}",
	}, "\n")
	if got != want {
		t.Errorf("Mask() =\n%s\nwant\n%s", got, want)
	}
	if len(matches) != 1 || matches[0].Kind != "private-key" || matches[0].File != "deploy/id_rsa" {
		t.Errorf("matches = %+v, want one private-key in deploy/id_rsa", matches)
	}
}

func TestMaskPrivateKeyInFileExcerpt(t *testing.T) {
	in := "### keys/server.pem\n\n1\t" + pemBegin + "\n2\t" + pemBody1 + "\n3\t" + pemEnd + "\n"
	got, matches := Mask(in)
	want := "### keys/server.pem\n\n1\t[REDACTED:private-key]\n2\t[REDACTED:private-key]\n3\t[REDACTED:private-key]\n"
	if got != want {
		t.Errorf("Mask() = %q, want %q", got, want)
	}
	if len(matches) != 1 || matches[0].File != "keys/server.pem" {
		t.Errorf("matches = %+v, want one in keys/server.pem", matches)
	}
}

func TestMaskEscapedPrivateKeyOnOneLine(t *testing.T) {
	in := `+  "private_key": "` + pemBegin + `\n` + pemBody1 + `\n` + pemEnd + `\n",`
	got, _ := Mask(in)
	want := `+  "private_key": "[REDACTED:private-key]\n",`
	if got != want {
		t.Errorf("Mask() = %q, want %q", got, want)
	}
}

func TestMaskUnterminatedPrivateKeyStops(t *testing.T) {
	// A diff cut off inside a key: the next file's code is left alone.
	in := strings.Join([]string{
		"+" + pemBegin,
		"+" + pemBody1,
		"diff --git a/main.go b/main.go",
		"+func ParseConfigurationFromEnvironment() {}",
	}, "\n")
	got, _ := Mask(in)
	if !strings.HasSuffix(got, "diff --git a/main.go b/main.go\n+func ParseConfigurationFromEnvironment() {}") {
		t.Errorf("Mask() = %q, masked past the end of the file", got)
	}
	if strings.Contains(got, pemBody1) {
		t.Errorf("Mask() = %q, left the key's body", got)
	}
}

func TestMaskKeepsLineCount(t *testing.T) {
	in := strings.Join([]string{
		"+" + pemBegin, "+" + pemBody1, "+" + pemEnd,
		"+aws_access_key_id = " + awsKeyID,
		`+  "private_key": "` + pemBegin + `\n` + pemBody1 + `\n` + pemEnd + `\n",`,
	}, "\n") + "\n"
	got, _ := Mask(in)
	if strings.Count(got, "\n") != strings.Count(in, "\n") {
		t.Errorf("Mask() changed the line count:\n%s", got)
	}
}

func TestMaskIsIdempotent(t *testing.T) {
	in := "a " + awsKeyID + " b " + ghToken + "\n+" + pemBegin + "\n+" + pemBody1 + "\n+" + pemEnd + "\n"
	once, _ := Mask(in)
	twice, matches := Mask(once)
	if twice != once || len(matches) != 0 {
		t.Errorf("Mask(Mask(s)) = %q, %+v; want %q and no matches", twice, matches, once)
	}
}

func TestMaskSameSecretSameID(t *testing.T) {
	_, a := Mask("x " + awsKeyID)
	_, b := Mask("y = \"" + awsKeyID + "\"")
	_, c := Mask("AKIA" + "IOSFODNN7EXAMPLF")
	if a[0].ID != b[0].ID {
		t.Errorf("same secret, different IDs: %q, %q", a[0].ID, b[0].ID)
	}
	if a[0].ID == c[0].ID {
		t.Errorf("different secrets, same ID %q", a[0].ID)
	}
	if strings.Contains(a[0].ID, "IOSFODNN") {
		t.Errorf("ID %q holds part of the secret", a[0].ID)
	}
}

func TestMasked(t *testing.T) {
	if !Masked(`key := "[REDACTED:aws-access-key-id]"`) {
		t.Error("Masked() = false for a placeholder")
	}
	if Masked(`key := os.Getenv("AWS_ACCESS_KEY_ID")`) {
		t.Error("Masked() = true without a placeholder")
	}
}

// The review sees a key in the diff and the check sees it in the file, with
// line numbers: it's the same key, so it's warned about once.
func TestMaskPrivateKeySameIDInDiffAndFile(t *testing.T) {
	inDiff := "+++ b/config.ts\n@@ -0,0 +1,3 @@\n    30 +export const privateKey = `" + pemBegin + "\n    31 +" + pemBody1 + "\n    32 +" + pemEnd + "`;\n"
	inFile := "### config.ts\n\n30\texport const privateKey = `" + pemBegin + "\n31\t" + pemBody1 + "\n32\t" + pemEnd + "`;\n"
	_, a := Mask(inDiff)
	_, b := Mask(inFile)
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("matches = %+v, %+v; want one each", a, b)
	}
	if a[0].ID != b[0].ID {
		t.Errorf("IDs differ: %q in the diff, %q in the file", a[0].ID, b[0].ID)
	}
	_, other := Mask("+" + pemBegin + "\n+" + pemBody2 + "\n+" + pemEnd + "\n")
	if other[0].ID == a[0].ID {
		t.Error("a different key has the same ID")
	}
}

// A key's last line of base64 is often short. It's masked too, in every
// format the key reaches a prompt in, and line numbers stay.
func TestMaskPrivateKeyShortLastLine(t *testing.T) {
	const tail = "Zx4="
	tests := []struct{ name, in, want string }{
		{"raw diff",
			"+" + pemBegin + "\n+" + pemBody1 + "\n+" + tail + "\n+" + pemEnd,
			"+[REDACTED:private-key]\n+[REDACTED:private-key]\n+[REDACTED:private-key]\n+[REDACTED:private-key]"},
		{"numbered diff past line 1000",
			"  1233 +" + pemBegin + "\n  1234 +" + pemBody1 + "\n  1235 +" + tail + "\n  1236 +" + pemEnd,
			"  1233 +[REDACTED:private-key]\n  1234 +[REDACTED:private-key]\n  1235 +[REDACTED:private-key]\n  1236 +[REDACTED:private-key]"},
		{"removed lines",
			"       -" + pemBegin + "\n       -" + tail + "\n       -" + pemEnd,
			"       -[REDACTED:private-key]\n       -[REDACTED:private-key]\n       -[REDACTED:private-key]"},
		{"file excerpt",
			"1233\t" + pemBegin + "\n1234\t" + tail + "\n1235\t" + pemEnd,
			"1233\t[REDACTED:private-key]\n1234\t[REDACTED:private-key]\n1235\t[REDACTED:private-key]"},
		{"string concatenation",
			"key := \"" + pemBegin + "\\n\" +\n\t\"" + tail + "\\n\" +\n\t\"" + pemEnd + "\"",
			"key := \"[REDACTED:private-key]\n\t\"[REDACTED:private-key]\\n\" +\n\t\"[REDACTED:private-key]\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matches := Mask(tt.in)
			if got != tt.want {
				t.Errorf("Mask() =\n%s\nwant\n%s", got, tt.want)
			}
			if len(matches) != 1 {
				t.Errorf("matches = %+v, want one key", matches)
			}
		})
	}
}

// Line numbers aren't part of a key's ID, past line 1000 too.
func TestMaskPrivateKeySameIDPastLine1000(t *testing.T) {
	_, a := Mask("  1234 +" + pemBegin + "\n  1235 +" + pemBody1 + "\n  1236 +" + pemEnd)
	_, b := Mask("5678\t" + pemBegin + "\n5679\t" + pemBody1 + "\n5680\t" + pemEnd)
	if a[0].ID != b[0].ID {
		t.Errorf("IDs differ: %q, %q", a[0].ID, b[0].ID)
	}
}
