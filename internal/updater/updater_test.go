package updater

import (
	"context"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatest(t *testing.T) {
	t.Parallel()
	doer := &fakeDoer{resp: &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"tag_name":"v1.2.3","html_url":"https://example","assets":[{"name":"pgsync-darwin-arm64.tar.gz","browser_download_url":"https://download","size":42}]}`))}}
	client := Client{RepoURL: "https://api.github.com/repos/o/r/", Doer: doer}
	rel, err := client.Latest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", rel.Version)
	assert.Equal(t, "https://download", rel.Assets[0].URL)
	assert.Equal(t, "https://api.github.com/repos/o/r/releases/latest", doer.req.URL.String())
}

func TestLatestErrors(t *testing.T) {
	t.Parallel()
	_, err := (Client{}).Latest(context.Background())
	assert.Error(t, err)
	_, err = (Client{RepoURL: "://bad", Doer: &fakeDoer{}}).Latest(context.Background())
	assert.Error(t, err)
	_, err = (Client{RepoURL: "https://x", Doer: &fakeDoer{err: errors.New("net")}}).Latest(context.Background())
	assert.Error(t, err)
	_, err = (Client{RepoURL: "https://x", Doer: &fakeDoer{resp: &http.Response{StatusCode: http.StatusNotFound, Status: "404", Body: io.NopCloser(strings.NewReader(""))}}}).Latest(context.Background())
	assert.Error(t, err)
	_, err = (Client{RepoURL: "https://x", Doer: &fakeDoer{resp: &http.Response{StatusCode: http.StatusOK, Status: "200", Body: io.NopCloser(strings.NewReader("{"))}}}).Latest(context.Background())
	assert.Error(t, err)
	_, err = (Client{RepoURL: "https://x", Doer: &fakeDoer{resp: &http.Response{StatusCode: http.StatusOK, Status: "200", Body: io.NopCloser(strings.NewReader(`{"html_url":"x"}`))}}}).Latest(context.Background())
	assert.Error(t, err)
}

func TestCheckFindAssetVersionAndDiscardHelpers(t *testing.T) {
	t.Parallel()
	assetName := "pgsync-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".zip"
	} else {
		assetName += ".tar.gz"
	}
	doer := &fakeDoer{resp: &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"tag_name":"v1.2.3","html_url":"https://example","assets":[{"name":"` + assetName + `","browser_download_url":"https://download","size":42},{"name":"checksums.txt","browser_download_url":"https://checksums","size":100}]}`))}}
	info, err := (Client{RepoURL: "https://api.github.com/repos/o/r", Doer: doer}).Check(context.Background(), "v1.2.2")
	require.NoError(t, err)
	assert.True(t, info.Available)
	assert.Equal(t, "https://download", info.DownloadURL)
	assert.Equal(t, "https://checksums", info.ChecksumURL)

	asset, err := FindAsset([]Asset{{Name: "pgsync-windows-amd64.zip", URL: "u"}}, "windows", "amd64")
	require.NoError(t, err)
	assert.Equal(t, "u", asset.URL)
	_, err = FindAsset(nil, "darwin", "arm64")
	assert.Error(t, err)

	assert.False(t, IsNewer("v1", "1"))
	assert.False(t, IsNewer("dev", "v1"))
	assert.False(t, IsNewer("v2", "v1"))
	assert.True(t, IsNewer("v1", "v2"))
	assert.True(t, IsNewer("v1.2.2", "v1.2.3"))
	assert.NoError(t, DiscardBody(strings.NewReader("abc")))
}

type fakeDoer struct {
	req  *http.Request
	resp *http.Response
	err  error
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.req = req
	return f.resp, f.err
}
