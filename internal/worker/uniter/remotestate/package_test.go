// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
)


func SecretRotateWatcher(w *RemoteStateWatcher) worker.Worker {
	return w.secretRotateWatcher
}

func SecretExpiryWatcherFunc(w *RemoteStateWatcher) worker.Worker {
	return w.secretExpiryWatcher
}
