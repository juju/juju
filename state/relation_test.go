package state

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/state/presence"
	"time"
)

type RelationUnitWatcherSuite struct {
	zkConn            *zookeeper.Conn
	agentPingerPeriod time.Duration
}

var _ = Suite(&RelationUnitWatcherSuite{})

func (s *RelationUnitWatcherSuite) SetUpSuite(c *C) {
	s.agentPingerPeriod = agentPingerPeriod
	agentPingerPeriod = testPingerPeriod
	st, err := Initialize(&Info{
		Addrs: []string{TestingZkAddr},
	})
	c.Assert(err, IsNil)
	s.zkConn = ZkConn(st)
}

func (s *RelationUnitWatcherSuite) TearDownSuite(c *C) {
	agentPingerPeriod = s.agentPingerPeriod
	err := zkRemoveTree(s.zkConn, "/")
	c.Assert(err, IsNil)
	s.zkConn.Close()
}

var (
	testPingerPeriod = 50 * time.Millisecond
	shortTimeout     = 2 * testPingerPeriod
	longTimeout      = 5 * testPingerPeriod
)

func (s *RelationUnitWatcherSuite) TestRelationUnitWatcher(c *C) {
	waitFor := func(w *relationUnitWatcher, timeout time.Duration, expectChange *relationUnitChange) {
		select {
		case <-time.After(timeout):
			if expectChange != nil {
				c.Fatalf("expected change, got none")
			}
		case change, ok := <-w.Changes():
			if expectChange != nil {
				c.Assert(ok, Equals, true)
				c.Assert(change, DeepEquals, *expectChange)
			} else if ok {
				c.Fatalf("unexpected presence change")
			}
		}
	}

	// Start watcher; check initial event.
	w := newRelationUnitWatcher(s.zkConn, "/collection", "u-123", RolePeer)
	waitFor(w, shortTimeout, &relationUnitChange{false, ""})

	// Create all relevant paths apart from presence node; check that
	// no events occur (settings watch should not be active, because
	// presence has not yet been detected).
	usp := unitScopePath("/collection")
	err := usp.prepareJoin(s.zkConn, RolePeer)
	c.Assert(err, IsNil)
	settingsPath := usp.settingsPath("u-123")
	_, err = s.zkConn.Create(settingsPath, "whatever", 0, zkPermAll)
	writeSettings := func(content string) {
		_, err = s.zkConn.Set(settingsPath, content, -1)
		c.Assert(err, IsNil)
	}
	writeSettings("something")
	waitFor(w, shortTimeout, nil)

	// Start a pinger on the presence node; check event.
	presencePath := usp.presencePath(RolePeer, "u-123")
	startPinger := func() *presence.Pinger {
		pinger, err := presence.StartPinger(s.zkConn, presencePath, agentPingerPeriod)
		c.Assert(err, IsNil)
		return pinger
	}
	pinger := startPinger()
	waitFor(w, shortTimeout, &relationUnitChange{true, "something"})

	// Write identical settings; check event.
	writeSettings("something")
	waitFor(w, shortTimeout, nil)

	// Write new settings; check event.
	writeSettings("different")
	waitFor(w, shortTimeout, &relationUnitChange{true, "different"})

	// Stop updating the presence node; but also slip in a subsequent settings
	// change, which will still be detected before the absence is detected.
	err = pinger.Stop()
	c.Assert(err, IsNil)
	writeSettings("alternative")
	c.Assert(err, IsNil)
	waitFor(w, shortTimeout, &relationUnitChange{true, "alternative"})
	waitFor(w, longTimeout, &relationUnitChange{false, ""})

	// Change settings again; check no event.
	writeSettings("sneaky")
	waitFor(w, shortTimeout, nil)

	// Start a new pinger; check that presence and settings changes are sent.
	pinger = startPinger()
	c.Assert(err, IsNil)
	waitFor(w, shortTimeout, &relationUnitChange{true, "sneaky"})

	// Stop the watcher; perturb the nodes; check no further events.
	err = w.Stop()
	c.Assert(err, IsNil)
	writeSettings("bizarre")
	waitFor(w, shortTimeout, nil)
	err = pinger.Kill()
	c.Assert(err, IsNil)
	waitFor(w, shortTimeout, nil)

	// Start a new pinger; start a new watcher; check event.
	pinger = startPinger()
	w = newRelationUnitWatcher(s.zkConn, "/collection", "u-123", RolePeer)
	waitFor(w, shortTimeout, &relationUnitChange{true, "bizarre"})
	err = w.Stop()
	c.Assert(err, IsNil)
	err = pinger.Kill()
	c.Assert(err, IsNil)

	// Final check that no spurious changes have been sent.
	waitFor(w, shortTimeout, nil)
}

type UnitScopePathSuite struct {
	zkConn *zookeeper.Conn
}

var _ = Suite(&UnitScopePathSuite{})

func (s *UnitScopePathSuite) SetUpSuite(c *C) {
	st, err := Initialize(&Info{
		Addrs: []string{TestingZkAddr},
	})
	c.Assert(err, IsNil)
	s.zkConn = ZkConn(st)
}

func (s *UnitScopePathSuite) TearDownSuite(c *C) {
	err := zkRemoveTree(s.zkConn, "/")
	c.Assert(err, IsNil)
	s.zkConn.Close()
}

func (s *UnitScopePathSuite) TestPaths(c *C) {
	usp := unitScopePath("/path/to/scope")
	c.Assert(usp.settingsPath("u-551"), Equals, "/path/to/scope/settings/u-551")
	c.Assert(usp.presencePath(RolePeer, "u-551"), Equals, "/path/to/scope/peer/u-551")
}

func (s *UnitScopePathSuite) TestPrepareJoin(c *C) {
	usp := unitScopePath("/scope")
	err := usp.prepareJoin(s.zkConn, RoleRequirer)
	c.Assert(err, IsNil)
	stat, err := s.zkConn.Exists("/scope/requirer")
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
	stat, err = s.zkConn.Exists("/scope/settings")
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
}
