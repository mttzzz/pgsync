// Package updater implements GitHub release update checks and self-updates.
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultRepoAPIURL is the GitHub API repository endpoint used by production updates.
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
	ChecksumURL    string
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
	if checksum, err := FindChecksumAsset(release.Assets); err == nil {
		info.ChecksumURL = checksum.URL
	}
	return info, nil
}

// Install downloads info's asset and atomically replaces the current executable.
//
//nolint:gocyclo,gocognit // Self-update install is a linear sequence with error handling for each filesystem step.
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
	assetData, err := c.downloadBytes(ctx, info.DownloadURL)
	if err != nil {
		return UpdateResult{}, err
	}
	if info.ChecksumURL != "" {
		checksumData, err := c.downloadBytes(ctx, info.ChecksumURL)
		if err != nil {
			return UpdateResult{}, err
		}
		if err := verifyChecksum(info.AssetName, assetData, checksumData); err != nil {
			return UpdateResult{}, err
		}
	}
	binaryData, err := extractBinary(info.AssetName, assetData, runtime.GOOS)
	if err != nil {
		return UpdateResult{}, err
	}
	if _, err := tmp.Write(binaryData); err != nil {
		return UpdateResult{}, fmt.Errorf("write update candidate: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		return UpdateResult{}, fmt.Errorf("chmod temp executable: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return UpdateResult{}, fmt.Errorf("close temp executable: %w", err)
	}
	if err := replaceExecutable(tmpPath, exe); err != nil {
		return UpdateResult{}, fmt.Errorf("replace executable: %w", err)
	}
	cleanup = false
	return UpdateResult{PreviousVersion: info.CurrentVersion, NewVersion: info.LatestVersion, Path: exe, Duration: time.Since(started)}, nil
}

func replaceExecutable(candidate, target string) error {
	if runtime.GOOS == "windows" {
		return scheduleWindowsReplace(candidate, target)
	}
	return os.Rename(candidate, target)
}

func scheduleWindowsReplace(candidate, target string) error {
	dir := filepath.Dir(target)
	script, err := os.CreateTemp(dir, ".pgsync-update-*.cmd")
	if err != nil {
		return fmt.Errorf("create update helper: %w", err)
	}
	scriptPath := script.Name()
	backup := target + ".old"
	content := windowsReplaceScript(scriptPath, candidate, target, backup)
	if _, err := script.WriteString(content); err != nil {
		_ = script.Close()
		_ = os.Remove(scriptPath)
		return fmt.Errorf("write update helper: %w", err)
	}
	if err := script.Close(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("close update helper: %w", err)
	}
	cmd := exec.Command("cmd", "/C", "start", "", "/MIN", scriptPath) // #nosec G204 -- command and helper path are generated by pgsync.
	if err := cmd.Run(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("start update helper: %w", err)
	}
	return nil
}

func windowsReplaceScript(scriptPath, candidate, target, backup string) string {
	return strings.Join([]string{
		"@echo off",
		"setlocal",
		"set \"SCRIPT=" + scriptPath + "\"",
		"set \"CANDIDATE=" + candidate + "\"",
		"set \"TARGET=" + target + "\"",
		"set \"BACKUP=" + backup + "\"",
		"for /L %%i in (1,1,120) do (",
		"  move /Y \"%TARGET%\" \"%BACKUP%\" >NUL 2>NUL",
		"  if not errorlevel 1 goto replace",
		"  ping -n 2 127.0.0.1 >NUL",
		")",
		"exit /B 1",
		":replace",
		"move /Y \"%CANDIDATE%\" \"%TARGET%\" >NUL 2>NUL",
		"if errorlevel 1 (",
		"  move /Y \"%BACKUP%\" \"%TARGET%\" >NUL 2>NUL",
		"  exit /B 1",
		")",
		"del /F /Q \"%BACKUP%\" >NUL 2>NUL",
		"del /F /Q \"%SCRIPT%\" >NUL 2>NUL",
		"exit /B 0",
		"",
	}, "\r\n")
}

func (c Client) downloadBytes(ctx context.Context, url string) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.download(ctx, url, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
	wants := assetNames(goos, goarch)
	for _, want := range wants {
		for _, asset := range assets {
			if asset.Name == want {
				return asset, nil
			}
		}
	}
	return Asset{}, fmt.Errorf("asset %q not found", wants[0])
}

func assetNames(goos, goarch string) []string {
	base := "pgsync-" + goos + "-" + goarch
	if goos == "windows" {
		return []string{base + ".zip", base + ".exe"}
	}
	return []string{base + ".tar.gz"}
}

// FindChecksumAsset returns the checksums.txt release asset.
func FindChecksumAsset(assets []Asset) (Asset, error) {
	for _, asset := range assets {
		if asset.Name == "checksums.txt" {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("asset %q not found", "checksums.txt")
}

func verifyChecksum(assetName string, data, checksums []byte) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && filepath.Base(fields[1]) == assetName {
			if strings.EqualFold(fields[0], got) {
				return nil
			}
			return fmt.Errorf("checksum mismatch for %s", assetName)
		}
	}
	return fmt.Errorf("checksum for %s not found", assetName)
}

//nolint:gocognit,gocyclo // Archive extraction validates zip and tar.gz formats with path traversal checks.
func extractBinary(assetName string, data []byte, goos string) ([]byte, error) {
	bin := "pgsync"
	if goos == "windows" {
		bin = "pgsync.exe"
	}
	if strings.HasSuffix(assetName, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("open update zip: %w", err)
		}
		for _, file := range zr.File {
			if unsafeArchiveName(file.Name) {
				return nil, fmt.Errorf("unsafe archive path %q", file.Name)
			}
			if filepath.Base(file.Name) != bin {
				continue
			}
			r, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("open update binary: %w", err)
			}
			defer func() { _ = r.Close() }()
			return io.ReadAll(r)
		}
		return nil, fmt.Errorf("update binary %s not found in %s", bin, assetName)
	}
	if strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz") {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("open update tar.gz: %w", err)
		}
		defer func() { _ = gz.Close() }()
		tr := tar.NewReader(gz)
		for {
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read update tar.gz: %w", err)
			}
			if unsafeArchiveName(header.Name) {
				return nil, fmt.Errorf("unsafe archive path %q", header.Name)
			}
			if filepath.Base(header.Name) != bin {
				continue
			}
			return io.ReadAll(tr)
		}
		return nil, fmt.Errorf("update binary %s not found in %s", bin, assetName)
	}
	return data, nil
}

func unsafeArchiveName(name string) bool {
	clean := filepath.Clean(name)
	return filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
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
