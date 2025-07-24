// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"
)

func GetWorkerRunner(c *gc.C, w worker.Worker) *worker.Runner {
	updater, ok := w.(*updaterWorker)
	c.Assert(ok, gc.Equals, true)
	c.Assert(updater, gc.NotNil)
	return updater.runner
}
