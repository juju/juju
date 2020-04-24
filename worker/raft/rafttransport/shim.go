// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttransport

import "github.com/juju/worker/v2"

// NewWorkerShim calls straight through to NewWorker. This exists
// only to adapt to the signature of ManifoldConfig.NewWorker.
func NewWorkerShim(config Config) (worker.Worker, error) {
	return NewWorker(config)
}
