// Package paths finds the directories Bark keeps its user files in.
//
// Bark follows the XDG base directory spec: settings, reviewers and
// instructions go in $XDG_CONFIG_HOME/bark, the prices file in
// $XDG_CACHE_HOME/bark and debug logs in $XDG_STATE_HOME/bark. Unset
// variables default to ~/.config, ~/.cache and ~/.local/state, on macOS
// too. On Windows they default to %AppData% (config) and %LocalAppData%
// (cache and state).
//
// Bark used to keep everything in ~/.bark. Until it is migrated (see
// Migrate), a ~/.bark with a config file in it is still used.
package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const (
	appName = "bark"

	// legacyDirName is the directory in $HOME Bark used before XDG.
	legacyDirName = ".bark"
	// legacyConfigFileName is the config file's name in legacyDirName.
	legacyConfigFileName = ".config.toml"
	// configFileName is the config file's name in the config directory.
	configFileName = "config.toml"
)

// Paths are the directories Bark keeps its user files in.
type Paths struct {
	// Config holds the config file, commit and PR instructions, reviewers
	// and saved instructions.
	Config string
	// Cache holds the prices file.
	Cache string
	// State holds the debug logs.
	State string
	// Legacy is true when all three are the old ~/.bark directory.
	Legacy bool
}

// ConfigFile is the path of the config file in p.Config.
func (p Paths) ConfigFile() string {
	if p.Legacy {
		return filepath.Join(p.Config, legacyConfigFileName)
	}
	return filepath.Join(p.Config, configFileName)
}

// Env is what the directories are worked out from.
type Env struct {
	Getenv func(key string) string
	Home   func() (string, error)
	GOOS   string
}

// System is the running process's Env.
func System() Env {
	return Env{Getenv: os.Getenv, Home: os.UserHomeDir, GOOS: runtime.GOOS}
}

// Resolve returns the running process's Paths. See Env.Resolve.
func Resolve() (Paths, error) {
	return System().Resolve()
}

// Resolve returns Bark's directories. It returns ~/.bark for all of them
// (and Legacy) when ~/.bark has a config file and the XDG config directory
// doesn't, so existing setups keep working until they are migrated. It
// doesn't create any directory.
func (e Env) Resolve() (Paths, error) {
	xdg, err := e.XDG()
	if err != nil {
		return Paths{}, err
	}

	if _, err := os.Stat(xdg.ConfigFile()); err == nil {
		return xdg, nil
	}

	// Without a home directory there can't be a ~/.bark.
	legacy, err := e.legacyDir()
	if err != nil {
		return xdg, nil
	}
	if info, err := os.Stat(filepath.Join(legacy, legacyConfigFileName)); err == nil && !info.IsDir() {
		return Paths{Config: legacy, Cache: legacy, State: legacy, Legacy: true}, nil
	}

	return xdg, nil
}

// XDG returns the XDG directories, whether or not ~/.bark exists.
func (e Env) XDG() (Paths, error) {
	var config, cache, state string

	if e.GOOS == "windows" {
		config = e.absEnv("AppData")
		cache = e.absEnv("LocalAppData")
		state = cache
	}

	config, err := e.base("XDG_CONFIG_HOME", config, ".config")
	if err != nil {
		return Paths{}, err
	}
	cache, err = e.base("XDG_CACHE_HOME", cache, ".cache")
	if err != nil {
		return Paths{}, err
	}
	state, err = e.base("XDG_STATE_HOME", state, filepath.Join(".local", "state"))
	if err != nil {
		return Paths{}, err
	}

	return Paths{
		Config: filepath.Join(config, appName),
		Cache:  filepath.Join(cache, appName),
		State:  filepath.Join(state, appName),
	}, nil
}

// base returns the XDG variable key if it is set to an absolute path, then
// platform if it isn't empty, then homeRel in the home directory.
func (e Env) base(key, platform, homeRel string) (string, error) {
	if dir := e.absEnv(key); dir != "" {
		return dir, nil
	}
	if platform != "" {
		return platform, nil
	}

	home, err := e.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, homeRel), nil
}

// absEnv returns the variable key if it is an absolute path. The XDG spec
// says relative paths are invalid and should be ignored.
func (e Env) absEnv(key string) string {
	if dir := e.Getenv(key); filepath.IsAbs(dir) {
		return dir
	}
	return ""
}

func (e Env) legacyDir() (string, error) {
	home, err := e.Home()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", errors.New("home directory is not set")
	}
	return filepath.Join(home, legacyDirName), nil
}
