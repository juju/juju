// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

func SecretRotateWatcher(w *RemoteStateWatcher) worker.Worker {
	return w.secretRotateWatcher
}

func SecretExpiryWatcherFunc(w *RemoteStateWatcher) worker.Worker {
	return w.secretExpiryWatcher
}
