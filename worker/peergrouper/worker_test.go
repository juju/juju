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
	w := peergrouper.New(s.State)
	err := worker.Stop(w)
	c.Assert(err, gc.IsNil)
}
