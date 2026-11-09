package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/prices"
	"github.com/spf13/cobra"
)

const pricesTimeout = 2 * time.Minute

func pricesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prices",
		Short: "Download model prices so Bark can show what each run costs",
		Long: `Download model prices from models.dev and save the ones for Bark's providers.

With a prices file, review, commit and pr print what their model calls cost on
stderr. Bark reads the repo's .bark/prices.json first, then ~/.bark/prices.json;
it never downloads prices on its own. Commit .bark/prices.json (bark prices
--repo) to show costs in CI. Run it again to update them.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			repo, _ := cmd.Flags().GetBool("repo")
			if repo {
				output = prices.RepoPath
				if warning := repoWarning(cmd.Context()); warning != "" {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "bark: %s\n", warning)
				}
			}
			if output == "" {
				storage, err := config.GetStorage()
				if err != nil {
					return fmt.Errorf("error getting storage: %w", err)
				}
				output = filepath.Join(storage, prices.FileName)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), pricesTimeout)
			defer cancel()
			return downloadPrices(ctx, http.DefaultClient, prices.SourceURL, output, time.Now(), cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringP("output", "o", "", "Where to save the prices (default is $HOME/.bark/prices.json)")
	// No -r shorthand: reset uses it for --reviewers.
	cmd.Flags().Bool("repo", false, "Save the prices to the repo's .bark/prices.json, which Bark reads first")
	cmd.MarkFlagsMutuallyExclusive("output", "repo")

	return cmd
}

// repoWarning says why prices saved to the current directory's .bark may
// not be read: Bark looks for .bark/ in the directory it runs from, which is
// usually the repo's top level. It is empty when the current directory is
// the top level.
func repoWarning(ctx context.Context) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, err := git.RepoRoot(ctx)
	return topLevelWarning(cwd, root, err)
}

// topLevelWarning is repoWarning for a working directory cwd and the repo
// root found for it (or rootErr).
func topLevelWarning(cwd, root string, rootErr error) string {
	if rootErr != nil {
		return "not in a git repository; saving to " + prices.RepoPath + " in the current directory"
	}
	if samePath(cwd, root) {
		return ""
	}
	return fmt.Sprintf("%s is not the repository's top level (%s); Bark reads %s from the directory it runs in", cwd, root, prices.RepoPath)
}

// samePath reports whether a and b are the same directory, following
// symlinks (on macOS, /tmp is /private/tmp).
func samePath(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// downloadPrices downloads the prices from url and saves them to output. A
// failed download leaves an existing file as it was.
func downloadPrices(ctx context.Context, client *http.Client, url, output string, today time.Time, out io.Writer) error {
	f, err := prices.Fetch(ctx, client, url, today)
	if err != nil {
		return err
	}
	if err := prices.Save(output, f); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Saved prices for %d models to %s\n", f.Models(), output)
	return nil
}
