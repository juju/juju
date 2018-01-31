// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/mgo"

	"github.com/juju/juju/mongo"
)

const (
	TxnWatcherStarting   = txnWatcherStarting
	TxnWatcherCollection = txnWatcherCollection
	TxnWatcherShortWait  = txnWatcherShortWait
)

func NewTestWatcher(changelog *mgo.Collection, iteratorFunc func() mongo.Iterator) *Watcher {
	return newWatcher(changelog, iteratorFunc)
}

func NewTestHubWatcher(hub HubSource, logger Logger) (*HubWatcher, <-chan struct{}) {
	return newHubWatcher(hub, logger)
}
