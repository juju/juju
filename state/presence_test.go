package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/state"
	"time"
)

type PresenceSuite struct {
	CommonSuite
}

var _ = Suite(&PresenceSuite{})

func (s *PresenceSuite) TestChangeNode(c *C) {
	// Set up infrastructure.
	path := "/change"
	stat, err := s.zkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)

	// Check a node gets created.
	node, err := state.NewChangeNode(s.zkConn, path)
	c.Assert(err, IsNil)
	_, _, watch, err := s.zkConn.GetW(path)
	c.Assert(err, IsNil)

	// Check watch fires on Change.
	t0, err := node.Change()
	c.Assert(err, IsNil)
	<-watch

	// Check another Change fires another watch, and returns a later time.
	time.Sleep(1e7)
	_, _, watch, err = s.zkConn.GetW(path)
	t1, err := node.Change()
	<-watch
	c.Assert(t1.After(t0), Equals, true)
}

func (s *PresenceSuite) TestPresenceNode(c *C) {
	// Set up infrastructure
	path := "/presence"
	timeout := time.Duration(5e7) // 50ms
	longEnough := timeout * 2
	clock, err := state.NewChangeNode(s.zkConn, "/clock")
	c.Assert(err, IsNil)

	// Check that a fresh node is unoccupied.
	client := state.NewPresenceNodeClient(s.zkConn, path, timeout)
	occupied, err := client.Occupied(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, false)

	// Check that OccupiedW agrees...
	occupied, watch, err := client.OccupiedW(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, false)

	// ...and that the watch fires when it gets occupied.
	node := state.NewPresenceNode(s.zkConn, path, timeout)
	done := node.Occupy()
	occupied = <-watch
	c.Assert(occupied, Equals, true)

	// Check that Occupied sees the node is now occupied...
	occupied, err = client.Occupied(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, true)

	// ...and that OccupiedW agrees...
	occupied, watch, err = client.OccupiedW(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, true)

	// ...and that the watch doesn't fire while the node remains occupied.
	t := time.NewTimer(longEnough)
	select {
	case <-watch:
		c.Log("occupation broke early")
		c.Fail()
	case <-t.C:
		// cool, didn't break
	}
	t.Stop()

	// Vacate the node; check that the watch fires...
	done <- true
	t = time.NewTimer(longEnough)
	select {
	case occupied = <-watch:
		c.Assert(occupied, Equals, false)
	case <-t.C:
		c.Log("unoccupation not detected")
		c.Fail()
	}
	t.Stop()

	// ...and that the node has been deleted...
	stat, err := s.zkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, IsNil)

	// ...and that Occupied still agrees.
	occupied, err = client.Occupied(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, false)
}

func (s *PresenceSuite) TestPresenceNodeWithoutDelete(c *C) {
	// Create a PresenceNodeClient with a much shorter timeout than the
	// associated PresenceNode, to verify that unoccupation watch still fires
	// when the PresenceNode is not updating often enough (and not just when
	// it's deleted, as in TestPresenceNode).

	// Set up infrastructure.
	path := "/presence"
	timeout := time.Duration(5e7) // 50ms
	nodeTimeout := timeout * 100
	clock, err := state.NewChangeNode(s.zkConn, "/clock")
	c.Assert(err, IsNil)

	// Wait for the client to see the node.
	client := state.NewPresenceNodeClient(s.zkConn, path, timeout)
	occupied, watch, err := client.OccupiedW(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, false)
	node := state.NewPresenceNode(s.zkConn, path, nodeTimeout)
	done := node.Occupy()
	occupied = <-watch
	c.Assert(occupied, Equals, true)

	// Watch again, wait for timeout.
	occupied, watch, err = client.OccupiedW(clock)
	c.Assert(err, IsNil)
	c.Assert(occupied, Equals, true)
	occupied = <-watch
	c.Assert(occupied, Equals, false)

	// Check the node really is still there.
	stat, err := s.zkConn.Exists(path)
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)

	// Tidy up.
	done <- true
}
