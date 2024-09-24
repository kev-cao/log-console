package wait

import (
	"context"
	"time"
)

type TimeoutError struct{}

func (e TimeoutError) Error() string {
	return "Timed out."
}

// WaitFunc polls a function until it returns true or the timeout is reached.
func WaitFunc(f func() bool, timeout time.Duration, poll time.Duration) error {
	ticker := time.NewTicker(poll)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		select {
		case <-ticker.C:
			if f() {
				return nil
			}
		case <-ctx.Done():
			return TimeoutError{}
		}
	}
}
