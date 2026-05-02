package pgtools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBinNameByOS(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "pg_dump.exe", binName("windows", "pg_dump"))
	assert.Equal(t, "pg_dump", binName("linux", "pg_dump"))
	assert.Equal(t, "pg_restore.exe", binName("windows", "pg_restore"))
	assert.Equal(t, "pg_restore", binName("darwin", "pg_restore"))
}
