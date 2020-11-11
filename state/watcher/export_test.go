// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

const (
	TxnWatcherShortWait      = txnWatcherShortWait
	TxnWatcherErrorShortWait = txnWatcherErrorShortWait
)

var OutOfSyncError = outOfSyncError{}

func NewTestHubWatcher(hub HubSource, clock Clock, modelUUID string, logger Logger) (*HubWatcher, <-chan struct{}) {
	return newHubWatcher(hub, clock, modelUUID, logger)
}
