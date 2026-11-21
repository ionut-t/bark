package findings

import (
	"regexp"
	"strings"
)

// severityTag is a bracketed tag a model puts before a title that starts
// with a severity word, such as "[major]" or "[HIGH · bug]".
var severityTag = regexp.MustCompile(`(?i)^[\[(]\s*(critical|high|medium|low|major|minor|blocker|nit|nitpick|trivial|info|warning)\b[^\])]*[\])]\s*`)

// tag is any bracketed tag before a title.
var tag = regexp.MustCompile(`^[\[(][^\])]+[\])]\s*`)

// CleanTitle removes the severity tags a model sometimes puts at the start
// of a finding's title: Bark's heading already shows the severity. Once a
// severity tag is removed, the tags right after it go too, as a category
// usually follows it. A title that is nothing but tags is kept.
func CleanTitle(title string) string {
	title = strings.TrimSpace(title)
	rest := severityTag.ReplaceAllString(title, "")
	if rest == title {
		return title
	}
	for {
		next := tag.ReplaceAllString(rest, "")
		if next == rest {
			break
		}
		rest = next
	}
	if rest == "" {
		return title
	}
	return rest
}
