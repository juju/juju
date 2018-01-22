// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport

import "gopkg.in/juju/worker.v1"

func NewWorkerShim(config Config) (worker.Worker, error) {
	return NewWorker(config)
}
