// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"testing"

	"github.com/juju/worker/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

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
