// Package updater implements GitHub release update checks and self-updates.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultRepoAPIURL = "https://api.github.com/repos/mttzzz/pgsync"
	httpTimeout       = 30 * time.Second
)

// HTTPDoer is the HTTP seam for update checks.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Asset describes one release asset.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// Release describes an available release.
type Release struct {
	Version string
	URL     string
	Notes   string
	Assets  []Asset
}

// UpdateInfo describes an available update for the current platform.
type UpdateInfo struct {
	Available      bool
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	DownloadURL    string
	AssetName      string
	AssetSize      int64
	Notes          string
}

// UpdateResult describes an installed update.
type UpdateResult struct {
	PreviousVersion string
	NewVersion      string
	Path            string
	Duration        time.Duration
}

// Client checks GitHub Releases and installs selected assets.
type Client struct {
	Doer    HTTPDoer
	RepoURL string
}

// NewClient returns a production GitHub updater client.
func NewClient() Client {
	return Client{RepoURL: DefaultRepoAPIURL, Doer: &http.Client{Timeout: httpTimeout}}
}

// Latest fetches the latest GitHub release metadata.
func (c Client) Latest(ctx context.Context) (Release, error) {
	if c.Doer == nil {
		c.Doer = &http.Client{Timeout: httpTimeout}
	}
	if c.RepoURL == "" {
		return Release{}, fmt.Errorf("repo url is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.RepoURL, "/")+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	resp, err := c.Doer.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("fetch latest release: %s", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("latest release has no tag")
	}
	release := Release{Version: payload.TagName, URL: payload.HTMLURL, Notes: payload.Body, Assets: make([]Asset, 0, len(payload.Assets))}
	for _, asset := range payload.Assets {
		release.Assets = append(release.Assets, Asset{Name: asset.Name, URL: asset.URL, Size: asset.Size})
	}
	return release, nil
}

// Check returns update information for the current platform.
func (c Client) Check(ctx context.Context, currentVersion string) (UpdateInfo, error) {
	release, err := c.Latest(ctx)
	if err != nil {
		return UpdateInfo{}, err
	}
	info := UpdateInfo{
		Available:      IsNewer(currentVersion, release.Version),
		CurrentVersion: currentVersion,
		LatestVersion:  release.Version,
		ReleaseURL:     release.URL,
		Notes:          release.Notes,
	}
	asset, err := FindAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		if info.Available {
			return UpdateInfo{}, err
		}
		return info, nil
	}
	info.DownloadURL = asset.URL
	info.AssetName = asset.Name
	info.AssetSize = asset.Size
	return info, nil
}

// Install downloads info's asset and atomically replaces the current executable.
func (c Client) Install(ctx context.Context, info UpdateInfo) (UpdateResult, error) {
	started := time.Now()
	if info.DownloadURL == "" {
		return UpdateResult{}, errors.New("download url is required")
	}
	exe, err := os.Executable()
	if err != nil {
		return UpdateResult{}, fmt.Errorf("resolve executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("resolve executable symlink: %w", err)
	}
	mode := os.FileMode(0o755)
	if stat, statErr := os.Stat(exe); statErr == nil {
		mode = stat.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".pgsync-update-*")
	if err != nil {
		return UpdateResult{}, fmt.Errorf("create temp executable: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := c.download(ctx, info.DownloadURL, tmp); err != nil {
		return UpdateResult{}, err
	}
	if err := tmp.Chmod(mode); err != nil {
		return UpdateResult{}, fmt.Errorf("chmod temp executable: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return UpdateResult{}, fmt.Errorf("close temp executable: %w", err)
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		return UpdateResult{}, fmt.Errorf("replace executable: %w", err)
	}
	cleanup = false
	return UpdateResult{PreviousVersion: info.CurrentVersion, NewVersion: info.LatestVersion, Path: exe, Duration: time.Since(started)}, nil
}

func (c Client) download(ctx context.Context, url string, w io.Writer) error {
	if c.Doer == nil {
		c.Doer = &http.Client{Timeout: httpTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.Doer.Do(req)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download update: %s", resp.Status)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("write update: %w", err)
	}
	return nil
}

// FindAsset returns the release asset matching goos/goarch.
func FindAsset(assets []Asset, goos, goarch string) (Asset, error) {
	want := "pgsync-" + goos + "-" + goarch
	if goos == "windows" {
		want += ".exe"
	}
	for _, asset := range assets {
		if asset.Name == want {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("asset %q not found", want)
}

// IsNewer reports whether candidate is semantically newer than current.
func IsNewer(current, candidate string) bool {
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "v")
	if candidate == "" || current == "" || current == "dev" {
		return false
	}
	currentParts := versionParts(current)
	candidateParts := versionParts(candidate)
	for i := 0; i < len(candidateParts); i++ {
		if candidateParts[i] > currentParts[i] {
			return true
		}
		if candidateParts[i] < currentParts[i] {
			return false
		}
	}
	return false
}

func versionParts(raw string) [3]int {
	var out [3]int
	parts := strings.Split(raw, ".")
	for i := 0; i < len(parts) && i < len(out); i++ {
		value, err := strconv.Atoi(parts[i])
		if err == nil {
			out[i] = value
		}
	}
	return out
}

// DiscardBody is a small helper used by download flows.
func DiscardBody(r io.Reader) error {
	_, err := io.Copy(io.Discard, r)
	return err
}
