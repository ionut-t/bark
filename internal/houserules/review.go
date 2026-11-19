package houserules

import "context"

// ForReview reads the house rules for a review of diff, with read reading a
// file from the version of the repository under review (git.ReviewDiff's
// RepoFile, or the working tree for a diff from stdin).
func ForReview(ctx context.Context, read ReadFunc, diff string, files []string) Rules {
	return Load(ctx, read, Options{Files: files, Changed: ChangedPaths(diff)})
}
