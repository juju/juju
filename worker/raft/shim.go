// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import "gopkg.in/juju/worker.v1"

// NewWorkerShim is suitable for use in ManifoldConfig.NewWorker,
// and simply calls through to NewWorker.
func NewWorkerShim(config Config) (worker.Worker, error) {
	return NewWorker(config)
}
