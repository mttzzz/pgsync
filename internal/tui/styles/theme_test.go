package styles

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestThemeNoColorAndColor(t *testing.T) {
	t.Parallel()
	plain := NewTheme(true, 4)
	assert.True(t, plain.NoColor)
	assert.Equal(t, "abcd", plain.Trim("abcdef"))
	assert.Equal(t, "abc", plain.Trim("abc"))
	assert.Equal(t, "abc", NewTheme(true, 0).Trim("abc"))
	assert.NotContains(t, plain.Title.Render("title"), "\x1b[")

	color := NewTheme(false, 10)
	assert.False(t, color.NoColor)
	assert.True(t, strings.Contains(color.Title.Render("title"), "title"))
}
