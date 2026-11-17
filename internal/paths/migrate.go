package paths

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// ErrNothingToMigrate is returned by Migrate when there is no ~/.bark.
var ErrNothingToMigrate = errors.New("no ~/.bark directory to migrate")

// Move is one file or directory Migrate copied.
type Move struct {
	From string
	To   string
}

// MigrationResult is what Migrate did.
type MigrationResult struct {
	// From is the ~/.bark directory that was migrated.
	From string
	// Backup is where From was renamed to.
	Backup string
	// Moved lists what was copied, in order.
	Moved []Move
	// Skipped lists the names in From that Bark doesn't use; they are left
	// in Backup.
	Skipped []string
}

// Migrate copies Bark's files from ~/.bark to the XDG directories, then
// renames ~/.bark to ~/.bark.bak (with a timestamp from now if that is
// taken). It refuses to overwrite anything: if any destination exists it
// returns an error naming it and changes nothing. If a copy fails, it
// removes what it copied and leaves ~/.bark as it was.
func (e Env) Migrate(now time.Time) (MigrationResult, error) {
	legacy, err := e.legacyDir()
	if err != nil {
		return MigrationResult{}, err
	}
	if info, err := os.Stat(legacy); errors.Is(err, fs.ErrNotExist) {
		return MigrationResult{}, ErrNothingToMigrate
	} else if err != nil {
		return MigrationResult{}, err
	} else if !info.IsDir() {
		return MigrationResult{}, fmt.Errorf("%s is not a directory", legacy)
	}

	xdg, err := e.XDG()
	if err != nil {
		return MigrationResult{}, err
	}

	moves, skipped, err := plan(legacy, xdg)
	if err != nil {
		return MigrationResult{}, err
	}

	for _, m := range moves {
		if _, err := os.Lstat(m.To); err == nil {
			return MigrationResult{}, fmt.Errorf("%s already exists; move it away or merge it by hand", m.To)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return MigrationResult{}, err
		}
	}

	backup, err := backupPath(legacy, now)
	if err != nil {
		return MigrationResult{}, err
	}

	// Remember which of Bark's directories already exist, so a rollback
	// removes only the ones Migrate made.
	baseDirs := []string{xdg.Config, xdg.Cache, xdg.State}
	existed := map[string]bool{}
	for _, dir := range baseDirs {
		_, err := os.Stat(dir)
		existed[dir] = err == nil
	}

	var done []Move
	rollback := func(cause error) error {
		for _, m := range done {
			_ = os.RemoveAll(m.To)
		}
		for _, dir := range baseDirs {
			if !existed[dir] {
				// Only removes it if it is empty.
				_ = os.Remove(dir)
			}
		}
		return cause
	}

	for _, m := range moves {
		// Recorded first so a half-copied directory is removed too.
		done = append(done, m)
		if err := copyTree(m.From, m.To); err != nil {
			return MigrationResult{}, rollback(fmt.Errorf("copying %s: %w", m.From, err))
		}
	}

	if err := os.Rename(legacy, backup); err != nil {
		return MigrationResult{}, rollback(fmt.Errorf("renaming %s: %w", legacy, err))
	}

	return MigrationResult{From: legacy, Backup: backup, Moved: moves, Skipped: skipped}, nil
}

// plan lists where each entry of legacy goes, and the entries Bark doesn't
// use.
func plan(legacy string, xdg Paths) ([]Move, []string, error) {
	targets := []struct{ name, to string }{
		{legacyConfigFileName, xdg.ConfigFile()},
		{"commit.md", filepath.Join(xdg.Config, "commit.md")},
		{"pull_request_description.md", filepath.Join(xdg.Config, "pull_request_description.md")},
		{"reviewers", filepath.Join(xdg.Config, "reviewers")},
		{"instructions", filepath.Join(xdg.Config, "instructions")},
		{"prices.json", filepath.Join(xdg.Cache, "prices.json")},
		{"logs", filepath.Join(xdg.State, "logs")},
	}

	entries, err := os.ReadDir(legacy)
	if err != nil {
		return nil, nil, err
	}
	present := map[string]bool{}
	for _, entry := range entries {
		present[entry.Name()] = true
	}

	var moves []Move
	known := map[string]bool{}
	for _, target := range targets {
		known[target.name] = true
		if present[target.name] {
			moves = append(moves, Move{From: filepath.Join(legacy, target.name), To: target.to})
		}
	}

	var skipped []string
	for _, entry := range entries {
		if !known[entry.Name()] {
			skipped = append(skipped, entry.Name())
		}
	}

	return moves, skipped, nil
}

// backupPath returns legacy + ".bak", or with a timestamp from now added if
// that exists.
func backupPath(legacy string, now time.Time) (string, error) {
	backup := legacy + ".bak"
	if _, err := os.Lstat(backup); errors.Is(err, fs.ErrNotExist) {
		return backup, nil
	} else if err != nil {
		return "", err
	}

	backup += "-" + now.Format("20060102-150405")
	if _, err := os.Lstat(backup); err == nil {
		return "", fmt.Errorf("%s already exists", backup)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return backup, nil
}

// copyTree copies the file or directory from to to, keeping permissions
// and recreating symlinks as they are.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)

		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case entry.IsDir():
			return os.MkdirAll(dest, info.Mode().Perm()|0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			return os.Symlink(target, dest)
		case info.Mode().IsRegular():
			return copyFile(path, dest, info.Mode().Perm())
		default:
			return fmt.Errorf("%s is not a regular file", path)
		}
	})
}

func copyFile(from, to string, perm fs.FileMode) (err error) {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}

	dst, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := dst.Close(); err == nil {
			err = closeErr
		}
	}()

	_, err = io.Copy(dst, src)
	return err
}
