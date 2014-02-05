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

func (s *workerSuite) 

how can we test that it works for real?

we can check that replicaset.Set is called with the expected
arguments, but the we have a single port for all the machines.

we could make it all operate on an interface:

type stateInterface interface {
	Machine(id string) (stateMachine, error)
	WatchStateServerInfo() state.NotifyWatcher
	StateServerInfo() (state.StateServerInfo, error)
}

type stateMachine interface {
	Refresh() error
	Watch() state.NotifyWatcher
	HasVote() bool
	SetHasVote(hasVote bool) error
	StateHostPort() string
}
