package state

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/testing"
	"net/url"
	"time"
)

type RelationSuite struct{}

var _ = Suite(&RelationSuite{})

// TestRelatedEndpoints verifies the behaviour of RelatedEndpoints in
// multi-endpoint peer relations, which are currently not constructable
// by normal means.
func (s *RelationSuite) TestRelatedEndpoints(c *C) {
	r := &Relation{nil, "", []RelationEndpoint{
		RelationEndpoint{"jeff", "ifce", "group", RolePeer, ScopeGlobal},
		RelationEndpoint{"mike", "ifce", "group", RolePeer, ScopeGlobal},
		RelationEndpoint{"bill", "ifce", "group", RolePeer, ScopeGlobal},
	}}
	eps, err := r.RelatedEndpoints("mike")
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []RelationEndpoint{
		RelationEndpoint{"jeff", "ifce", "group", RolePeer, ScopeGlobal},
		RelationEndpoint{"mike", "ifce", "group", RolePeer, ScopeGlobal},
		RelationEndpoint{"bill", "ifce", "group", RolePeer, ScopeGlobal},
	})
}

type UnitScopePathSuite struct {
	testing.ZkConnSuite
}

var _ = Suite(&UnitScopePathSuite{})

func (s *UnitScopePathSuite) TestPaths(c *C) {
	usp := unitScopePath("/path/to/scope")
	c.Assert(usp.settingsPath("u-551"), Equals, "/path/to/scope/settings/u-551")
	c.Assert(usp.presencePath(RolePeer, "u-551"), Equals, "/path/to/scope/peer/u-551")
}

func (s *UnitScopePathSuite) TestPrepareJoin(c *C) {
	usp := unitScopePath("/scope")
	err := usp.prepareJoin(s.ZkConn, RoleRequirer)
	c.Assert(err, IsNil)
	stat, err := s.ZkConn.Exists("/scope/requirer")
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
	stat, err = s.ZkConn.Exists("/scope/settings")
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
}

type RelationUnitsWatcherSuite struct {
	// Can't use state/testing.StateSuite; import cycle. The duplication
	// here is irritating, but probably not as unhelpful as it would be to
	// have to export unitScopePath and relationUnitsWatcher, neither of
	// which have a useful existence independent of the rest of the state
	// package.
	testing.ZkSuite
	st    *State
	units []*Unit
}

var _ = Suite(&RelationUnitsWatcherSuite{})

func (s *RelationUnitsWatcherSuite) SetUpTest(c *C) {
	info := &Info{Addrs: []string{testing.ZkAddr}}
	st, err := Initialize(info)
	c.Assert(err, IsNil)
	s.st = st
	ch := testing.Charms.Dir("dummy")
	ident := fmt.Sprintf("dummy-%d", ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := s.st.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	srv, err := s.st.AddService("srv", sch)
	c.Assert(err, IsNil)
	for i := 0; i < 3; i++ {
		unit, err := srv.AddUnit()
		c.Assert(err, IsNil)
		s.units = append(s.units, unit)
	}
}

func (s *RelationUnitsWatcherSuite) TearDownTest(c *C) {
	c.Assert(s.st.Close(), IsNil)
	s.ZkSuite.TearDownTest(c)
}

func kill(c *C, p *presence.Pinger) {
	select {
	case <-p.Dying():
	default:
		c.Assert(p.Kill(), IsNil)
	}
}

func (s *RelationUnitsWatcherSuite) TestWatcher(c *C) {
	// Create a totally arbitrary scope and role, and watch it on
	// behalf of the first unit.
	_, err := s.st.zk.Create("/some-scope-path", "", 0, zkPermAll)
	c.Assert(err, IsNil)
	role := RelationRole("dummy")
	scope := unitScopePath("/some-scope-path")
	w := newRelationUnitsWatcher(scope, role, s.units[0])

	// Check empty initial event, and no followup.
	assertChange := func(expect RelationUnitsChange) {
		select {
		case ch, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(ch, DeepEquals, expect)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("expected %#v, got nothing", expect)
		}
	}
	assertChange(RelationUnitsChange{})
	assertNoChange := func() {
		select {
		case ch := <-w.Changes():
			c.Fatalf("expected nothing, got %#v", ch)
		case <-time.After(200 * time.Millisecond):
		}
	}
	assertNoChange()

	// Create required paths, and settings nodes for all the units; check
	// nothing detected.
	err = scope.prepareJoin(s.st.zk, role)
	c.Assert(err, IsNil)
	changeSettings := func(u *Unit) {
		node, err := readConfigNode(s.st.zk, scope.settingsPath(u.key))
		c.Assert(err, IsNil)
		value, _ := node.Get("value")
		v, _ := value.(int)
		node.Set("value", v+1)
		_, err = node.Write()
		c.Assert(err, IsNil)
	}
	for _, u := range s.units {
		changeSettings(u)
	}
	assertNoChange()

	// Create a presence node for the watching unit; check nothing detected.
	startPinger := func(u *Unit) *presence.Pinger {
		p, err := presence.StartPinger(
			s.st.zk, scope.presencePath(role, u.key), agentPingerPeriod,
		)
		c.Assert(err, IsNil)
		return p
	}
	p0 := startPinger(s.units[0])
	defer kill(c, p0)
	assertNoChange()

	// Change the watching unit's settings; check nothing detected.
	changeSettings(s.units[0])
	assertNoChange()

	// Vacate presence node, check still no changes.
	err = p0.Kill()
	c.Assert(err, IsNil)
	assertNoChange()

	// Create a presence node for another unit; check detected.
	p1 := startPinger(s.units[1])
	defer kill(c, p1)
	expect := RelationUnitsChange{Changed: map[string]UnitSettings{
		"srv/1": UnitSettings{0, map[string]interface{}{"value": 1}},
	}}
	assertChange(expect)
	assertNoChange()

	// Change its settings; check also detected.
	changeSettings(s.units[1])
	expect = RelationUnitsChange{Changed: map[string]UnitSettings{
		"srv/1": UnitSettings{1, map[string]interface{}{"value": 2}},
	}}
	assertChange(expect)
	assertNoChange()

	// Create a presence node for yet another unit; check detected.
	p2 := startPinger(s.units[2])
	defer kill(c, p2)
	expect = RelationUnitsChange{Changed: map[string]UnitSettings{
		"srv/2": UnitSettings{0, map[string]interface{}{"value": 1}},
	}}
	assertChange(expect)
	assertNoChange()

	// Vacate a unit's settings node, check detected.
	err = p1.Kill()
	c.Assert(err, IsNil)
	expect = RelationUnitsChange{Departed: []string{"srv/1"}}
	assertChange(expect)
	assertNoChange()

	// Stop the watcher and check changes channel is closed.
	err = w.Stop()
	c.Assert(err, IsNil)
	assertClosed := func() {
		select {
		case _, ok := <-w.Changes():
			c.Assert(ok, Equals, false)
		default:
		}
	}
	assertClosed()

	// Make another couple of settings changes.
	changeSettings(s.units[1])
	changeSettings(s.units[2])

	// Start a new watcher, check initial event.
	w = newRelationUnitsWatcher(scope, role, s.units[0])
	expect = RelationUnitsChange{Changed: map[string]UnitSettings{
		"srv/2": UnitSettings{1, map[string]interface{}{"value": 2}},
	}}
	assertChange(expect)
	assertNoChange()

	// Kill remaining pinger, check event.
	err = p2.Kill()
	c.Assert(err, IsNil)
	expect = RelationUnitsChange{Departed: []string{"srv/2"}}
	assertChange(expect)
	assertNoChange()

	// Stop the watcher again, check closed.
	err = w.Stop()
	c.Assert(err, IsNil)
	assertClosed()
}
