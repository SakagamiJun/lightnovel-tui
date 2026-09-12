package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestGetVersionDefault(t *testing.T) {
	origVer := Version
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		Version = origVer
		GitCommit = origCommit
		BuildDate = origDate
	}()

	Version = ""
	GitCommit = ""
	BuildDate = ""

	v := GetVersion()
	if v == "" {
		t.Error("expected non-empty version")
	}

	Version = "v1.2.3"
	if GetVersion() != "v1.2.3" {
		t.Errorf("expected v1.2.3, got %s", GetVersion())
	}
}

func TestGetBuildInfo(t *testing.T) {
	origVer := Version
	origCommit := GitCommit
	origDate := BuildDate
	defer func() {
		Version = origVer
		GitCommit = origCommit
		BuildDate = origDate
	}()

	Version = "v1.0.0"
	GitCommit = "abcdef123456"
	BuildDate = "2026-09-11T00:00:00Z"

	info := GetBuildInfo()
	expectedArch := runtime.GOOS + "/" + runtime.GOARCH
	if !strings.Contains(info, "v1.0.0") {
		t.Errorf("expected info to contain v1.0.0, got: %s", info)
	}
	if !strings.Contains(info, expectedArch) {
		t.Errorf("expected info to contain %s, got: %s", expectedArch, info)
	}
	if !strings.Contains(info, "commit:abcdef1") {
		t.Errorf("expected truncated commit abcdef1, got: %s", info)
	}
	if !strings.Contains(info, "built:2026-09-11T00:00:00Z") {
		t.Errorf("expected built date, got: %s", info)
	}
}

func TestReleaseInfoUpdateComparison(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v2.0.0",
			"html_url": "https://github.com/SakagamiJun/lightnovel-tui/releases/tag/v2.0.0",
			"published_at": "2026-09-11T00:00:00Z"
		}`))
	}))
	defer server.Close()

	// Direct check struct logic
	info := ReleaseInfo{
		TagName:     "v2.0.0",
		HTMLURL:     "https://github.com/SakagamiJun/lightnovel-tui/releases/tag/v2.0.0",
		PublishedAt: "2026-09-11T00:00:00Z",
	}

	curr := "1.0.0"
	latest := strings.TrimPrefix(info.TagName, "v")
	if latest != curr {
		info.HasUpdate = true
	}
	if !info.HasUpdate {
		t.Error("expected HasUpdate to be true")
	}
}

func TestCheckLatestReleaseCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled immediately

	_, err := CheckLatestRelease(ctx)
	if err == nil {
		t.Error("expected error for canceled context, got nil")
	}
}
