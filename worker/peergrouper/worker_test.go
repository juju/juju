package peergrouper_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/peergrouper"
)

type workerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestStartStop(c *gc.C) {
	w, err := peergrouper.New(s.State)
	c.Assert(err, gc.IsNil)
	err = worker.Stop(w)
	c.Assert(err, gc.IsNil)
}

//how can we test that it works for real?
//
//we can check that replicaset.Set is called with the expected
//arguments, but the we have a single port for all the machines.
//
//we could make it all operate on an interface:
