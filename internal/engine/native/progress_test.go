package native

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

func TestProgressReaderCountsBytesWithoutObserver(t *testing.T) {
	t.Parallel()
	reader := NewProgressReader(strings.NewReader("abcdef"), ProgressOptions{})

	buf := make([]byte, 2)
	n, err := reader.Read(buf)

	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, "ab", string(buf))
	assert.Equal(t, int64(2), reader.Bytes())

	remaining, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "cdef", string(remaining))
	assert.Equal(t, int64(6), reader.Bytes())
}

func TestProgressReaderPercentClampsAndHandlesUnknownEstimate(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0.0, progressPercent(10, 0))
	assert.Equal(t, 0.0, progressPercent(-1, 10))
	assert.Equal(t, 50.0, progressPercent(5, 10))
	assert.Equal(t, 100.0, progressPercent(20, 10))

	unknownObserver := &recordingObserver{}
	unknownClock := newManualClock()
	unknownReader := NewProgressReader(strings.NewReader("abc"), ProgressOptions{
		Observer: unknownObserver,
		Clock:    unknownClock,
	})
	_, err := io.ReadAll(unknownReader)
	require.NoError(t, err)
	unknownEvents := unknownObserver.Events()
	require.NotEmpty(t, unknownEvents)
	assert.Equal(t, int64(0), unknownEvents[len(unknownEvents)-1].Estimated)
	assert.Equal(t, 0.0, unknownEvents[len(unknownEvents)-1].Percent)

	clampedObserver := &recordingObserver{}
	clampedClock := newManualClock()
	clampedReader := NewProgressReader(strings.NewReader("abcd"), ProgressOptions{
		Observer:  clampedObserver,
		Clock:     clampedClock,
		Estimated: 2,
	})
	_, err = io.ReadAll(clampedReader)
	require.NoError(t, err)
	clampedEvents := clampedObserver.Events()
	require.NotEmpty(t, clampedEvents)
	assert.Equal(t, 100.0, clampedEvents[len(clampedEvents)-1].Percent)
}

func TestProgressReaderBytesPerSecondUsesInjectedClock(t *testing.T) {
	t.Parallel()
	clock := newManualClock()
	observer := &recordingObserver{}
	reader := NewProgressReader(strings.NewReader("1234"), ProgressOptions{
		Table:     models.Table{Schema: "public", Name: "users"},
		Observer:  observer,
		Clock:     clock,
		Estimated: 8,
		Context:   context.Background(),
	})

	clock.Advance(2 * time.Second)
	buf := make([]byte, 4)
	n, err := reader.Read(buf)

	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, int64(4), reader.Bytes())
	events := observer.Events()
	require.Len(t, events, 2)
	assert.Equal(t, engine.EventTableCopyProgress, events[1].Name)
	assert.Equal(t, "copy", events[1].Stage)
	assert.Equal(t, "native", events[1].Engine)
	assert.Equal(t, "public.users", events[1].Table)
	assert.Equal(t, int64(4), events[1].Rows)
	assert.Equal(t, int64(8), events[1].Estimated)
	assert.Equal(t, int64(4), events[1].Bytes)
	assert.Equal(t, 50.0, events[1].Percent)
	assert.Equal(t, 2.0, events[1].BytesPerSec)
	assert.Equal(t, 2*time.Second, events[1].Duration)
}

func TestProgressReaderThrottlesMiddleEventsButEmitsInitialAndFinal(t *testing.T) {
	t.Parallel()
	clock := newManualClock()
	observer := &recordingObserver{}
	reader := NewProgressReader(strings.NewReader("xy"), ProgressOptions{
		Table:     models.Table{Schema: "public", Name: "events", SizeBytes: 2},
		Observer:  observer,
		Clock:     clock,
		Interval:  time.Hour,
		Estimated: 2,
	})

	buf := make([]byte, 1)
	clock.Advance(time.Second)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	clock.Advance(time.Second)
	n, err = reader.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	clock.Advance(time.Second)
	n, err = reader.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	assert.Zero(t, n)

	clock.Advance(time.Second)
	n, err = reader.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	assert.Zero(t, n)

	events := observer.Events()
	require.Len(t, events, 2)
	assert.Equal(t, int64(0), events[0].Bytes)
	assert.Equal(t, int64(2), events[1].Bytes)
	assert.Equal(t, 100.0, events[1].Percent)
	assert.Equal(t, 3*time.Second, events[1].Duration)
}

func TestProgressEstimateAndTableEventNameFallbacks(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int64(9), progressEstimate(ProgressOptions{Estimated: 9, EstimatedBytes: 8, Total: 7}))
	assert.Equal(t, int64(8), progressEstimate(ProgressOptions{EstimatedBytes: 8, Total: 7}))
	assert.Equal(t, int64(7), progressEstimate(ProgressOptions{Total: 7, Table: models.Table{SizeBytes: 6}}))
	assert.Equal(t, int64(6), progressEstimate(ProgressOptions{Table: models.Table{SizeBytes: 6, Rows: 4}}))
	assert.Equal(t, int64(5), progressEstimate(ProgressOptions{EstimatedRows: 5, Table: models.Table{Rows: 4}}))
	assert.Equal(t, int64(4), progressEstimate(ProgressOptions{Table: models.Table{Rows: 4}}))
	assert.Zero(t, progressEstimate(ProgressOptions{}))

	assert.Equal(t, "public.users", tableEventName(models.Table{Schema: "public", Name: "users"}))
	assert.Equal(t, "users", tableEventName(models.Table{Name: "users"}))
	assert.Equal(t, "public", tableEventName(models.Table{Schema: "public"}))
}

type manualClock struct {
	mu  sync.Mutex
	now time.Time
}

func newManualClock() *manualClock {
	return &manualClock{now: time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type recordingObserver struct {
	mu     sync.Mutex
	events []engine.Event
}

func (o *recordingObserver) OnEvent(_ context.Context, event engine.Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event)
}

func (o *recordingObserver) Events() []engine.Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]engine.Event(nil), o.events...)
}
