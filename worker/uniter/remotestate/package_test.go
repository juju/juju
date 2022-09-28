// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"testing"

	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func SecretRotateWatcher(w *RemoteStateWatcher) worker.Worker {
	return w.secretRotateWatcher
}

func SecretExpiryWatcherFunc(w *RemoteStateWatcher) worker.Worker {
	return w.secretExpiryWatcher
}
