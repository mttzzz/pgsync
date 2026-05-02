package models

import "time"

/* SyncPlan is the immutable plan consumed by a sync engine. */
type SyncPlan struct {
	Database string
	Tables   []Table
	DryRun   bool
	Threads  int
	Engine   string
}

/* IsEmpty reports whether the plan has no selected database. */
func (p *SyncPlan) IsEmpty() bool { return p.Database == "" }

/* SyncResult summarizes a completed sync run. */
type SyncResult struct {
	Database     string
	StartedAt    time.Time
	FinishedAt   time.Time
	BytesCopied  int64
	RowsCopied   int64
	TablesCopied int
	Stages       map[string]time.Duration
	Err          error
}

/* Duration returns the wall-clock duration of the sync run. */
func (r SyncResult) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}
