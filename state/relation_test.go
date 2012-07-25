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

func (s *RelationSuite) TestRelationErrors(c *C) {
	req, err := s.State.AddService("req", s.charm)
	c.Assert(err, IsNil)
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, state.ScopeGlobal}

	// Check we can't add a relation until both services exist.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal}
	_, err = s.State.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `cannot add relation "pro:foo req:bar": service with name "pro" not found`)
	assertNoRelations(c, req)
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)

	// Check that interfaces have to match.
	proep2 := state.RelationEndpoint{"pro", "other", "foo", state.RoleProvider, state.ScopeGlobal}
	_, err = s.State.AddRelation(proep2, reqep)
	c.Assert(err, ErrorMatches, `cannot add relation "pro:foo req:bar": endpoints do not relate`)
	assertNoRelations(c, pro)
	assertNoRelations(c, req)

	// Check a variety of surprising endpoint combinations.
	_, err = s.State.AddRelation(reqep)
	c.Assert(err, ErrorMatches, `cannot add relation "req:bar": single endpoint must be a peer relation`)
	assertNoRelations(c, req)

	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	_, err = s.State.AddRelation(peerep, reqep)
	c.Assert(err, ErrorMatches, `cannot add relation "peer:baz req:bar": endpoints do not relate`)
	assertNoRelations(c, peer)
	assertNoRelations(c, req)

	_, err = s.State.AddRelation(peerep, peerep)
	c.Assert(err, ErrorMatches, `cannot add relation "peer:baz peer:baz": endpoints do not relate`)
	assertNoRelations(c, peer)

	_, err = s.State.AddRelation()
	c.Assert(err, ErrorMatches, `cannot add relation "": cannot relate 0 endpoints`)
	_, err = s.State.AddRelation(proep, reqep, peerep)
	c.Assert(err, ErrorMatches, `cannot add relation "pro:foo req:bar peer:baz": cannot relate 3 endpoints`)

	_, err = s.State.Relation(peerep)
	c.Assert(err, ErrorMatches, `cannot get relation "peer:baz": relation does not exist`)
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
	rel, err := s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `cannot add relation "pro:foo req:bar": relation already exists`)
	assertOneRelation(c, pro, 0, proep, reqep)
	assertOneRelation(c, req, 0, reqep, proep)

	// Remove the relation, and check it can't be removed again.
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
	assertNoRelations(c, pro)
	assertNoRelations(c, req)
	err = s.State.RemoveRelation(rel)
	c.Assert(err, ErrorMatches, `cannot remove relation "pro:foo req:bar": not found`)

	// Check that we can add it again if we want to; but this time,
	// give one of the endpoints container scope and check that both
	// resulting service relations get that scope.
	reqep.RelationScope = state.ScopeContainer
	_, err = s.State.AddRelation(proep, reqep)
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
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(peerep)
	c.Assert(err, ErrorMatches, `cannot add relation "peer:baz": relation already exists`)
	assertOneRelation(c, peer, 0, peerep)

	// Remove the relation, and check it can't be removed again.
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
	assertNoRelations(c, peer)
	err = s.State.RemoveRelation(rel)
	c.Assert(err, ErrorMatches, `cannot remove relation "peer:baz": not found`)
}

func (s *RelationSuite) TestRemoveServiceRemovesRelations(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	err = s.State.RemoveService(peer)
	c.Assert(err, IsNil)
	_, err = s.State.Service("peer")
	c.Assert(err, ErrorMatches, `cannot get service "peer": service with name "peer" not found`)
	_, err = s.State.Relation(peerep)
	c.Assert(err, ErrorMatches, `cannot get relation "peer:baz": relation does not exist`)
	err = s.State.RemoveRelation(rel)
	c.Assert(err, ErrorMatches, `cannot remove relation "peer:baz": not found`)
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

func (s *RelationUnitSuite) TestRelationUnitJoinError(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	u, err := peer.AddUnit()
	c.Assert(err, IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, IsNil)
	err = peer.RemoveUnit(u)
	c.Assert(err, IsNil)
	_, err = ru.Join()
	c.Assert(err, ErrorMatches, `cannot join unit "peer/0" to relation "peer:baz": unit has no private address`)
}

func (s *RelationUnitSuite) TestPeerRelationUnit(c *C) {
	// Create a service and get a peer relation.
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)

	// Add some units to the service and set their private addresses; get
	// the relevant RelationUnits.
	// (Private addresses should be set by their unit agents on
	// startup; this test does not include that, but Join expects
	// the information to be available, and uses it to populate the
	// relation settings node.)
	addUnit := func(i int) *state.RelationUnit {
		unit, err := peer.AddUnit()
		c.Assert(err, IsNil)
		err = unit.SetPrivateAddress(fmt.Sprintf("peer%d.example.com", i))
		c.Assert(err, IsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, IsNil)
		return ru
	}
	ru0 := addUnit(0)
	ru1 := addUnit(1)
	ru2 := addUnit(2)

	// ---------- Single unit ----------

	// Start watching the relation from the perspective of the first unit.
	w0 := ru0.Watch()
	defer stop(c, w0)
	assertChange(c, w0, state.RelationUnitsChange{})
	assertNoChange(c, w0)

	// Join the first unit to the relation, and change the settings, and
	// check that nothing apparently happens.
	p0, err := ru0.Join()
	c.Assert(err, IsNil)
	defer kill(c, p0)
	s0 := changeSettings(c, ru0)
	assertNoChange(c, w0)

	// ---------- Two units ----------

	// Now join another unit to the relation...
	p1, err := ru1.Join()
	c.Assert(err, IsNil)
	defer kill(c, p1)

	// ...and check that the first relation unit sees the change.
	s1, err := ru1.Settings()
	c.Assert(err, IsNil)
	expect := state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/1": state.UnitSettings{0, s1.Map()},
	}}
	assertChange(c, w0, expect)
	assertNoChange(c, w0)

	// Start watching the relation from the perspective of the second unit,
	// and check that it sees the right state.
	w1 := ru1.Watch()
	defer stop(c, w1)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/0": state.UnitSettings{1, s0.Map()},
	}}
	assertChange(c, w1, expect)
	assertNoChange(c, w1)

	// ---------- Three units ----------

	// Whoa, it works. Ok, check the third unit's opinion of the state.
	w2 := ru2.Watch()
	defer stop(c, w2)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/0": state.UnitSettings{1, s0.Map()},
		"peer/1": state.UnitSettings{0, s1.Map()},
	}}
	assertChange(c, w2, expect)
	assertNoChange(c, w2)

	// Join the third unit, and check the first and second units see it.
	p2, err := ru2.Join()
	c.Assert(err, IsNil)
	defer kill(c, p2)
	s2, err := ru2.Settings()
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/2": state.UnitSettings{0, s2.Map()},
	}}
	assertChange(c, w0, expect)
	assertNoChange(c, w0)
	assertChange(c, w1, expect)
	assertNoChange(c, w1)

	// Change the second unit's settings, and check that only
	// the first and third see changes.
	s1 = changeSettings(c, ru1)
	assertNoChange(c, w1)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"peer/1": state.UnitSettings{1, s1.Map()},
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
	s1 = changeSettings(c, ru1)
	assertNoChange(c, w0)
	assertNoChange(c, w2)

	// Check no spurious events showed up on the second unit's watch, and check
	// it closes cleanly.
	assertNoChange(c, w1)
	stop(c, w1)

	// ---------- One unit again, briefly  ----------

	// Cause the first unit to silently depart, then bring it back, and
	// check its watcher observes no changes...
	err = p0.Stop()
	c.Assert(err, IsNil)
	defer kill(c, p0)
	p0, err = ru0.Join()
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
	// will be handled by the deferred kill/stop calls. Phew.
}

func (s *RelationUnitSuite) TestGlobalProReqRelationUnit(c *C) {
	// Create a pair of services and a relation between them.
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)
	req, err := s.State.AddService("req", s.charm)
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(
		state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal},
		state.RelationEndpoint{"req", "ifce", "foo", state.RoleRequirer, state.ScopeGlobal},
	)
	c.Assert(err, IsNil)

	// Add some units to the services and set their private addresses.
	addUnit := func(srv *state.Service, sub string) *state.RelationUnit {
		unit, err := srv.AddUnit()
		c.Assert(err, IsNil)
		err = unit.SetPrivateAddress(fmt.Sprintf("%s.example.com", sub))
		c.Assert(err, IsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, IsNil)
		return ru
	}
	proru0 := addUnit(pro, "pro0")
	proru1 := addUnit(pro, "pro1")
	reqru0 := addUnit(req, "req0")
	reqru1 := addUnit(req, "req1")

	// ---------- Single role active ----------

	// Watch the relation from the perspective of the first provider unit and
	// check initial event.
	prow0 := proru0.Watch()
	defer stop(c, prow0)
	assertChange(c, prow0, state.RelationUnitsChange{})
	assertNoChange(c, prow0)

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	prop0, err := proru0.Join()
	c.Assert(err, IsNil)
	defer kill(c, prop0)
	pros0 := changeSettings(c, proru0)
	assertNoChange(c, prow0)

	// Join the second provider unit, start its watch, and check what it thinks the
	// state of the relation is.
	prop1, err := proru1.Join()
	c.Assert(err, IsNil)
	defer kill(c, prop1)
	prow1 := proru1.Watch()
	assertChange(c, prow1, state.RelationUnitsChange{})
	assertNoChange(c, prow1)

	// Change the unit's settings, and check that neither provider unit
	// observes any change.
	pros1 := changeSettings(c, proru1)
	assertNoChange(c, prow1)
	assertNoChange(c, prow0)

	// ---------- Two roles active ----------

	// Start watches from both requirer units' perspectives, and check that
	// they see the provider units.
	expect := state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"pro/0": state.UnitSettings{1, pros0.Map()},
		"pro/1": state.UnitSettings{1, pros1.Map()},
	}}
	reqw0 := reqru0.Watch()
	defer stop(c, reqw0)
	assertChange(c, reqw0, expect)
	assertNoChange(c, reqw0)
	reqw1 := reqru1.Watch()
	defer stop(c, reqw1)
	assertChange(c, reqw1, expect)
	assertNoChange(c, reqw1)

	// Join the first requirer unit, and check the provider units see it.
	reqp0, err := reqru0.Join()
	c.Assert(err, IsNil)
	defer kill(c, reqp0)
	reqs0, err := reqru0.Settings()
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"req/0": state.UnitSettings{0, reqs0.Map()},
	}}
	assertChange(c, prow0, expect)
	assertNoChange(c, prow0)
	assertChange(c, prow1, expect)
	assertNoChange(c, prow1)

	// Join the second requirer, and check the provider units see the change.
	reqp1, err := reqru1.Join()
	c.Assert(err, IsNil)
	defer kill(c, reqp1)
	reqs1, err := reqru1.Settings()
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"req/1": state.UnitSettings{0, reqs1.Map()},
	}}
	assertChange(c, prow0, expect)
	assertNoChange(c, prow0)
	assertChange(c, prow1, expect)
	assertNoChange(c, prow1)

	// Verify that neither requirer has observed any change to the relation.
	assertNoChange(c, reqw0)
	assertNoChange(c, reqw1)

	// Change settings for the first requirer, check providers see it...
	reqs0 = changeSettings(c, reqru0)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"req/0": state.UnitSettings{1, reqs0.Map()},
	}}
	assertChange(c, prow0, expect)
	assertNoChange(c, prow0)
	assertChange(c, prow1, expect)
	assertNoChange(c, prow1)

	// ...and requirers don't.
	assertNoChange(c, reqw0)
	assertNoChange(c, reqw1)

	// Depart the second requirer and check the providers see it...
	err = reqp1.Kill()
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Departed: []string{"req/1"}}
	assertChange(c, prow0, expect)
	assertNoChange(c, prow0)
	assertChange(c, prow1, expect)
	assertNoChange(c, prow1)

	// ...and the requirers don't.
	assertNoChange(c, reqw0)
	assertNoChange(c, reqw1)

	// ...and I think we're done. No need to further test Pinger response,
	// which is already handled in TestPeerRelation, because the important
	// differences between Join and Watch for different relations/units are
	// in what changes they cause or respond to, and I think we have enough
	// information to determine that the correct connections are in place.
	// Cleanup handled by defers as before.
}

func (s *RelationUnitSuite) TestContainerProReqRelationUnit(c *C) {
	// Create a pair of services and a relation between them.
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)
	req, err := s.State.AddService("req", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(
		state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal},
		state.RelationEndpoint{"req", "ifce", "foo", state.RoleRequirer, state.ScopeContainer},
	)
	c.Assert(err, IsNil)

	// Add some units to the services and set their private addresses.
	addUnits := func(i int) (*state.RelationUnit, *state.RelationUnit) {
		prou, err := pro.AddUnit()
		c.Assert(err, IsNil)
		err = prou.SetPrivateAddress(fmt.Sprintf("pro%d.example.com", i))
		c.Assert(err, IsNil)
		proru, err := rel.Unit(prou)
		c.Assert(err, IsNil)
		requ, err := req.AddUnitSubordinateTo(prou)
		c.Assert(err, IsNil)
		err = requ.SetPrivateAddress(fmt.Sprintf("req%d.example.com", i))
		c.Assert(err, IsNil)
		reqru, err := rel.Unit(requ)
		c.Assert(err, IsNil)
		return proru, reqru
	}
	proru0, reqru0 := addUnits(0)
	proru1, reqru1 := addUnits(1)

	// ---------- Single role active ----------

	// Start watching the relation from the perspective of the first unit, and
	// check the initial event.
	prow0 := proru0.Watch()
	defer stop(c, prow0)
	assertChange(c, prow0, state.RelationUnitsChange{})
	assertNoChange(c, prow0)

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	prop0, err := proru0.Join()
	c.Assert(err, IsNil)
	defer kill(c, prop0)
	pros0 := changeSettings(c, proru0)
	assertNoChange(c, prow0)

	// Watch the relation from the perspective of the second provider, and
	// check initial event.
	prow1 := proru1.Watch()
	defer stop(c, prow1)
	assertChange(c, prow1, state.RelationUnitsChange{})
	assertNoChange(c, prow1)

	// Join the second provider unit to the relation, and check that neither
	// watching unit observes any change.
	prop1, err := proru1.Join()
	c.Assert(err, IsNil)
	defer kill(c, prop1)
	assertNoChange(c, prow1)
	assertNoChange(c, prow0)

	// Change the unit's settings, and check that nothing apparently happens.
	pros1 := changeSettings(c, proru1)
	assertNoChange(c, prow1)
	assertNoChange(c, prow0)

	// ---------- Two roles active ----------

	// Start a watch from the first requirer unit's perspective, and check it
	// only sees the first provider (with which it shares a container).
	reqw0 := reqru0.Watch()
	defer stop(c, reqw0)
	expect := state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"pro/0": state.UnitSettings{1, pros0.Map()},
	}}
	assertChange(c, reqw0, expect)
	assertNoChange(c, reqw0)

	// Join the first requirer unit, and check that only the first provider
	// observes the change.
	reqp0, err := reqru0.Join()
	c.Assert(err, IsNil)
	defer kill(c, reqp0)
	reqs0, err := reqru0.Settings()
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"req/0": state.UnitSettings{0, reqs0.Map()},
	}}
	assertChange(c, prow0, expect)
	assertNoChange(c, prow0)
	assertNoChange(c, prow1)
	assertNoChange(c, reqw0)

	// Watch from the second requirer's perspective, and check it only sees the
	// second provider.
	reqw1 := reqru1.Watch()
	defer stop(c, reqw1)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"pro/1": state.UnitSettings{1, pros1.Map()},
	}}
	assertChange(c, reqw1, expect)
	assertNoChange(c, reqw1)

	// Join the second requirer, and check that the first provider observes it...
	reqp1, err := reqru1.Join()
	c.Assert(err, IsNil)
	defer kill(c, reqp1)
	reqs1, err := reqru1.Settings()
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"req/1": state.UnitSettings{0, reqs1.Map()},
	}}
	assertChange(c, prow1, expect)
	assertNoChange(c, prow1)

	// ...and that nothing else sees anything.
	assertNoChange(c, prow0)
	assertNoChange(c, reqw0)
	assertNoChange(c, reqw1)

	// Change the second provider's settings and check that the second
	// requirer notices...
	pros1 = changeSettings(c, proru1)
	expect = state.RelationUnitsChange{Changed: map[string]state.UnitSettings{
		"pro/1": state.UnitSettings{2, pros1.Map()},
	}}
	assertChange(c, reqw1, expect)
	assertNoChange(c, reqw1)

	// ...but that nothing else does.
	assertNoChange(c, prow0)
	assertNoChange(c, prow1)
	assertNoChange(c, prow0)

	// Finally, depart the first provider, and check that only the first
	// requirer observes any change.
	err = prop0.Kill()
	c.Assert(err, IsNil)
	expect = state.RelationUnitsChange{Departed: []string{"pro/0"}}
	assertChange(c, reqw0, expect)
	assertNoChange(c, reqw0)
	assertNoChange(c, reqw1)
	assertNoChange(c, prow0)
	assertNoChange(c, prow1)

	// Again, I think we're done, and can be comfortable that the appropriate
	// connections are in place.
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
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, Equals, false)
	default:
	}
}

func changeSettings(c *C, ru *state.RelationUnit) *state.ConfigNode {
	node, err := ru.Settings()
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
		if !ok {
			c.Fatalf("channel closed: watcher error %#v", w.Err())
		}
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
