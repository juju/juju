// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/runner"
)

var (
	NewMeterStatusClient = &newMeterStatusClient
	NewRunner            = &newRunner
)

// Ensure hookContext is a runner.Context.
var _ runner.Context = (*limitedContext)(nil)

func PatchInit(w worker.Worker, init func()) {
	aw := w.(*activeStatusWorker)
	aw.init = init
}
