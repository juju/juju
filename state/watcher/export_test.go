// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
)

const (
	TxnWatcherStarting   = txnWatcherStarting
	TxnWatcherSyncErr    = txnWatcherSyncErr
	TxnWatcherCollection = txnWatcherCollection
	TxnWatcherShortWait  = txnWatcherShortWait
)

func NewTestWatcher(changelog *mgo.Collection, iteratorFunc func() mongo.Iterator) *Watcher {
	return newWatcher(changelog, iteratorFunc)
}

func NewTestHubWatcher(hub HubSource, clock Clock, modelUUID string, logger Logger) (*HubWatcher, <-chan struct{}) {
	return newHubWatcher(hub, clock, modelUUID, logger)
}
