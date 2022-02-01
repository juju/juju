// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import "time"

const (
	// Add a small fudge factor to the wait times; if we use exactly the same wait time
	// for the fake clock, on slow systems the next watcher event can occur before the
	// watcher sync can run to process the first event.

	TxnWatcherShortWait      = time.Duration(1.1 * float64(txnWatcherShortWait))
	TxnWatcherErrorShortWait = time.Duration(1.1 * float64(txnWatcherErrorShortWait))
)

var OutOfSyncError = outOfSyncError{}

func NewTestHubWatcher(hub HubSource, clock Clock, modelUUID string, logger Logger) (*HubWatcher, <-chan struct{}) {
	return newHubWatcher(hub, clock, modelUUID, logger)
}
