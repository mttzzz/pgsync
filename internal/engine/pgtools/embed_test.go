package pgtools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManifest(t *testing.T) {
	t.Parallel()
	data := []byte(`schema_version = 1
tool_version = "18.0"
[platforms.linux-amd64]
url = "https://example.invalid/pg.zip"
archive_sha256 = "abc"
files = ["pg_dump", "pg_restore"]
expected_binaries = ["pg_dump", "pg_restore"]
`)
	manifest, err := ParseManifest(data)
	require.NoError(t, err)
	assert.Equal(t, "18.0", manifest.ToolVersion)
	_, err = ParseManifest([]byte(`bad`))
	assert.Error(t, err)
	_, err = ParseManifest([]byte(`schema_version = 2
tool_version = "18"
[platforms.x]
url="u"
archive_sha256="s"
files=["pg_dump"]
expected_binaries=["pg_dump"]`))
	assert.Error(t, err)
	_, err = ParseManifest([]byte(`schema_version = 1`))
	assert.Error(t, err)
	_, err = ParseManifest([]byte(`schema_version = 1
tool_version="18"`))
	assert.Error(t, err)
	_, err = ParseManifest([]byte(`schema_version = 1
tool_version="18"
[platforms.x]
url=""
archive_sha256="s"
files=[]
expected_binaries=[]`))
	assert.Error(t, err)
	_, err = ParseManifest([]byte(`schema_version = 1
tool_version="18"
[platforms.x]
url="u"
archive_sha256="s"
files=[]
expected_binaries=[]`))
	assert.Error(t, err)
	_, err = ParseManifest([]byte(`schema_version = 1
tool_version="18"
[platforms.x]
url="u"
archive_sha256="s"
files=["pg_dump"]
expected_binaries=["pg_restore"]`))
	assert.Error(t, err)
}

func TestEmbeddedLocator(t *testing.T) {
	t.Parallel()
	source := fakePayload{platform: "test", files: map[string][]byte{BinDump(): []byte("dump"), BinRestore(): []byte("restore")}}
	loc := NewEmbeddedLocator(source, t.TempDir())
	dump, err := loc.PgDump()
	require.NoError(t, err)
	assert.FileExists(t, dump)
	restore, err := loc.PgRestore()
	require.NoError(t, err)
	assert.FileExists(t, restore)
	assert.NotEmpty(t, payloadHash(source.files))

	loc = NewEmbeddedLocator(nil, t.TempDir())
	assert.Error(t, loc.Extract())
	loc = NewEmbeddedLocator(fakePayload{platform: "x", files: map[string][]byte{}}, t.TempDir())
	assert.Error(t, loc.Extract())
	loc = NewEmbeddedLocator(fakePayload{platform: "x", files: map[string][]byte{"other": []byte("x")}}, t.TempDir())
	_, err = loc.PgDump()
	assert.Error(t, err)
}

func TestEmbeddedWriteError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(path, 0o700))
	err := writeEmbeddedFile(path, []byte("x"))
	assert.Error(t, err)
}

type fakePayload struct {
	platform string
	files    map[string][]byte
}

func (f fakePayload) Platform() string         { return f.platform }
func (f fakePayload) Files() map[string][]byte { return f.files }
