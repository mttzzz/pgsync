package clock_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/clock"
)

func TestSystemNow(t *testing.T) {
	t.Parallel()
	c := clock.NewSystem()
	before := time.Now().Add(-time.Millisecond)
	got := c.Now()
	after := time.Now().Add(time.Millisecond)
	assert.True(t, !got.Before(before) && !got.After(after),
		"got=%v, want in [%v, %v]", got, before, after)
}
