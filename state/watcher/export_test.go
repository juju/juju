// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	"github.com/juju/juju/core/logger"
)

const (
	TxnWatcherErrorWait = time.Duration(1.1 * float64(txnWatcherErrorWait))
)

func NewTestHubWatcher(hub HubSource, clock Clock, modelUUID string, logger logger.Logger) (*HubWatcher, <-chan struct{}) {
	return newHubWatcher(hub, clock, modelUUID, logger)
}
