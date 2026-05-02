package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiRed   = "\x1b[31m"
)

// PlainOptions configures human-readable sync output.
type PlainOptions struct {
	Quiet   bool
	NoColor bool
	Color   bool
}

// PlainObserver writes human-readable engine progress events.
type PlainObserver struct {
	w    io.Writer
	opts PlainOptions
	mu   sync.Mutex
}

// NewPlainObserver returns a human-readable progress observer.
func NewPlainObserver(w io.Writer, opts PlainOptions) *PlainObserver {
	return &PlainObserver{w: outputWriter(w), opts: opts}
}

// OnEvent writes one human-readable line for an engine event.
func (o *PlainObserver) OnEvent(_ context.Context, event engine.Event) {
	if o == nil || o.opts.Quiet {
		return
	}
	line := plainEventLine(event, o.opts)
	if line == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_, _ = fmt.Fprintln(o.w, line)
}

// PrintPlan writes a dry-run sync plan without connection details or secrets.
func PrintPlan(w io.Writer, plan *models.SyncPlan, opts PlainOptions) error {
	if plan == nil {
		return errors.New("sync plan is required")
	}
	writer := outputWriter(w)
	if _, err := fmt.Fprintf(writer, "%s database=%s engine=%s tables=%d dry_run=%t\n",
		plainColor("plan", ansiBold, opts), plan.Database, plan.Engine, len(plan.Tables), plan.DryRun); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "threads=%d sequences=%d\n", plan.Threads, len(plan.Sequences)); err != nil {
		return err
	}
	for index, table := range plan.Tables {
		if _, err := fmt.Fprintf(writer, "%d. %s rows=%d bytes=%d\n",
			index+1, plainTableName(table), table.Rows, table.SizeBytes); err != nil {
			return err
		}
	}
	return nil
}

// PrintResult writes a human-readable sync result summary.
func PrintResult(w io.Writer, result *models.SyncResult, opts PlainOptions) error {
	if result == nil {
		return errors.New("sync result is required")
	}
	_, err := fmt.Fprintf(outputWriter(w), "%s database=%s tables=%d rows=%d bytes=%d duration=%s\n",
		plainColor("synced", ansiBold, opts), result.Database, result.TablesCopied,
		result.RowsCopied, result.BytesCopied, formatDuration(result.Duration()))
	return err
}

func plainEventLine(event engine.Event, opts PlainOptions) string {
	for _, formatter := range []func(engine.Event, PlainOptions) string{
		plainSyncLine,
		plainSchemaLine,
		plainTableLine,
	} {
		if line := formatter(event, opts); line != "" {
			return line
		}
	}
	return plainStageLine(event)
}

func plainSyncLine(event engine.Event, opts PlainOptions) string {
	switch event.Name {
	case engine.EventSyncStart:
		return fmt.Sprintf("%s database=%s tables=%d engine=%s",
			plainColor("starting sync", ansiBold, opts), event.Database, event.Tables, event.Engine)
	case engine.EventSyncDone:
		return fmt.Sprintf("sync done tables=%d bytes=%d duration=%s",
			event.Tables, event.Bytes, formatDuration(event.Duration))
	case engine.EventSyncFailed:
		return plainFailureLine(event, opts)
	default:
		return ""
	}
}

func plainSchemaLine(event engine.Event, _ PlainOptions) string {
	switch event.Name {
	case engine.EventSchemaPreDataStart:
		return "schema pre-data: start"
	case engine.EventSchemaPreDataDone:
		return fmt.Sprintf("schema pre-data: done in %s", formatDuration(event.Duration))
	case engine.EventSchemaPostDataStart:
		return "schema post-data: start"
	case engine.EventSchemaPostDataDone:
		return fmt.Sprintf("schema post-data: done in %s", formatDuration(event.Duration))
	default:
		return ""
	}
}

func plainTableLine(event engine.Event, _ PlainOptions) string {
	switch event.Name {
	case engine.EventTableCopyStart:
		return fmt.Sprintf("copying table %s est_rows=%d", event.Table, event.Estimated)
	case engine.EventTableCopyProgress:
		return fmt.Sprintf("table %s rows=%d pct=%.1f%% bytes_per_sec=%.0f",
			event.Table, event.Rows, event.Percent, event.BytesPerSec)
	case engine.EventTableCopyDone:
		return fmt.Sprintf("table %s done rows=%d bytes=%d duration=%s",
			event.Table, event.Rows, event.Bytes, formatDuration(event.Duration))
	default:
		return ""
	}
}

func plainFailureLine(event engine.Event, opts PlainOptions) string {
	message := fmt.Sprintf("%s stage=%s", plainColor("error", ansiRed, opts), event.Stage)
	if event.Table != "" {
		message += " table=" + scrubSecrets(event.Table)
	}
	if event.Error != "" {
		message += ": " + scrubSecrets(event.Error)
	}
	return message
}

func plainStageLine(event engine.Event) string {
	if event.Name == "" {
		return ""
	}
	line := scrubSecrets(event.Name)
	if event.Stage != "" {
		line += " stage=" + scrubSecrets(event.Stage)
	}
	if event.Error != "" {
		line += " error=" + scrubSecrets(event.Error)
	}
	return line
}

func plainTableName(table models.Table) string {
	if table.Schema == "" {
		return table.Name
	}
	if table.Name == "" {
		return table.Schema
	}
	return table.Schema + "." + table.Name
}

func plainColor(text string, code string, opts PlainOptions) string {
	if !opts.Color || opts.NoColor {
		return text
	}
	return code + text + ansiReset
}

func outputWriter(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		return "0s"
	}
	return duration.Round(time.Millisecond).String()
}
