// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	jworker "github.com/juju/juju/worker"
)

type FinishedSuite struct{}

var _ = gc.Suite(&FinishedSuite{})

func (s *FinishedSuite) TestFinishedWorker(c *gc.C) {
	// Pretty dumb test if interface is implemented
	// and Wait() returns nil.
	var fw worker.Worker = jworker.FinishedWorker{}
	c.Assert(fw.Wait(), gc.IsNil)
}
