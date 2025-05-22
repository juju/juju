// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"

	jworker "github.com/juju/juju/internal/worker"
)

type FinishedSuite struct{}

func TestFinishedSuite(t *stdtesting.T) {
	tc.Run(t, &FinishedSuite{})
}

func (s *FinishedSuite) TestFinishedWorker(c *tc.C) {
	// Pretty dumb test if interface is implemented
	// and Wait() returns nil.
	var fw worker.Worker = jworker.FinishedWorker{}
	c.Assert(fw.Wait(), tc.IsNil)
}
