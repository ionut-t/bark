#!/usr/bin/env bash
# Records assets/demo.gif with VHS. Re-runnable.
#
# Builds bark with the "demo" build tag, which adds an offline mock LLM
# (enabled by BARK_DEMO_MOCK=1), so the recording makes no network calls
# and needs no API keys. Everything runs against a throwaway git repo and
# an isolated HOME under demo/.tmp, which is removed afterwards.
#
# Set KEEP_TMP=1 to keep demo/.tmp for debugging.
set -euo pipefail

ORIG_HOME="$HOME"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$ROOT/demo/.tmp"
REPO="$TMP/sample-repo"
DEMO_HOME="$TMP/home"

for bin in go git vhs; do
	command -v "$bin" >/dev/null || { echo "error: $bin not found in PATH" >&2; exit 1; }
done

cleanup() {
	if [[ "${KEEP_TMP:-0}" != "1" ]]; then
		rm -rf "${TMP:?}"
	fi
}
trap cleanup EXIT

rm -rf "${TMP:?}"
mkdir -p "$TMP/bin" "$DEMO_HOME/.bark" "$TMP/xdg" "$REPO"

echo "==> Building bark (demo build)"
(cd "$ROOT" && go build -tags demo -o "$TMP/bin/bark" .)

# Pre-create the config so bark doesn't print "Created config at <path>".
cat >"$DEMO_HOME/.bark/.config.toml" <<'TOML'
editor = 'vim'
llm_provider = ''
llm_model = 'gemini-2.5-pro'
max_diff_lines = 0
relative_number = false
context_enrichment = false
TOML

echo "==> Building sample repo"
# Isolate git from the real user's global/system config.
export HOME="$DEMO_HOME" XDG_CONFIG_HOME="$TMP/xdg"
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
export GIT_AUTHOR_NAME="Demo User" GIT_AUTHOR_EMAIL="demo@example.com"
export GIT_COMMITTER_NAME="Demo User" GIT_COMMITTER_EMAIL="demo@example.com"
export GIT_AUTHOR_DATE="2026-01-15T10:00:00Z" GIT_COMMITTER_DATE="2026-01-15T10:00:00Z"

cd "$REPO"
git init -q -b main
git config user.name "Demo User"
git config user.email "demo@example.com"

cat >go.mod <<'GO'
module example.com/users

go 1.23
GO

cat >handler.go <<'GO'
package users

import (
	"encoding/json"
	"net/http"
)

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Server struct {
	store Store
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	user, err := s.store.Create(r.Context(), req.Name, req.Email)
	if err != nil {
		http.Error(w, "could not create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}
GO

cat >store.go <<'GO'
package users

import "context"

type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Store interface {
	Create(ctx context.Context, name, email string) (User, error)
}
GO

git add -A
git commit -q -m "feat: add CreateUser handler"

# The staged change: input validation.
cat >handler.go <<'GO'
package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
)

const maxBodyBytes = 1 << 20

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (req *CreateUserRequest) Validate() error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return errors.New("name is required")
	}
	if len(req.Name) > 100 {
		return errors.New("name must be at most 100 characters")
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return errors.New("email is invalid")
	}
	return nil
}

type Server struct {
	store Store
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	user, err := s.store.Create(r.Context(), req.Name, req.Email)
	if err != nil {
		http.Error(w, "could not create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}
GO
git add handler.go

echo "==> Recording"
cd "$ROOT"
# Strip any provider credentials from the recording environment.
env -u GEMINI_API_KEY -u OPENAI_API_KEY -u ANTHROPIC_API_KEY \
	-u VERTEXAI_PROJECT_ID -u VERTEXAI_LOCATION -u OLLAMA_HOST -u XDG_CONFIG_HOME \
	HOME="$ORIG_HOME" vhs demo/demo.tape

echo "==> Wrote assets/demo.gif"
