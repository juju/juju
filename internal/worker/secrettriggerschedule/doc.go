// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package secrettriggerschedule provides shared scheduling, retry, and
// timer/alarm helpers for secret trigger workers.
//
// The workers share a common flow: a watcher reports upcoming trigger
// times, a timer or alarm fires when one is due, and a fired trigger is
// confirmed only once a subsequent watcher event reports the updated
// schedule. How long to keep awaiting that confirmation before giving up
// and restarting with fresh state from the controller is a per-worker
// policy:
//
//   - secretbackendrotate retains a fired token rotation for a single
//     RotateRetryDelay confirmation window, exiting with a sentinel error
//     if the updated trigger time does not arrive within it, so the
//     runner created by NewWithReset restarts the worker with a fresh
//     watcher.
//
//   - secretexpire requeues unconfirmed revision expiries every
//     ExpireRetryDelay for up to MaxTriggerRetries attempts, then exits
//     with a sentinel error so that the runner created by NewWithReset
//     restarts it with a fresh watcher.
//
// secretrotate needs neither policy since the controller re-sends rotate
// trigger events every RotateRetryDelay until the rotation is actually
// done, so its watcher cannot go stale.
//
// While a fired trigger is awaiting confirmation or retrying, watcher
// updates with the identical trigger time are ignored
// (ShouldIgnoreTime). Strictly earlier or later times are always applied.
package secrettriggerschedule
