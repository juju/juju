// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"github.com/juju/juju/worker"
)

var NewManifoldWorker = &newManifoldWorker

func DummyTrackerWorker() worker.Worker {
	// yes, this is entirely unsafe to *use*. It's just to get something
	// of the right type to use in the manifold's Output tests.
	return &tracker{}
}
