#!/usr/bin/env bash
# Prints the reviewer a pull request's "bark: as <name>" label picks (for
# example "bark: as Yoda"), or nothing when it has no such label. The label
# stays on the pull request, so every review of it uses that reviewer.
#
# The labels are read from GitHub, not the event payload: labels added with
# `gh pr create --label` land after the "opened" event is sent.
#
# The name goes to `bark review --as`, which also reads a file path, so only
# a plain name is accepted: letters, digits, spaces, hyphens and dots (as in
# "Dr. House"), not ending in .md, and nothing that exists in the checkout.
# Case doesn't matter. More than one such label is an error rather than a
# guess.
#
# Needs GH_TOKEN, REPO (owner/name) and PR_NUMBER.
set -euo pipefail

LABELS=$(gh pr view "$PR_NUMBER" --repo "$REPO" --json labels --jq '.labels[].name')

NAMES=()
while IFS= read -r LABEL; do
  LABEL=${LABEL,,}
  case "$LABEL" in
    "bark: as "*)
      NAME=${LABEL#bark: as }
      # Trim the spaces around the name.
      NAME=${NAME#"${NAME%%[![:space:]]*}"}
      NAME=${NAME%"${NAME##*[![:space:]]}"}
      NAMES+=("$NAME")
      ;;
  esac
done <<< "$LABELS"

case ${#NAMES[@]} in
  0) exit 0 ;;
  1) ;;
  *)
    echo "::error::The pull request has more than one \"bark: as\" label ($(IFS=,; echo "${NAMES[*]}")); keep one." >&2
    exit 1
    ;;
esac

NAME=${NAMES[0]}
if ! [[ "$NAME" =~ ^[a-z0-9][a-z0-9\ .-]*$ ]] || [[ "$NAME" == *.md ]] || [ -e "$NAME" ]; then
  echo "::error::\"bark: as $NAME\" doesn't name a reviewer: use a built-in reviewer's name, such as \"bark: as Yoda\"." >&2
  exit 1
fi

echo "$NAME"
