// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"time"

	"github.com/juju/worker/v2"
)

// RestartDelay holds the length of time that a worker
// will wait between exiting and restarting.
const RestartDelay = 3 * time.Second

// Runner is implemented by instances capable of starting and stopping workers.
type Runner interface {
	worker.Worker
	StartWorker(id string, startFunc func() (worker.Worker, error)) error
	StopWorker(id string) error
}
