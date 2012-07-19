package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
	"time"
)

type RelationSuite struct {
	ConnSuite
	charm *state.Charm
}

var _ = Suite(&RelationSuite{})

func (s *RelationSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *RelationSuite) TestAddRelationErrors(c *C) {
	req, err := s.State.AddService("req", s.charm)
	c.Assert(err, IsNil)
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, state.ScopeGlobal}

	// Check we can't add a relation until both services exist.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal}
	err = s.State.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": service with name "pro" not found`)
	assertNoRelations(c, req)
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)

	// Check that interfaces have to match.
	proep2 := state.RelationEndpoint{"pro", "other", "foo", state.RoleProvider, state.ScopeGlobal}
	err = s.State.AddRelation(proep2, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": endpoints do not relate`)
	assertNoRelations(c, pro)
	assertNoRelations(c, req)

	// Check a variety of surprising endpoint combinations.
	err = s.State.AddRelation(reqep)
	c.Assert(err, ErrorMatches, `can't add relation "req:bar": single endpoint must be a peer relation`)
	assertNoRelations(c, req)

	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	err = s.State.AddRelation(peerep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz req:bar": endpoints do not relate`)
	assertNoRelations(c, peer)
	assertNoRelations(c, req)

	err = s.State.AddRelation(peerep, peerep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz peer:baz": endpoints do not relate`)
	assertNoRelations(c, peer)

	err = s.State.AddRelation()
	c.Assert(err, ErrorMatches, `can't add relation "": can't relate 0 endpoints`)
	err = s.State.AddRelation(proep, reqep, peerep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar peer:baz": can't relate 3 endpoints`)
}

func (s *RelationSuite) TestProviderRequirerRelation(c *C) {
	req, err := s.State.AddService("req", s.charm)
	c.Assert(err, IsNil)
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)
	assertNoRelations(c, req)
	assertNoRelations(c, pro)

	// Add a relation, and check we can only do so once.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal}
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, state.ScopeGlobal}
	err = s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	err = s.State.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": relation already exists`)
	assertOneRelation(c, pro, 0, proep, reqep)
	assertOneRelation(c, req, 0, reqep, proep)

	// Remove the relation, and check it can't be removed again.
	err = s.State.RemoveRelation(proep, reqep)
	c.Assert(err, IsNil)
	assertNoRelations(c, pro)
	assertNoRelations(c, req)
	err = s.State.RemoveRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't remove relation "pro:foo req:bar": relation doesn't exist`)

	// Check that we can add it again if we want to; but this time,
	// give one of the endpoints container scope and check that both
	// resulting service relations get that scope.
	reqep.RelationScope = state.ScopeContainer
	err = s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	// After adding relation, make proep container-scoped as well, for
	// simplicity of testing.
	proep.RelationScope = state.ScopeContainer
	assertOneRelation(c, pro, 1, proep, reqep)
	assertOneRelation(c, req, 1, reqep, proep)
}

func (s *RelationSuite) TestPeerRelation(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	assertNoRelations(c, peer)

	// Add a relation, and check we can only do so once.
	err = s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	err = s.State.AddRelation(peerep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz": relation already exists`)
	assertOneRelation(c, peer, 0, peerep)

	// Remove the relation, and check it can't be removed again.
	err = s.State.RemoveRelation(peerep)
	c.Assert(err, IsNil)
	assertNoRelations(c, peer)
	err = s.State.RemoveRelation(peerep)
	c.Assert(err, ErrorMatches, `can't remove relation "peer:baz": relation doesn't exist`)
}

func assertNoRelations(c *C, srv *state.Service) {
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 0)
}

func assertOneRelation(c *C, srv *state.Service, relId int, endpoints ...state.RelationEndpoint) {
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 1)
	rel := rels[0]
	c.Assert(rel.Id(), Equals, relId)
	name := srv.Name()
	expectEp := endpoints[0]
	ep, err := rel.Endpoint(name)
	c.Assert(err, IsNil)
	c.Assert(ep, DeepEquals, expectEp)
	if len(endpoints) == 2 {
		expectEp = endpoints[1]
	}
	eps, err := rel.RelatedEndpoints(name)
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []state.RelationEndpoint{expectEp})
}

type RelationUnitSuite struct {
	ConnSuite
	charm *state.Charm
}

var _ = Suite(&RelationUnitSuite{})

func (s *RelationUnitSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *RelationUnitSuite) TestPeerRelation(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	err = s.State.AddRelation(peerep)
	c.Assert(err, IsNil)

	// Add some units to the service and set their private addresses.
	// (Private addresses should be set by their unit agents on
	// startup; this test does not include that, but Join expects
	// the information to be available, and uses it to populate the
	// relation settings node.)
	units := []*state.Unit{}
	for i := 0; i < 3; i++ {
		unit, err := peer.AddUnit()
		c.Assert(err, IsNil)
		err = unit.SetPrivateAddress(fmt.Sprintf("peer%d.example.com", i))
		c.Assert(err, IsNil)
		units = append(units, unit)
	}

	// Get the peer relation.
	rels, err := peer.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 1)
	rel := rels[0]

	// ---------- Single unit ----------

	// Start watching the relation from the perspective of the first unit.
	w0, err := rel.Watch(units[0])
	c.Assert(err, IsNil)
	defer stop(c, w0)
	assertChange(c, w0, state.RelationUnitsChange{})
	assertNoChange(c, w0)

	// Join the first unit to the relation, and change the settings, and
	// check that nothing apparently happens.
	p0, err := rel.Join(units[0])
	c.Assert(err, IsNil)
	defer kill(c, p0)
	settings0 := changeSettings(c, rel, units[0])
	assertNoChange(c, w0)

	// ---------- Two units ----------

	// Now join another unit to the relation...
	p1, err := rel.Join(units[1])
	c.Assert(err, IsNil)
	defer kill(c, p1)

	// ...and check that the first relation unit sees the change.
	settings1, err := rel.Settings(units[1])
	c.Assert(err, IsNil)
	expect := state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/1": state.UnitSettings{0, settings1.Map()},
	}}
	assertChange(c, w0, expect)
	assertNoChange(c, w0)

	// Start watching the relation from the perspective of the second unit,
	// and check that it sees the right state.
	w1, err := rel.Watch(units[1])
	c.Assert(err, IsNil)
	defer stop(c, w1)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/0": state.UnitSettings{1, settings0.Map()},
	}}
	assertChange(c, w1, expect)
	assertNoChange(c, w1)

	// ---------- Three units ----------

	// Whoa, it works. Ok, check the third unit's opinion of the state.
	w2, err := rel.Watch(units[2])
	c.Assert(err, IsNil)
	defer stop(c, w2)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/0": state.UnitSettings{1, settings0.Map()},
		"peer/1": state.UnitSettings{0, settings1.Map()},
	}}
	assertChange(c, w2, expect)
	assertNoChange(c, w2)

	// Join the third unit, and check the first and second units see it.
	p2, err := rel.Join(units[2])
	c.Assert(err, IsNil)
	defer kill(c, p2)
	settings2, err := rel.Settings(units[2])
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/2": state.UnitSettings{0, settings2.Map()},
	}}
	assertChange(c, w0, expect)
	assertNoChange(c, w0)
	assertChange(c, w1, expect)
	assertNoChange(c, w1)

	// Change the second unit's settings, and check that only
	// the first and third see changes.
	settings1 = changeSettings(c, rel, units[1])
	assertNoChange(c, w1)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/1": state.UnitSettings{1, settings1.Map()},
	}}
	assertChange(c, w0, expect)
	assertNoChange(c, w0)
	assertChange(c, w2, expect)
	assertNoChange(c, w2)

	// ---------- Two units again ----------

	// Depart the second unit, and check that the first and third detect it.
	err = p1.Kill()
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Departed: []string{"peer/1"}}
	assertChange(c, w0, expect)
	assertNoChange(c, w0)
	assertChange(c, w2, expect)
	assertNoChange(c, w2)

	// Change its settings, and check the others don't observe anything.
	settings1 = changeSettings(c, rel, units[1])
	assertNoChange(c, w0)
	assertNoChange(c, w2)

	// Check no spurious events showed up on the second unit's watch, and check
	// it closes cleanly.
	assertNoChange(c, w1)
	err = w1.Stop()
	c.Assert(err, IsNil)
	assertClosed(c, w1)

	// ---------- One unit again, briefly  ----------

	// Cause the first unit to silently depart, then bring it back, and
	// check its watcher observes no changes...
	err = p0.Stop()
	c.Assert(err, IsNil)
	defer kill(c, p0)
	p0, err = rel.Join(units[0])
	c.Assert(err, IsNil)
	defer kill(c, p0)
	assertNoChange(c, w0)

	// ...then check that the third unit didn't notice anything either.
	// Note that joining should have caused the private address to
	// be set again, but this should not cause an actual *change*
	// in the settings, so nothing should actually have been written,
	// and thus nothing should have been detected.
	assertNoChange(c, w2)

	// OK, we're done here. Cleanup, and error detection during same,
	// will be handled by the deferred depart() calls. Phew.
}

func kill(c *C, p *presence.Pinger) {
	select {
	case <-p.Dying():
	default:
		c.Assert(p.Kill(), IsNil)
	}
}

func stop(c *C, w *state.RelationUnitsWatcher) {
	select {
	case <-w.Dying():
	default:
		c.Assert(w.Stop(), IsNil)
	}
}

func changeSettings(c *C, r *state.Relation, u *state.Unit) *state.ConfigNode {
	node, err := r.Settings(u)
	c.Assert(err, IsNil)
	value, _ := node.Get("value")
	v, _ := value.(int)
	node.Set("value", v+1)
	_, err = node.Write()
	c.Assert(err, IsNil)
	return node
}

func assertChange(c *C, w *state.RelationUnitsWatcher, expect state.RelationUnitsChange) {
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(ch, DeepEquals, expect)
	case <-time.After(1000 * time.Millisecond):
		c.Fatalf("expected %#v, got nothing", expect)
	}
}

func assertNoChange(c *C, w *state.RelationUnitsWatcher) {
	select {
	case ch := <-w.Changes():
		c.Fatalf("got %#v, expected nothing", ch)
	case <-time.After(200 * time.Millisecond):
	}
}

func assertClosed(c *C, w *state.RelationUnitsWatcher) {
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, Equals, false)
	default:
	}
}
