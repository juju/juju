// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/cleaner"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type CleanerSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&CleanerSuite{})

var _ worker.Worker = (*cleaner.Cleaner)(nil)

func (s *CleanerSuite) TestCleaner(c *C) {
	cr := cleaner.NewCleaner(s.State)
	defer func() { c.Assert(cr.Stop(), IsNil) }()

	needed, err := s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(needed, Equals, false)

	_, err = s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	needed, err = s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(needed, Equals, false)

	// Observe destroying of the relation with a watcher. After waiting
	// for the initial event the relation is destroyed. This raises one
	// event for the destroying and one for the cleanup.
	cw := s.State.WatchCleanups()
	defer func() { c.Assert(cw.Stop(), IsNil) }()

	_, ok := <-cw.Changes()
	c.Assert(ok, Equals, true)

	err = relM.Destroy()
	c.Assert(err, IsNil)

	s.State.StartSync()
	_, ok = <-cw.Changes()
	c.Assert(ok, Equals, true)
	_, ok = <-cw.Changes()
	c.Assert(ok, Equals, true)

	// Cleanup is done, so not needed anymore.
	needed, err = s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(needed, Equals, false)
}
