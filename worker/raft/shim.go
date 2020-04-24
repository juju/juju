// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import "github.com/juju/worker/v2"

// NewWorkerShim is suitable for use in ManifoldConfig.NewWorker,
// and simply calls through to NewWorker.
func NewWorkerShim(config Config) (worker.Worker, error) {
	return NewWorker(config)
}
