// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package collect provides a worker that executes the collect-metrics hook
// periodically, as long as the workload has been started (between start and
// stop hooks). collect-metrics executes in its own execution context, which is
// restricted to avoid contention with uniter "lifecycle" hooks.
package collect
