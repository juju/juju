// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import "time"

// minRetryDelay is the minimum delay to apply
// to operation retries; this does not apply to
// the first attempt for operations.
const minRetryDelay = 30 * time.Second

// maxRetryDelay is the maximum delay to apply
// to operation retries. Retry delays will backoff
// up to this ceiling.
const maxRetryDelay = 30 * time.Minute

// scheduleOperations schedules the given operations
// by calculating the current time once, and then
// adding each operation's delay to that time. By
// calculating the current time once, we guarantee
// that operations with the same delay will be
// batched together.
func scheduleOperations(ctx *context, ops ...scheduleOp) {
	if len(ops) == 0 {
		return
	}
	now := ctx.time.Now()
	for _, op := range ops {
		k := op.key()
		d := op.delay()
		ctx.schedule.Add(k, op, now.Add(d))
	}
}

// scheduleOp is an interface implemented by schedule
// operations.
type scheduleOp interface {
	// key is the key for the operation; this
	// must be unique among all operations.
	key() interface{}

	// delay is the amount of time to delay
	// before next executing the operation.
	delay() time.Duration
}

// exponentialBackoff is a type that can be embedded to implement the
// delay() method of scheduleOp, providing truncated binary exponential
// backoff for operations that may be rescheduled.
type exponentialBackoff struct {
	d time.Duration
}

func (s *exponentialBackoff) delay() time.Duration {
	current := s.d
	if s.d < minRetryDelay {
		s.d = minRetryDelay
	} else {
		s.d *= 2
		if s.d > maxRetryDelay {
			s.d = maxRetryDelay
		}
	}
	return current
}
