package cmd

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// These mirror the layout fang uses for the help it prints.
const (
	// maxHelpWidth is the widest fang lets help get, however wide the
	// terminal is.
	maxHelpWidth = 120
	// helpIndent is how far fang indents a flag or command name.
	helpIndent = 4
	// minKeyColumn is the narrowest fang makes the name column.
	minKeyColumn = 10
	// keyGap is the room fang leaves after the widest name.
	keyGap = 2
	// minDescriptionWidth keeps a description readable in a very narrow
	// terminal, even if its lines then run past the edge.
	minDescriptionWidth = 30
)

// helpWidth returns the width fang lays help out in: the terminal's, up to
// maxHelpWidth, or maxHelpWidth when stdout isn't a terminal.
func helpWidth() int {
	// fang's own override, read here too so the two never disagree.
	if s := os.Getenv("__FANG_TEST_WIDTH"); s != "" {
		if w, err := strconv.Atoi(s); err == nil {
			return w
		}
	}
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return maxHelpWidth
	}
	return min(w, maxHelpWidth)
}

// wrapFlagUsages breaks every flag description and subcommand summary in the
// tree into lines that fit width once fang has indented them under their own
// column. fang prints each on a single line however long it is, but keeps
// the lines of one that already has them aligned. A flag several commands
// share, such as --config, is wrapped for the narrowest column it shows up
// in.
//
// Each flag description also ends in a blank line, so that the flags stand
// apart once some take several lines. fang prints a flag's default after its
// description, which would put it on that blank line, so the default moves
// into the description and wraps with it. The flag a command lists last gets
// no blank line, as fang ends the help with one; a flag that is last in some
// commands but not others, which only a shared flag can be, keeps it.
func wrapFlagUsages(root *cobra.Command, width int) {
	limits := map[*pflag.Flag]int{}
	shown := map[*pflag.Flag]int{}
	last := map[*pflag.Flag]int{}
	var visit func(c *cobra.Command)
	visit = func(c *cobra.Command) {
		// Pull the parents' persistent flags into c.Flags(), as cobra does
		// before it prints help, so they are laid out with the rest.
		c.LocalFlags()
		// cobra adds --help and --version only when it runs; add them now
		// so they are spaced like the rest.
		c.InitDefaultHelpFlag()
		c.InitDefaultVersionFlag()
		room := width - helpIndent - keyColumn(c)
		for _, sub := range c.Commands() {
			// fang heads a command's own help with its Long, or its Short
			// when it has none; keep that one on full-width lines.
			if sub.Long == "" {
				sub.Long = sub.Short
			}
			sub.Short = wrapWords(sub.Short, max(room, minDescriptionWidth))
		}
		var final *pflag.Flag
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			limit := max(room, minDescriptionWidth)
			if old, ok := limits[f]; !ok || limit < old {
				limits[f] = limit
			}
			shown[f]++
			final = f
		})
		if final != nil {
			last[final]++
		}
		for _, sub := range c.Commands() {
			visit(sub)
		}
	}
	visit(root)

	for f, limit := range limits {
		usage := f.Usage
		if showsDefault(f) {
			usage += " (" + f.DefValue + ")"
			f.DefValue = ""
		}
		f.Usage = wrapWords(usage, limit)
		if last[f] < shown[f] {
			f.Usage += "\n"
		}
	}
}

// wrapWords breaks each line of s at spaces so that none is wider than limit,
// unless a single word is. Unlike ansi.Wordwrap it never breaks after a
// hyphen, which would split a flag name such as --comment.
func wrapWords(s string, limit int) string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		var cur string
		for word := range strings.FieldsSeq(line) {
			switch {
			case cur == "":
				cur = word
			case ansi.StringWidth(cur)+1+ansi.StringWidth(word) <= limit:
				cur += " " + word
			default:
				out = append(out, cur)
				cur = word
			}
		}
		out = append(out, cur)
	}
	return strings.Join(out, "\n")
}

// keyColumn returns how wide fang makes the name column in c's help: wide
// enough for its longest flag and subcommand name, plus a gap.
func keyColumn(c *cobra.Command) int {
	widest := 0
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		key := "--" + f.Name
		if f.Shorthand != "" {
			key = "-" + f.Shorthand + " " + key
		}
		widest = max(widest, len(key))
	})
	if c.HasAvailableSubCommands() && !c.HasParent() {
		widest = max(widest, len("help [command]")) // cobra adds help too
	}
	for _, sub := range c.Commands() {
		if !sub.Hidden {
			widest = max(widest, commandKeyWidth(sub))
		}
	}
	return max(widest+keyGap, minKeyColumn)
}

// commandKeyWidth returns how wide fang prints a subcommand in its parent's
// list: its use line, plus [command] when it has subcommands of its own and
// [--flags], which every command takes since the root has --config.
func commandKeyWidth(c *cobra.Command) int {
	use := c.Use
	for _, tag := range []string{"[flags]", "[--flags]", "[command]"} {
		use = strings.ReplaceAll(use, tag, "")
	}
	w := len(strings.Join(strings.Fields(use), " "))
	if c.HasAvailableSubCommands() {
		w += len(" [command]")
	}
	return w + len(" [--flags]")
}

// showsDefault reports whether fang prints f's default after its
// description.
func showsDefault(f *pflag.Flag) bool {
	switch f.DefValue {
	case "", "false", "0", "[]":
		return false
	}
	return true
}
