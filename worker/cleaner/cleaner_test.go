// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

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

var _ = gc.Suite(&CleanerSuite{})

var _ worker.NotifyWatchHandler = (*cleaner.Cleaner)(nil)

func (s *CleanerSuite) TestCleaner(c *gc.C) {
	cr := cleaner.NewCleaner(s.State)
	defer func() { c.Assert(worker.Stop(cr), gc.IsNil) }()

	needed, err := s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(needed, gc.Equals, false)

	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	relM, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	needed, err = s.State.NeedsCleanup()
	c.Assert(err, gc.IsNil)
	c.Assert(needed, gc.Equals, false)

	// Observe destroying of the relation with a watcher.
	cw := s.State.WatchCleanups()
	defer func() { c.Assert(cw.Stop(), gc.IsNil) }()

	err = relM.Destroy()
	c.Assert(err, gc.IsNil)

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
			c.Assert(err, gc.IsNil)
			if needed {
				continue
			}
		}
		break
	}
}
