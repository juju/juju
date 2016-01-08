// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apidiscoverspaces "github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/discoverspaces"
)

type workerSuite struct {
	testing.JujuConnSuite

	Worker  worker.Worker
	OpsChan chan dummy.Operation

	APIConnection api.Connection
	API           *apidiscoverspaces.API
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Unbreak dummy provider methods.
	s.AssertConfigParameterUpdated(c, "broken", "")

	s.APIConnection, _ = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.API = s.APIConnection.DiscoverSpaces()

	//s.State.StartSync()

	s.OpsChan = make(chan dummy.Operation, 10)
	dummy.Listen(s.OpsChan)

	// Start the Addresser worker.
	s.Worker = discoverspaces.NewWorker(s.API)
}

func (s *workerSuite) TearDownTest(c *gc.C) {
	c.Assert(worker.Stop(s.Worker), jc.ErrorIsNil)
	s.JujuConnSuite.TearDownTest(c)
}
