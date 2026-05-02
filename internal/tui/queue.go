package tui

import "github.com/mttzzz/pgsync/internal/models"

// Queue is a pure FIFO sync queue model.
type Queue struct {
	items   []*models.SyncPlan
	Results []*models.SyncResult
	Paused  bool
	Current *models.SyncPlan
}

// Enqueue appends plans to the queue.
func (q Queue) Enqueue(plans ...*models.SyncPlan) Queue {
	q.items = append(q.items, plans...)
	return q
}

// StartNext starts the next queued plan unless paused or already running.
func (q Queue) StartNext() Queue {
	if q.Paused || q.Current != nil || len(q.items) == 0 {
		return q
	}
	q.Current = q.items[0]
	q.items = q.items[1:]
	return q
}

// Complete records a result and clears the current plan.
func (q Queue) Complete(result *models.SyncResult) Queue {
	if result != nil {
		q.Results = append(q.Results, result)
	}
	q.Current = nil
	return q
}

// Pause pauses future queue starts.
func (q Queue) Pause() Queue { q.Paused = true; return q }

// Resume resumes queue starts.
func (q Queue) Resume() Queue { q.Paused = false; return q }

// Len returns queued item count.
func (q Queue) Len() int { return len(q.items) }
