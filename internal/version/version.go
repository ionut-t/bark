package version

import (
	"runtime/debug"
	"strings"
	"time"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func init() {
	// Only use build info if version wasn't set via ldflags
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			// Try to get version from build info
			if info.Main.Version != "(devel)" && info.Main.Version != "" {
				// Clean up pseudo-versions for better display
				v := info.Main.Version
				// If it's a pseudo-version like v0.0.0-20210101120000-abcdef123456
				// Just show "dev-abcdef" instead
				if strings.HasPrefix(v, "v0.0.0-") && strings.Contains(v, "-") {
					parts := strings.Split(v, "-")
					if len(parts) >= 3 {
						version = "dev-" + parts[2][:7]
					}
				} else {
					version = v
				}
			}

			// Get commit and date from VCS info
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					if commit == "unknown" {
						if len(setting.Value) > 7 {
							commit = setting.Value[:7]
						} else if setting.Value != "" {
							commit = setting.Value
						}
					}
				case "vcs.time":
					if date == "unknown" {
						if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
							date = t.Format("02/01/2006")
						}
					}
				}
			}
		}
	}
}

func Version() string {
	return version
}

func Commit() string {
	return commit
}

func Date() string {
	return date
}
