// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/cleaner"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type CleanerSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&CleanerSuite{})

var _ worker.NotifyWatchHandler = (*cleaner.Cleaner)(nil)

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

	// Observe destroying of the relation with a watcher.
	cw := s.State.WatchCleanups()
	defer func() { c.Assert(cw.Stop(), IsNil) }()

	err = relM.Destroy()
	c.Assert(err, IsNil)

	timeout := time.After(coretesting.LongWait)
	for {
		s.State.StartSync()
		select {
		case <-time.After(coretesting.ShortWait):
			continue
		case <-timeout:
			c.Fatalf("timed out waiting for cleanup")
		case <-cw.Changes():
			needed, err = s.State.NeedsCleanup()
			c.Assert(err, IsNil)
			if needed {
				continue
			}
		}
		break
	}
}
