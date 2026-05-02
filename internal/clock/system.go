package clock

import "time"

/* System is a Clock backed by time.Now. */
type System struct{}

/* NewSystem returns a time.Now-backed Clock. */
func NewSystem() *System { return &System{} }

/* Now returns the current wall-clock time. */
func (System) Now() time.Time { return time.Now() }
