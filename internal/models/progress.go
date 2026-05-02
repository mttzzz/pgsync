package models

import "context"

/* Progress describes one sync progress event. */
type Progress struct {
	Stage string
	Table string
	Done  int64
	Total int64
}

/* Percent returns completion as 0..100, or zero when Total is unknown. */
func (p Progress) Percent() float64 {
	if p.Total <= 0 {
		return 0
	}
	return float64(p.Done) / float64(p.Total) * 100.0
}

/* ProgressObserver receives progress events from sync engines. */
type ProgressObserver interface {
	OnEvent(ctx context.Context, p Progress)
}
