// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
)

// maxRetries is set to a lot lower number than lxc.
// https://github.com/lxc/lxd/blob/master/lxd/db/query/retry.go#L16
const maxRetries = 10

var strategy = retry.CallArgs{
	IsFatalError: func(err error) bool {
		// No point continuing if we hit a no-error.
		if errors.Cause(err) == sql.ErrNoRows {
			return true
		}

		// If the error is not retryable then we should consider it fatal.
		return !isErrorRetryable(err)
	},
	// Allow the retry strategy to back-off with some jitter to prevent
	// contention.
	BackoffFunc: retry.ExpBackoff(time.Millisecond*20, time.Millisecond*200, 2.0, true),
	Attempts:    maxRetries,
	// TODO (stickupkid): Allow injection of the wall clock.
	Clock: clock.WallClock,
	Delay: time.Millisecond * 10,
}

// withRetry wraps a function that wraps database calls, and retries it in
// case a transient dqlite/sqlite error is hit.
func withRetry(fn func() error) error {
	args := strategy
	args.Func = fn
	return retry.Call(args)
}
