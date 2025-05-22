// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"github.com/juju/worker/v4"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
)

type dummyWorker struct {
	worker.Worker
}

type dummyAPICaller struct {
	base.APICaller
}

type stubWorker struct {
	worker.Worker
}

type stubFlagWorker struct {
	engine.Flag
	worker.Worker
}
