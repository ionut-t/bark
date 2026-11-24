#!/usr/bin/env bash
# Prints the reviewer's name for the "Reviewed by" label, spelled as its
# file in Bark's config directory spells it: "guido van rossum" gives
# "Guido van Rossum". The file is found the way `bark review --as` finds
# it: the first one, by file name, whose name contains the given name,
# ignoring case. Run it after Bark, which puts the reviewers there.
#
# Without a matching file, the given name is printed with each word
# capitalised. No reviewer, or a reviewer file path, gives "Bark".
#
# Usage: reviewer-label-name.sh [reviewer]
set -euo pipefail

REVIEWER=${1:-}
if [ -z "$REVIEWER" ] || [[ "$REVIEWER" == */* ]] || [[ "${REVIEWER,,}" == *.md ]]; then
  echo "Bark"
  exit 0
fi

# Byte order, as Bark reads the directory.
LC_ALL=C
shopt -s nullglob
WANT=${REVIEWER,,}
for FILE in "${XDG_CONFIG_HOME:-$HOME/.config}"/bark/reviewers/*; do
  [ -f "$FILE" ] || continue
  NAME=$(basename "$FILE")
  NAME=${NAME%.*}
  if [[ "${NAME,,}" == *"$WANT"* ]]; then
    echo "$NAME"
    exit 0
  fi
done

read -ra WORDS <<< "$REVIEWER"
echo "${WORDS[*]^}"
