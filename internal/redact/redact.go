// Package redact masks secrets in text before it's sent to a model
// provider: API keys of the common providers and private keys, found by
// their shape. It makes no model call. A masked secret becomes a
// placeholder that names its kind, such as [REDACTED:github-token], so the
// model can still point out that a secret is hard-coded.
//
// Masking never adds or removes a line, so line numbers in a masked diff or
// file still hold, and masked text masks to itself.
package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// Match is one secret Mask found.
type Match struct {
	// Kind names the secret's shape, such as "aws-access-key-id".
	Kind string
	// File is the file the secret is in, when the text says (a diff's
	// "+++ b/" line or a "### path" heading before it); empty otherwise.
	File string
	// ID tells secrets apart without holding them: a short hash of the
	// secret, the same wherever it appears.
	ID string
	// Line is the index of the line of the text the secret starts on,
	// counting from 0.
	Line int
}

const placeholderPrefix = "[REDACTED:"

func placeholder(kind string) string { return placeholderPrefix + kind + "]" }

// Masked reports whether s holds a placeholder Mask wrote.
func Masked(s string) bool { return strings.Contains(s, placeholderPrefix) }

// pattern finds one kind of secret. The secret is the regexp's first
// group; the rest of the match is context that stays (the name it's
// assigned to, or the character after it, which RE2 can't look ahead at).
type pattern struct {
	kind string
	re   *regexp.Regexp
	// skip, if not nil, leaves a match alone: a placeholder, not a secret.
	skip func(secret string) bool
}

// patterns are tried in order on each line. Anthropic's keys start with
// "sk-" too, so they come before OpenAI's.
var patterns = []pattern{
	{"aws-access-key-id", regexp.MustCompile(`\b((?:AKIA|ASIA|ABIA|ACCA)[A-Z0-9]{16})\b`), nil},
	// Forty base64 characters are too common to mask on their own (a SHA-1
	// in hex is forty characters), so only next to the name of the key.
	{"aws-secret-access-key", regexp.MustCompile(`(?i)(?:aws.{0,20}?secret|secret_?access_?key).{0,20}?[:=>]\s*['"]?([A-Za-z0-9/+]{40})(?:[^A-Za-z0-9/+=]|$)`), nil},
	{"github-token", regexp.MustCompile(`\b((?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{36,255})\b`), nil},
	{"github-token", regexp.MustCompile(`\b(github_pat_[A-Za-z0-9_]{50,255})\b`), nil},
	{"anthropic-api-key", regexp.MustCompile(`\b(sk-ant-[a-z0-9]+-[A-Za-z0-9_-]{32,})`), nil},
	{"openai-api-key", regexp.MustCompile(`\b(sk-(?:proj|svcacct|admin)-[A-Za-z0-9_-]{32,})`), nil},
	{"openai-api-key", regexp.MustCompile(`\b(sk-[A-Za-z0-9_-]*T3BlbkFJ[A-Za-z0-9_-]+)`), nil},
	{"google-api-key", regexp.MustCompile(`\b(AIza[0-9A-Za-z_-]{35})(?:[^0-9A-Za-z_-]|$)`), nil},
	{"google-oauth-client-secret", regexp.MustCompile(`\b(GOCSPX-[A-Za-z0-9_-]{28})(?:[^A-Za-z0-9_-]|$)`), nil},
	// Secret and restricted keys; publishable keys (pk_) are public.
	{"stripe-secret-key", regexp.MustCompile(`\b((?:sk|rk)_(?:live|test)_[A-Za-z0-9]{20,})`), nil},
	{"stripe-webhook-secret", regexp.MustCompile(`\b(whsec_[A-Za-z0-9]{24,})`), nil},
	{"slack-webhook-url", regexp.MustCompile(`(https://hooks\.slack\.com/(?:services|workflows|triggers)/[A-Za-z0-9_/-]{20,})`), nil},
	{"slack-token", regexp.MustCompile(`\b(xox[abposr]-[A-Za-z0-9-]{20,})`), nil},
	// The password in a database or broker URL. One that's filled in at
	// runtime (${DB_PASSWORD}, $PASS), or the docs' "password", isn't a
	// secret.
	{
		"database-password", regexp.MustCompile(`\b(?:postgres(?:ql)?|mysql|mariadb|mongodb(?:\+srv)?|rediss?|amqps?|sqlserver|mssql|cockroachdb)://[^:/@\s'"]*:([^@\s'"/]+)@`),
		isPlaceholderPassword,
	},
}

// isPlaceholderPassword reports whether a password in a URL stands for one
// rather than being one.
func isPlaceholderPassword(p string) bool {
	if strings.ContainsAny(p[:1], "$<{%*") || strings.Trim(p, "*x") == "" {
		return true
	}
	switch strings.ToLower(p) {
	case "password", "pass", "passwd", "pwd", "secret", "changeme":
		return true
	}
	return false
}

const privateKey = "private-key"

var (
	pemBeginRE = regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`)
	pemEndRE   = regexp.MustCompile(`-----END (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-----`)
	// pemBodyRE is a run of base64 in a key's body. It starts with a letter
	// or digit so a diff's "+" before it stays.
	pemBodyRE = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9+/=]{15,}`)
)

// maxPEMLines bounds how far a private key without an END line is masked.
// A 4096-bit RSA key is about 50 lines.
const maxPEMLines = 120

// Mask returns s with every secret it finds replaced by a placeholder, and
// the secrets it found (a private key once, however many lines it spans).
func Mask(s string) (string, []Match) {
	if s == "" {
		return s, nil
	}
	lines := strings.Split(s, "\n")
	var matches []Match
	var file string
	// pem is set inside a private key whose BEGIN line was masked: the
	// match being built, the key's text so far (for its ID), and the
	// lines left before giving up on its END line.
	var pem *Match
	var pemText strings.Builder
	pemLeft := 0
	endPEM := func() {
		pem.ID = id(pemText.String())
		matches = append(matches, *pem)
		pem = nil
		pemText.Reset()
	}

	for i, l := range lines {
		f, isFile := fileOf(l)
		if isFile {
			file = f
		}
		if isFile || hunkLine.MatchString(l) {
			if pem != nil {
				// A new file or hunk: the key ended without an END line
				// (a diff cut short, say).
				endPEM()
			}
		}

		if pem != nil {
			if loc := pemEndRE.FindStringIndex(l); loc != nil {
				writeKey(&pemText, l[:loc[0]])
				l = maskBody(l[:loc[0]]) + placeholder(privateKey) + l[loc[1]:]
				endPEM()
			} else {
				writeKey(&pemText, l)
				l = maskBody(l)
				if pemLeft--; pemLeft == 0 {
					endPEM()
				}
			}
		} else if loc := pemBeginRE.FindStringIndex(l); loc != nil {
			m := Match{Kind: privateKey, File: file, Line: i}
			if end := pemEndRE.FindStringIndex(l[loc[1]:]); end != nil {
				// The whole key on one line, its newlines escaped (as in
				// a JSON credentials file).
				stop := loc[1] + end[1]
				m.ID = id(l[loc[0]:stop])
				matches = append(matches, m)
				l = l[:loc[0]] + placeholder(privateKey) + l[stop:]
			} else {
				pem, pemLeft = &m, maxPEMLines
				pemText.WriteString(l[loc[0]:loc[1]])
				writeKey(&pemText, l[loc[1]:])
				// Anything after the header (an escaped key cut by a
				// line break, say) is part of the key.
				l = l[:loc[0]] + placeholder(privateKey)
			}
		}

		for _, p := range patterns {
			l = maskPattern(l, p, Match{Kind: p.kind, File: file, Line: i}, &matches)
		}
		lines[i] = l
	}
	if pem != nil {
		endPEM()
	}
	return strings.Join(lines, "\n"), matches
}

// maskPattern masks p's secrets in l, adding a copy of m for each to
// matches.
func maskPattern(l string, p pattern, m Match, matches *[]Match) string {
	locs := p.re.FindAllStringSubmatchIndex(l, -1)
	if locs == nil {
		return l
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		start, end := loc[2], loc[3]
		if p.skip != nil && p.skip(l[start:end]) {
			continue
		}
		b.WriteString(l[last:start])
		b.WriteString(placeholder(p.kind))
		m.ID = id(l[start:end])
		*matches = append(*matches, m)
		last = end
	}
	b.WriteString(l[last:])
	return b.String()
}

// writeKey adds the base64 in l, a line of a private key's body, to b. Only
// the key goes into its ID, not the diff markers or line numbers around it,
// so the key has the same ID in a diff and in a file.
func writeKey(b *strings.Builder, l string) {
	for _, run := range pemBodyRE.FindAllString(l, -1) {
		b.WriteString(run)
	}
}

// maskBody masks the base64 in a line of a private key's body.
func maskBody(l string) string {
	return pemBodyRE.ReplaceAllLiteralString(l, placeholder(privateKey))
}

// hunkLine is a diff's hunk header; a private key's body doesn't run past
// one.
var hunkLine = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// fileOf returns the file a line starts, if it's a diff's file header or
// a "### path" heading (the check's files and related code), and whether
// it is.
func fileOf(l string) (string, bool) {
	switch {
	case strings.HasPrefix(l, "+++ "):
		f := strings.TrimPrefix(l, "+++ ")
		if f == "/dev/null" {
			return "", true
		}
		return strings.TrimPrefix(f, "b/"), true
	case strings.HasPrefix(l, "diff --git "):
		if i := strings.LastIndex(l, " b/"); i >= 0 {
			return l[i+3:], true
		}
		return "", true
	case strings.HasPrefix(l, "### "):
		f, _, _ := strings.Cut(strings.TrimPrefix(l, "### "), " ")
		// Related code is headed "path:start-end (label)".
		if i := strings.LastIndexByte(f, ':'); i > 0 && strings.Trim(f[i+1:], "0123456789-") == "" {
			f = f[:i]
		}
		return f, true
	}
	return "", false
}

func id(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:6])
}
