// Package clock makes wall-clock access injectable and testable.
package clock

import "time"

/* Clock returns the current wall-clock time. */
type Clock interface {
	Now() time.Time
}
