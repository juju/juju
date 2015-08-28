// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/worker"
)

// Runner is the portion of worker.Runner needed for Event handlers.
type Runner interface {
	// Start a worker using the provided func.
	StartWorker(id string, newWorker func() (worker.Worker, error)) error
	// Stop the identified worker.
	StopWorker(id string) error
}

func newRunner() worker.Runner {
	return worker.NewRunner(isFatal, func(err0, err1 error) bool {
		return moreImportant(err0, err1) == err0
	})
}
