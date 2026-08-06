package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

var (
	// Version is injected at build time via -ldflags:
	// -X lnr-core/pkg/version.Version=v1.0.0
	Version   = ""
	GitCommit = ""
	BuildDate = ""
)

// GetVersion returns the semantic version of the application.
// Priority:
// 1. Build-time injected Version variable (if not empty and not "dev")
// 2. runtime/debug.ReadBuildInfo() Main.Version (if not empty and not "(devel)")
// 3. VCS revision from build settings (truncated to 7 characters)
// 4. Injected Version variable (even if "dev")
// 5. Fallback string "dev"
func GetVersion() string {
	if Version != "" && Version != "dev" {
		return Version
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
		var revision string
		var modified bool
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					modified = true
				}
			}
		}
		if revision != "" {
			rev := revision
			if len(rev) > 7 {
				rev = rev[:7]
			}
			if modified {
				rev += "-dirty"
			}
			return rev
		}
	}

	if Version != "" {
		return Version
	}
	return "dev"
}

// GetBuildInfo returns formatted build metadata.
func GetBuildInfo() string {
	ver := GetVersion()
	commit := GitCommit
	date := BuildDate

	if commit == "" || date == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					if commit == "" {
						commit = s.Value
					}
				case "vcs.time":
					if date == "" {
						date = s.Value
					}
				}
			}
		}
	}

	if commit != "" && len(commit) > 7 {
		commit = commit[:7]
	}

	parts := []string{ver, fmt.Sprintf("(%s/%s)", runtime.GOOS, runtime.GOARCH)}
	if commit != "" {
		parts = append(parts, "commit:"+commit)
	}
	if date != "" {
		parts = append(parts, "built:"+date)
	}
	return strings.Join(parts, " ")
}

// ReleaseInfo represents GitHub release metadata for update checking.
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	HasUpdate   bool   `json:"-"`
}

// CheckLatestRelease queries GitHub API to check if a newer version is available.
// It is lightweight, respects timeouts, and uses streaming json decoding.
func CheckLatestRelease(ctx context.Context) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/SakagamiJun/lnovel_tui/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "lnr-core/"+GetVersion())

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var info ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	current := strings.TrimPrefix(GetVersion(), "v")
	latest := strings.TrimPrefix(info.TagName, "v")

	if current != "dev" && latest != "" && current != latest {
		info.HasUpdate = true
	}

	return &info, nil
}
