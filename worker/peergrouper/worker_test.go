package peergrouper

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/worker"
)

type workerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestStartStop(c *gc.C) {
	w, err := New(s.State)
	c.Assert(err, gc.IsNil)
	err = worker.Stop(w)
	c.Assert(err, gc.IsNil)
}

//func (s *workerSuite) {
//}
