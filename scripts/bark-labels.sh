#!/usr/bin/env bash
# Copy Bark's "bark: …" pull request labels, with their colours and
# descriptions, from this repository to other GitHub repositories.
#
# Usage:
#   scripts/bark-labels.sh [--from owner/repo] [--dry-run] owner/repo [owner/repo...]
#
# Labels that already exist on a target are updated to match the source.
# Labels the target has that the source doesn't are left alone.
# Requires the GitHub CLI (gh), signed in with write access to the targets.
set -euo pipefail

source_repo="ionut-t/bark"
dry_run=false
targets=()

usage() {
	sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
	exit "${1:-0}"
}

while (($#)); do
	case "$1" in
	--from)
		[[ $# -ge 2 ]] || usage 1
		source_repo="$2"
		shift 2
		;;
	--dry-run)
		dry_run=true
		shift
		;;
	-h | --help) usage ;;
	-*)
		echo "unknown option: $1" >&2
		usage 1
		;;
	*)
		targets+=("$1")
		shift
		;;
	esac
done

((${#targets[@]})) || usage 1
command -v gh >/dev/null || {
	echo "gh (GitHub CLI) is required" >&2
	exit 1
}

# name<TAB>color<TAB>description, one label per line.
labels=$(gh label list --repo "$source_repo" --limit 1000 \
	--json name,color,description \
	--jq '.[] | select(.name | startswith("bark: ")) | [.name, .color, .description] | @tsv')

[[ -n "$labels" ]] || {
	echo "no \"bark: \" labels found on $source_repo" >&2
	exit 1
}

count=$(wc -l <<<"$labels" | tr -d ' ')
echo "Copying $count labels from $source_repo"

failed=0
for target in "${targets[@]}"; do
	echo "→ $target"
	while IFS=$'\t' read -r name color description; do
		if $dry_run; then
			printf '  would set %-30s #%s  %s\n' "$name" "$color" "$description"
			continue
		fi
		if gh label create "$name" --repo "$target" --color "$color" \
			--description "$description" --force >/dev/null; then
			printf '  ✓ %s\n' "$name"
		else
			printf '  ✗ %s\n' "$name" >&2
			failed=1
		fi
	done <<<"$labels"
done

exit "$failed"
