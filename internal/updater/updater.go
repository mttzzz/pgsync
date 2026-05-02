// Package updater implements GitHub release update checks.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPDoer is the HTTP seam for update checks.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Release describes an available release.
type Release struct {
	Version string
	URL     string
}

// Client checks GitHub Releases.
type Client struct {
	Doer    HTTPDoer
	RepoURL string
}

// Latest fetches the latest GitHub release metadata.
func (c Client) Latest(ctx context.Context) (Release, error) {
	if c.Doer == nil {
		c.Doer = http.DefaultClient
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	if payload.TagName == "" {
		return Release{}, fmt.Errorf("latest release has no tag")
	}
	return Release{Version: payload.TagName, URL: payload.HTMLURL}, nil
}

// IsNewer reports whether candidate differs from current.
func IsNewer(current, candidate string) bool {
	return strings.TrimPrefix(candidate, "v") != strings.TrimPrefix(current, "v")
}

// DiscardBody is a small helper used by download flows.
func DiscardBody(r io.Reader) error {
	_, err := io.Copy(io.Discard, r)
	return err
}
