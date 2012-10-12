package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"sort"
	"strings"
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
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, charm.ScopeGlobal}

	// Check we can't add a relation until both services exist.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}
	_, err = s.State.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `cannot add relation "pro:foo req:bar": .*`)
	assertNoRelations(c, req)
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)

	// Check that interfaces have to match.
	proep2 := state.RelationEndpoint{"pro", "other", "foo", state.RoleProvider, charm.ScopeGlobal}
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
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
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
	c.Assert(err, ErrorMatches, `cannot add relation "peer:baz pro:foo req:bar": cannot relate 3 endpoints`)
}

func (s *RelationSuite) TestRetrieveSuccess(c *C) {
	_, err := s.State.AddService("subway", s.charm)
	c.Assert(err, IsNil)
	_, err = s.State.AddService("mongo", s.charm)
	c.Assert(err, IsNil)
	subway := state.RelationEndpoint{"subway", "mongodb", "db", state.RoleRequirer, charm.ScopeGlobal}
	mongo := state.RelationEndpoint{"mongo", "mongodb", "server", state.RoleProvider, charm.ScopeGlobal}
	expect, err := s.State.AddRelation(subway, mongo)
	c.Assert(err, IsNil)
	rel, err := s.State.EndpointsRelation(subway, mongo)
	check := func() {
		c.Assert(err, IsNil)
		c.Assert(rel.Id(), Equals, expect.Id())
		c.Assert(rel.String(), Equals, expect.String())
	}
	check()
	rel, err = s.State.EndpointsRelation(mongo, subway)
	check()
	rel, err = s.State.Relation(expect.Id())
	check()
}

func (s *RelationSuite) TestRetrieveNotFound(c *C) {
	subway := state.RelationEndpoint{"subway", "mongodb", "db", state.RoleRequirer, charm.ScopeGlobal}
	mongo := state.RelationEndpoint{"mongo", "mongodb", "server", state.RoleProvider, charm.ScopeGlobal}
	_, err := s.State.EndpointsRelation(subway, mongo)
	c.Assert(err, ErrorMatches, `relation "mongo:server subway:db" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	_, err = s.State.Relation(999)
	c.Assert(err, ErrorMatches, `relation 999 not found`)
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *RelationSuite) TestProviderRequirerRelation(c *C) {
	req, err := s.State.AddService("req", s.charm)
	c.Assert(err, IsNil)
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)
	assertNoRelations(c, req)
	assertNoRelations(c, pro)

	// Add a relation, and check we can only do so once.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `cannot add relation "pro:foo req:bar": .*`)

	err = s.State.RemoveRelation(rel)
	c.Assert(err, ErrorMatches, `cannot remove relation "pro:foo req:bar": relation is not dead`)

	assertOneRelation(c, pro, 0, proep, reqep)
	assertOneRelation(c, req, 0, reqep, proep)
	testWhenDying(c, rel, noErr, noErr, func() error {
		assertOneRelation(c, pro, 0, proep, reqep)
		assertOneRelation(c, req, 0, reqep, proep)
		return nil
	})

	// Remove the relation, and check it's ok to remove again.
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)

	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)

	// Check that we can add it again if we want to; but this time,
	// give one of the endpoints container scope and check that both
	// resulting service relations get that scope.
	reqep.RelationScope = charm.ScopeContainer
	_, err = s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	// After adding relation, make proep container-scoped as well, for
	// simplicity of testing.
	proep.RelationScope = charm.ScopeContainer
	assertOneRelation(c, req, 2, reqep, proep)
	assertOneRelation(c, pro, 2, proep, reqep)
}

func (s *RelationSuite) TestRefresh(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	// Add a relation, and check we can only do so once.
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)

	rels, err := peer.Relations()
	c.Assert(err, IsNil)
	rel1 := rels[0]
	err = rel.EnsureDead()
	c.Assert(err, IsNil)

	c.Assert(rel1.Life(), Equals, state.Alive)
	err = rel1.Refresh()
	c.Assert(err, IsNil)
	c.Assert(rel1.Life(), Equals, state.Dead)

	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
	err = rel1.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *RelationSuite) TestPeerRelation(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
	assertNoRelations(c, peer)

	// Add a relation, and check we can only do so once.
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(peerep)
	c.Assert(err, ErrorMatches, `cannot add relation "peer:baz": .*`)
	assertOneRelation(c, peer, 0, peerep)

	// Remove the relation, and check it can't be removed again.
	err = rel.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
	assertNoRelations(c, peer)
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
}

func (s *RelationSuite) TestRemoveServiceRemovesRelations(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
	_, err = s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	err = peer.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(peer)
	c.Assert(err, IsNil)
	_, err = s.State.Service("peer")
	c.Assert(err, ErrorMatches, `service "peer" not found`)
	_, err = s.State.EndpointsRelation(peerep)
	c.Assert(err, ErrorMatches, `relation "peer:baz" not found`)
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
}

var _ = Suite(&RelationUnitSuite{})

func (s *RelationUnitSuite) TestPeerSettings(c *C) {
	pr := NewPeerRelation(c, &s.ConnSuite)
	rus := RUs{pr.ru0, pr.ru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("peer/0")
		c.Assert(err, ErrorMatches, `cannot read settings for unit "peer/0" in relation "peer:name": not found`)
	}

	// Add settings for one RU.
	err := pr.ru0.EnterScope()
	c.Assert(err, IsNil)
	node, err := pr.ru0.Settings()
	c.Assert(err, IsNil)
	node.Set("meme", "socially-awkward-penguin")
	_, err = node.Write()
	c.Assert(err, IsNil)

	// Check settings can be read by every RU.
	for _, ru := range rus {
		m, err := ru.ReadSettings("peer/0")
		c.Assert(err, IsNil)
		c.Assert(m["meme"], Equals, "socially-awkward-penguin")
	}
}

func (s *RelationUnitSuite) TestProReqSettings(c *C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	rus := RUs{prr.pru0, prr.pru1, prr.rru0, prr.rru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("pro/0")
		c.Assert(err, ErrorMatches, `cannot read settings for unit "pro/0" in relation "pro:pname req:rname": not found`)
	}

	// Add settings for one RU.
	err := prr.pru0.EnterScope()
	c.Assert(err, IsNil)
	node, err := prr.pru0.Settings()
	c.Assert(err, IsNil)
	node.Set("meme", "foul-bachelor-frog")
	_, err = node.Write()
	c.Assert(err, IsNil)

	// Check settings can be read by every RU.
	for _, ru := range rus {
		m, err := ru.ReadSettings("pro/0")
		c.Assert(err, IsNil)
		c.Assert(m["meme"], Equals, "foul-bachelor-frog")
	}
}

func (s *RelationUnitSuite) TestContainerSettings(c *C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	rus := RUs{prr.pru0, prr.pru1, prr.rru0, prr.rru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("pro/0")
		c.Assert(err, ErrorMatches, `cannot read settings for unit "pro/0" in relation "pro:pname req:rname": not found`)
	}

	// Add settings for one RU.
	err := prr.pru0.EnterScope()
	c.Assert(err, IsNil)
	node, err := prr.pru0.Settings()
	c.Assert(err, IsNil)
	node.Set("meme", "foul-bachelor-frog")
	_, err = node.Write()
	c.Assert(err, IsNil)

	// Check settings can be read by RUs in the same container.
	rus0 := RUs{prr.pru0, prr.rru0}
	for _, ru := range rus0 {
		m, err := ru.ReadSettings("pro/0")
		c.Assert(err, IsNil)
		c.Assert(m["meme"], Equals, "foul-bachelor-frog")
	}

	// Check settings are still inaccessible to RUs outside that container
	rus1 := RUs{prr.pru1, prr.rru1}
	for _, ru := range rus1 {
		_, err := ru.ReadSettings("pro/0")
		c.Assert(err, ErrorMatches, `cannot read settings for unit "pro/0" in relation "pro:pname req:rname": not found`)
	}
}

func (s *RelationUnitSuite) TestPeerWatchScope(c *C) {
	pr := NewPeerRelation(c, &s.ConnSuite)

	// Test empty initial event.
	w0 := pr.ru0.WatchScope()
	defer stop(c, w0)
	s.assertScopeChange(c, w0, nil, nil)
	s.assertNoScopeChange(c, w0)

	// ru0 enters; check no change, but private-address written.
	err := pr.ru0.EnterScope()
	c.Assert(err, IsNil)
	s.assertNoScopeChange(c, w0)
	node, err := pr.ru0.Settings()
	c.Assert(err, IsNil)
	c.Assert(node.Map(), DeepEquals, map[string]interface{}{"private-address": "peer-0.example.com"})

	// ru1 enters; check change is observed.
	err = pr.ru1.EnterScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, w0, []string{"peer/1"}, nil)
	s.assertNoScopeChange(c, w0)

	// ru1 enters again, check no problems and no changes.
	err = pr.ru1.EnterScope()
	c.Assert(err, IsNil)
	s.assertNoScopeChange(c, w0)

	// Stop watching; ru2 enters.
	stop(c, w0)
	err = pr.ru2.EnterScope()
	c.Assert(err, IsNil)

	// Start watch again, check initial event.
	w0 = pr.ru0.WatchScope()
	defer stop(c, w0)
	s.assertScopeChange(c, w0, []string{"peer/1", "peer/2"}, nil)
	s.assertNoScopeChange(c, w0)

	// ru1 leaves; check event.
	err = pr.ru1.LeaveScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, w0, nil, []string{"peer/1"})
	s.assertNoScopeChange(c, w0)

	// ru1 leaves again; check no problems and no changes.
	err = pr.ru1.LeaveScope()
	c.Assert(err, IsNil)
	s.assertNoScopeChange(c, w0)
}

func (s *RelationUnitSuite) TestProReqWatchScope(c *C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)

	// Test empty initial events for all RUs.
	ws := prr.watches()
	for _, w := range ws {
		defer stop(c, w)
	}
	for _, w := range ws {
		s.assertScopeChange(c, w, nil, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 enters; check detected only by req RUs.
	err := prr.pru0.EnterScope()
	c.Assert(err, IsNil)
	rws := func() []*state.RelationScopeWatcher {
		return []*state.RelationScopeWatcher{ws[2], ws[3]}
	}
	for _, w := range rws() {
		s.assertScopeChange(c, w, []string{"pro/0"}, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// req0 enters; check detected only by pro RUs.
	err = prr.rru0.EnterScope()
	c.Assert(err, IsNil)
	pws := func() []*state.RelationScopeWatcher {
		return []*state.RelationScopeWatcher{ws[0], ws[1]}
	}
	for _, w := range pws() {
		s.assertScopeChange(c, w, []string{"req/0"}, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// Stop watches; remaining RUs enter.
	for _, w := range ws {
		stop(c, w)
	}
	err = prr.pru1.EnterScope()
	c.Assert(err, IsNil)
	err = prr.rru1.EnterScope()
	c.Assert(err, IsNil)

	// Start new watches, check initial events.
	ws = prr.watches()
	for _, w := range ws {
		defer stop(c, w)
	}
	for _, w := range pws() {
		s.assertScopeChange(c, w, []string{"req/0", "req/1"}, nil)
	}
	for _, w := range rws() {
		s.assertScopeChange(c, w, []string{"pro/0", "pro/1"}, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 leaves; check detected only by req RUs.
	err = prr.pru0.LeaveScope()
	c.Assert(err, IsNil)
	for _, w := range rws() {
		s.assertScopeChange(c, w, nil, []string{"pro/0"})
	}
	s.assertNoScopeChange(c, ws...)

	// rru0 leaves; check detected only by pro RUs.
	err = prr.rru0.LeaveScope()
	c.Assert(err, IsNil)
	for _, w := range pws() {
		s.assertScopeChange(c, w, nil, []string{"req/0"})
	}
	s.assertNoScopeChange(c, ws...)
}

func (s *RelationUnitSuite) TestContainerWatchScope(c *C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)

	// Test empty initial events for all RUs.
	ws := prr.watches()
	for _, w := range ws {
		defer stop(c, w)
	}
	for _, w := range ws {
		s.assertScopeChange(c, w, nil, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 enters; check detected only by same-container req.
	err := prr.pru0.EnterScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, ws[2], []string{"pro/0"}, nil)
	s.assertNoScopeChange(c, ws...)

	// req1 enters; check detected only by same-container pro.
	err = prr.rru1.EnterScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, ws[1], []string{"req/1"}, nil)
	s.assertNoScopeChange(c, ws...)

	// Stop watches; remaining RUs enter scope.
	for _, w := range ws {
		stop(c, w)
	}
	err = prr.pru1.EnterScope()
	c.Assert(err, IsNil)
	err = prr.rru0.EnterScope()
	c.Assert(err, IsNil)

	// Start new watches, check initial events.
	ws = prr.watches()
	for _, w := range ws {
		defer stop(c, w)
	}
	s.assertScopeChange(c, ws[0], []string{"req/0"}, nil)
	s.assertScopeChange(c, ws[1], []string{"req/1"}, nil)
	s.assertScopeChange(c, ws[2], []string{"pro/0"}, nil)
	s.assertScopeChange(c, ws[3], []string{"pro/1"}, nil)
	s.assertNoScopeChange(c, ws...)

	// pru0 leaves; check detected only by same-container req.
	err = prr.pru0.LeaveScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, ws[2], nil, []string{"pro/0"})
	s.assertNoScopeChange(c, ws...)

	// rru0 leaves; check detected only by same-container pro.
	err = prr.rru0.LeaveScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, ws[0], nil, []string{"req/0"})
	s.assertNoScopeChange(c, ws...)
}

func (s *RelationUnitSuite) assertScopeChange(c *C, w *state.RelationScopeWatcher, entered, left []string) {
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		sort.Strings(entered)
		sort.Strings(ch.Entered)
		c.Assert(ch.Entered, DeepEquals, entered)
		sort.Strings(left)
		sort.Strings(ch.Left)
		c.Assert(ch.Left, DeepEquals, left)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("no change")
	}
}

func (s *RelationUnitSuite) assertNoScopeChange(c *C, ws ...*state.RelationScopeWatcher) {
	s.State.StartSync()
	for _, w := range ws {
		select {
		case ch, ok := <-w.Changes():
			c.Fatalf("got unwanted change: %#v, %t", ch, ok)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type PeerRelation struct {
	u0, u1, u2    *state.Unit
	ru0, ru1, ru2 *state.RelationUnit
}

func NewPeerRelation(c *C, s *ConnSuite) *PeerRelation {
	ch := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("peer", ch)
	c.Assert(err, IsNil)
	ep := state.RelationEndpoint{"peer", "ifce", "name", state.RolePeer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(ep)
	c.Assert(err, IsNil)
	pr := &PeerRelation{}
	pr.u0, pr.ru0 = addRU(c, svc, rel, nil)
	pr.u1, pr.ru1 = addRU(c, svc, rel, nil)
	pr.u2, pr.ru2 = addRU(c, svc, rel, nil)
	return pr
}

type ProReqRelation struct {
	pu0, pu1, ru0, ru1     *state.Unit
	pru0, pru1, rru0, rru1 *state.RelationUnit
}

func NewProReqRelation(c *C, s *ConnSuite, scope charm.RelationScope) *ProReqRelation {
	ch := s.AddTestingCharm(c, "dummy")
	psvc, err := s.State.AddService("pro", ch)
	c.Assert(err, IsNil)
	if scope == charm.ScopeContainer {
		ch = s.AddTestingCharm(c, "logging")
	}
	rsvc, err := s.State.AddService("req", ch)
	c.Assert(err, IsNil)
	pep := state.RelationEndpoint{"pro", "ifce", "pname", state.RoleProvider, scope}
	rep := state.RelationEndpoint{"req", "ifce", "rname", state.RoleRequirer, scope}
	rel, err := s.State.AddRelation(pep, rep)
	c.Assert(err, IsNil)
	prr := &ProReqRelation{}
	prr.pu0, prr.pru0 = addRU(c, psvc, rel, nil)
	prr.pu1, prr.pru1 = addRU(c, psvc, rel, nil)
	if scope == charm.ScopeGlobal {
		prr.ru0, prr.rru0 = addRU(c, rsvc, rel, nil)
		prr.ru1, prr.rru1 = addRU(c, rsvc, rel, nil)
	} else {
		prr.ru0, prr.rru0 = addRU(c, rsvc, rel, prr.pu0)
		prr.ru1, prr.rru1 = addRU(c, rsvc, rel, prr.pu1)
	}
	return prr
}

func (prr *ProReqRelation) watches() []*state.RelationScopeWatcher {
	return []*state.RelationScopeWatcher{
		prr.pru0.WatchScope(), prr.pru1.WatchScope(),
		prr.rru0.WatchScope(), prr.rru1.WatchScope(),
	}
}

func addRU(c *C, svc *state.Service, rel *state.Relation, principal *state.Unit) (*state.Unit, *state.RelationUnit) {
	var u *state.Unit
	var err error
	if principal == nil {
		u, err = svc.AddUnit()
	} else {
		u, err = svc.AddUnitSubordinateTo(principal)
	}
	c.Assert(err, IsNil)
	addr := fmt.Sprintf("%s.example.com", strings.Replace(u.Name(), "/", "-", 1))
	err = u.SetPrivateAddress(addr)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, IsNil)
	return u, ru
}

type RUs []*state.RelationUnit

type stopper interface {
	Stop() error
}

func stop(c *C, s stopper) {
	c.Assert(s.Stop(), IsNil)
}

type OriginalRelationUnitSuite struct {
	ConnSuite
	charm *state.Charm
}

var _ = Suite(&OriginalRelationUnitSuite{})

func (s *OriginalRelationUnitSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *OriginalRelationUnitSuite) TestRelationUnitEnterScopeError(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	u, err := peer.AddUnit()
	c.Assert(err, IsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, IsNil)
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = peer.RemoveUnit(u)
	c.Assert(err, IsNil)
	err = ru.EnterScope()
	c.Assert(err, ErrorMatches, `cannot initialize state for unit "peer/0" in relation "peer:baz": private address of unit "peer/0" not found`)
}

func (s *OriginalRelationUnitSuite) TestRelationUnitReadSettings(c *C) {
	// Create a peer service with a relation and two units.
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	u0, err := peer.AddUnit()
	c.Assert(err, IsNil)
	ru0, err := rel.Unit(u0)
	c.Assert(err, IsNil)
	u1, err := peer.AddUnit()
	c.Assert(err, IsNil)
	ru1, err := rel.Unit(u1)
	c.Assert(err, IsNil)

	// Check various errors.
	_, err = ru0.ReadSettings("nonsense")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "nonsense" in relation "peer:baz": "nonsense" is not a valid unit name`)
	_, err = ru0.ReadSettings("unknown/0")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "unknown/0" in relation "peer:baz": service "unknown" is not a member of "peer:baz"`)
	_, err = ru0.ReadSettings("peer/pressure")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "peer/pressure" in relation "peer:baz": "peer/pressure" is not a valid unit name`)
	_, err = ru0.ReadSettings("peer/1")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "peer/1" in relation "peer:baz": not found`)

	// Put some valid settings in ru1, and check they are now accessible to
	// both RelationUnits.
	err = u1.SetPrivateAddress("ru1.example.com")
	c.Assert(err, IsNil)
	err = ru1.EnterScope()
	c.Assert(err, IsNil)
	node, err := ru1.Settings()
	c.Assert(err, IsNil)
	node.Set("catchphrase", "eat my shorts")
	_, err = node.Write()
	c.Assert(err, IsNil)
	assertSetting := func(ru *state.RelationUnit, expect interface{}) {
		settings, err := ru.ReadSettings("peer/1")
		c.Assert(err, IsNil)
		c.Assert(settings["catchphrase"], Equals, expect)
	}
	assertSetting(ru0, "eat my shorts")
	assertSetting(ru1, "eat my shorts")

	// Delete the settings content, but not the node, and check that they
	// are still accessible without error.
	node.Delete("catchphrase")
	_, err = node.Write()
	c.Assert(err, IsNil)
	assertSetting(ru0, nil)
	assertSetting(ru1, nil)

	// TODO(fwer) handle relation removal
}

func (s *OriginalRelationUnitSuite) TestPeerRelationUnit(c *C) {
	// Create a service and get a peer relation.
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
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
		c.Assert(ru.Endpoint(), Equals, peerep)
		return ru
	}
	ru0 := addUnit(0)
	ru1 := addUnit(1)
	ru2 := addUnit(2)

	// ---------- Single unit ----------

	// Start watching the relation from the perspective of the first unit.
	w0 := ru0.Watch()
	defer stop(c, w0)
	s.assertChange(c, w0, nil, nil, nil)
	s.assertNoChange(c, w0)

	// Join the first unit to the relation, and change the settings, and
	// check that nothing apparently happens.
	err = ru0.EnterScope()
	c.Assert(err, IsNil)
	s0 := changeSettings(c, ru0)
	s.assertNoChange(c, w0)

	// ---------- Two units ----------

	// Now join another unit to the relation...
	err = ru1.EnterScope()
	c.Assert(err, IsNil)

	// ...and check that the first relation unit sees the change.
	s1, err := ru1.Settings()
	c.Assert(err, IsNil)
	expectChanged := map[string]map[string]interface{}{
		"peer/1": s1.Map(),
	}
	s.assertChange(c, w0, []string{"peer/1"}, expectChanged, nil)
	s.assertNoChange(c, w0)

	// Join again, check it's a no-op.
	err = ru1.EnterScope()
	c.Assert(err, IsNil)
	s.assertNoChange(c, w0)

	// Start watching the relation from the perspective of the second unit,
	// and check that it sees the right state.
	w1 := ru1.Watch()
	defer stop(c, w1)
	expectChanged = map[string]map[string]interface{}{
		"peer/0": s0.Map(),
	}
	s.assertChange(c, w1, []string{"peer/0"}, expectChanged, nil)
	s.assertNoChange(c, w1)

	// ---------- Three units ----------

	// Whoa, it works. Ok, check the third unit's opinion of the state.
	w2 := ru2.Watch()
	defer stop(c, w2)
	expectChanged = map[string]map[string]interface{}{
		"peer/0": s0.Map(),
		"peer/1": s1.Map(),
	}
	s.assertChange(c, w2, []string{"peer/0", "peer/1"}, expectChanged, nil)
	s.assertNoChange(c, w2)

	// Join the third unit, and check the first and second units see it.
	err = ru2.EnterScope()
	c.Assert(err, IsNil)
	s2, err := ru2.Settings()
	c.Assert(err, IsNil)
	expectChanged = map[string]map[string]interface{}{
		"peer/2": s2.Map(),
	}
	s.assertChange(c, w0, []string{"peer/2"}, expectChanged, nil)
	s.assertNoChange(c, w0)
	s.assertChange(c, w1, []string{"peer/2"}, expectChanged, nil)
	s.assertNoChange(c, w1)

	// Change the second unit's settings, and check that only
	// the first and third see changes.
	s1 = changeSettings(c, ru1)
	s.assertNoChange(c, w1)
	expectChanged = map[string]map[string]interface{}{
		"peer/1": s1.Map(),
	}
	s.assertChange(c, w0, nil, expectChanged, nil)
	s.assertNoChange(c, w0)
	s.assertChange(c, w2, nil, expectChanged, nil)
	s.assertNoChange(c, w2)

	// ---------- Two units again ----------

	// Depart the second unit, and check that the first and third detect it.
	err = ru1.LeaveScope()
	c.Assert(err, IsNil)
	expectDeparted := []string{"peer/1"}
	s.assertChange(c, w0, nil, nil, expectDeparted)
	s.assertNoChange(c, w0)
	s.assertChange(c, w2, nil, nil, expectDeparted)
	s.assertNoChange(c, w2)

	// Change its settings, and check the others don't observe anything.
	s1 = changeSettings(c, ru1)
	s.assertNoChange(c, w0)
	s.assertNoChange(c, w2)

	// Check no spurious events showed up on the second unit's watch, and check
	// it closes cleanly.
	s.assertNoChange(c, w1)
	stop(c, w1)

	// OK, we're done here. Cleanup, and error detection during same,
	// will be handled by the deferred kill/stop calls. Phew.
}

func (s *OriginalRelationUnitSuite) TestGlobalProReqRelationUnit(c *C) {
	// Create a pair of services and a relation between them.
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)
	req, err := s.State.AddService("req", s.charm)
	c.Assert(err, IsNil)
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)

	// Add some units to the services and set their private addresses.
	addUnit := func(srv *state.Service, sub string, ep state.RelationEndpoint) *state.RelationUnit {
		unit, err := srv.AddUnit()
		c.Assert(err, IsNil)
		err = unit.SetPrivateAddress(fmt.Sprintf("%s.example.com", sub))
		c.Assert(err, IsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, IsNil)
		c.Assert(ru.Endpoint(), Equals, ep)
		return ru
	}
	proru0 := addUnit(pro, "pro0", proep)
	proru1 := addUnit(pro, "pro1", proep)
	reqru0 := addUnit(req, "req0", reqep)
	reqru1 := addUnit(req, "req1", reqep)

	// ---------- Single role active ----------

	// Watch the relation from the perspective of the first provider unit and
	// check initial event.
	prow0 := proru0.Watch()
	defer stop(c, prow0)
	s.assertChange(c, prow0, nil, nil, nil)
	s.assertNoChange(c, prow0)

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	err = proru0.EnterScope()
	c.Assert(err, IsNil)
	pros0 := changeSettings(c, proru0)
	s.assertNoChange(c, prow0)

	// Join the second provider unit, start its watch, and check what it thinks the
	// state of the relation is.
	err = proru1.EnterScope()
	c.Assert(err, IsNil)
	prow1 := proru1.Watch()
	defer stop(c, prow1)
	s.assertChange(c, prow1, nil, nil, nil)
	s.assertNoChange(c, prow1)

	// Change the unit's settings, and check that neither provider unit
	// observes any change.
	pros1 := changeSettings(c, proru1)
	s.assertNoChange(c, prow1)
	s.assertNoChange(c, prow0)

	// ---------- Two roles active ----------

	// Start watches from both requirer units' perspectives, and check that
	// they see the provider units.
	expectJoined := []string{"pro/0", "pro/1"}
	expectChanged := map[string]map[string]interface{}{
		"pro/0": pros0.Map(),
		"pro/1": pros1.Map(),
	}
	reqw0 := reqru0.Watch()
	defer stop(c, reqw0)
	s.assertChange(c, reqw0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, reqw0)
	reqw1 := reqru1.Watch()
	defer stop(c, reqw1)
	s.assertChange(c, reqw1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, reqw1)

	// Join the first requirer unit, and check the provider units see it.
	err = reqru0.EnterScope()
	c.Assert(err, IsNil)
	reqs0, err := reqru0.Settings()
	c.Assert(err, IsNil)
	expectJoined = []string{"req/0"}
	expectChanged = map[string]map[string]interface{}{
		"req/0": reqs0.Map(),
	}
	s.assertChange(c, prow0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, prow0)
	s.assertChange(c, prow1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, prow1)

	// Join again, check no-op.
	err = reqru0.EnterScope()
	c.Assert(err, IsNil)
	s.assertNoChange(c, prow0)
	s.assertNoChange(c, prow1)

	// Join the second requirer, and check the provider units see the change.
	err = reqru1.EnterScope()
	c.Assert(err, IsNil)
	reqs1, err := reqru1.Settings()
	c.Assert(err, IsNil)
	expectJoined = []string{"req/1"}
	expectChanged = map[string]map[string]interface{}{
		"req/1": reqs1.Map(),
	}
	s.assertChange(c, prow0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, prow0)
	s.assertChange(c, prow1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, prow1)

	// Verify that neither requirer has observed any change to the relation.
	s.assertNoChange(c, reqw0)
	s.assertNoChange(c, reqw1)

	// Change settings for the first requirer, check providers see it...
	reqs0 = changeSettings(c, reqru0)
	expectChanged = map[string]map[string]interface{}{
		"req/0": reqs0.Map(),
	}
	s.assertChange(c, prow0, nil, expectChanged, nil)
	s.assertNoChange(c, prow0)
	s.assertChange(c, prow1, nil, expectChanged, nil)
	s.assertNoChange(c, prow1)

	// ...and requirers don't.
	s.assertNoChange(c, reqw0)
	s.assertNoChange(c, reqw1)

	// Depart the second requirer and check the providers see it...
	err = reqru1.LeaveScope()
	c.Assert(err, IsNil)
	expectDeparted := []string{"req/1"}
	s.assertChange(c, prow0, nil, nil, expectDeparted)
	s.assertNoChange(c, prow0)
	s.assertChange(c, prow1, nil, nil, expectDeparted)
	s.assertNoChange(c, prow1)

	// ...and the requirers don't.
	s.assertNoChange(c, reqw0)
	s.assertNoChange(c, reqw1)

	// Cleanup handled by defers as before.
}

func (s *OriginalRelationUnitSuite) TestContainerProReqRelationUnit(c *C) {
	// Create a pair of services and a relation between them.
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)
	req, err := s.State.AddService("req", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, charm.ScopeContainer}
	rel, err := s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)

	// Change proep to match the endpoint that will actually be used by the relation.
	proep.RelationScope = charm.ScopeContainer

	// Add some units to the services and set their private addresses.
	addUnits := func(i int) (*state.RelationUnit, *state.RelationUnit) {
		prou, err := pro.AddUnit()
		c.Assert(err, IsNil)
		err = prou.SetPrivateAddress(fmt.Sprintf("pro%d.example.com", i))
		c.Assert(err, IsNil)
		proru, err := rel.Unit(prou)
		c.Assert(err, IsNil)
		c.Assert(proru.Endpoint(), Equals, proep)
		requ, err := req.AddUnitSubordinateTo(prou)
		c.Assert(err, IsNil)
		err = requ.SetPrivateAddress(fmt.Sprintf("req%d.example.com", i))
		c.Assert(err, IsNil)
		reqru, err := rel.Unit(requ)
		c.Assert(err, IsNil)
		c.Assert(reqru.Endpoint(), Equals, reqep)
		return proru, reqru
	}
	proru0, reqru0 := addUnits(0)
	proru1, reqru1 := addUnits(1)

	// ---------- Single role active ----------

	// Start watching the relation from the perspective of the first unit, and
	// check the initial event.
	prow0 := proru0.Watch()
	defer stop(c, prow0)
	s.assertChange(c, prow0, nil, nil, nil)
	s.assertNoChange(c, prow0)

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	err = proru0.EnterScope()
	c.Assert(err, IsNil)
	pros0 := changeSettings(c, proru0)
	s.assertNoChange(c, prow0)

	// Watch the relation from the perspective of the second provider, and
	// check initial event.
	prow1 := proru1.Watch()
	defer stop(c, prow1)
	s.assertChange(c, prow1, nil, nil, nil)
	s.assertNoChange(c, prow1)

	// Join the second provider unit to the relation, and check that neither
	// watching unit observes any change.
	err = proru1.EnterScope()
	c.Assert(err, IsNil)
	s.assertNoChange(c, prow1)
	s.assertNoChange(c, prow0)

	// Change the unit's settings, and check that nothing apparently happens.
	pros1 := changeSettings(c, proru1)
	s.assertNoChange(c, prow1)
	s.assertNoChange(c, prow0)

	// ---------- Two roles active ----------

	// Start a watch from the first requirer unit's perspective, and check it
	// only sees the first provider (with which it shares a container).
	reqw0 := reqru0.Watch()
	defer stop(c, reqw0)
	expectJoined := []string{"pro/0"}
	expectChanged := map[string]map[string]interface{}{
		"pro/0": pros0.Map(),
	}
	s.assertChange(c, reqw0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, reqw0)

	// Join the first requirer unit, and check that only the first provider
	// observes the change.
	err = reqru0.EnterScope()
	c.Assert(err, IsNil)
	reqs0, err := reqru0.Settings()
	c.Assert(err, IsNil)
	expectJoined = []string{"req/0"}
	expectChanged = map[string]map[string]interface{}{
		"req/0": reqs0.Map(),
	}
	s.assertChange(c, prow0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, prow0)
	s.assertNoChange(c, prow1)
	s.assertNoChange(c, reqw0)

	// Watch from the second requirer's perspective, and check it only sees the
	// second provider.
	reqw1 := reqru1.Watch()
	defer stop(c, reqw1)
	expectJoined = []string{"pro/1"}
	expectChanged = map[string]map[string]interface{}{
		"pro/1": pros1.Map(),
	}
	s.assertChange(c, reqw1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, reqw1)

	// Join the second requirer, and check that the first provider observes it...
	err = reqru1.EnterScope()
	c.Assert(err, IsNil)
	reqs1, err := reqru1.Settings()
	c.Assert(err, IsNil)
	expectJoined = []string{"req/1"}
	expectChanged = map[string]map[string]interface{}{
		"req/1": reqs1.Map(),
	}
	s.assertChange(c, prow1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, prow1)

	// ...and that nothing else sees anything.
	s.assertNoChange(c, prow0)
	s.assertNoChange(c, reqw0)
	s.assertNoChange(c, reqw1)

	// Change the second provider's settings and check that the second
	// requirer notices...
	pros1 = changeSettings(c, proru1)
	expectChanged = map[string]map[string]interface{}{
		"pro/1": pros1.Map(),
	}
	s.assertChange(c, reqw1, nil, expectChanged, nil)
	s.assertNoChange(c, reqw1)

	// ...but that nothing else does.
	s.assertNoChange(c, prow0)
	s.assertNoChange(c, prow1)
	s.assertNoChange(c, prow0)

	// Finally, depart the first provider, and check that only the first
	// requirer observes any change.
	err = proru0.LeaveScope()
	c.Assert(err, IsNil)
	expectDeparted := []string{"pro/0"}
	s.assertChange(c, reqw0, nil, nil, expectDeparted)
	s.assertNoChange(c, reqw0)
	s.assertNoChange(c, reqw1)
	s.assertNoChange(c, prow0)
	s.assertNoChange(c, prow1)

	// Again, I think we're done, and can be comfortable that the appropriate
	// connections are in place.
}

func changeSettings(c *C, ru *state.RelationUnit) *state.Settings {
	node, err := ru.Settings()
	c.Assert(err, IsNil)
	value, _ := node.Get("value")
	v, _ := value.(int)
	node.Set("value", v+1)
	_, err = node.Write()
	c.Assert(err, IsNil)
	return node
}

func (s *OriginalRelationUnitSuite) assertChange(
	c *C, w *state.RelationUnitsWatcher,
	joined []string,
	changed map[string]map[string]interface{},
	departed []string,
) {
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		if !ok {
			c.Fatalf("channel closed; watcher error: %#v", w.Err())
		}
		sort.Strings(joined)
		sort.Strings(ch.Joined)
		c.Assert(joined, DeepEquals, ch.Joined)
		c.Assert(len(changed), Equals, len(ch.Changed))
		for name, m := range changed {
			c.Assert(ch.Changed[name].Settings, DeepEquals, m)
		}
		sort.Strings(departed)
		sort.Strings(ch.Departed)
		c.Assert(departed, DeepEquals, ch.Departed)
	case <-time.After(5 * time.Second):
		c.Fatalf("expected joined %#v, changed %#v, departed %#v; got nothing", joined, changed, departed)
	}
}

func (s *OriginalRelationUnitSuite) assertNoChange(c *C, w *state.RelationUnitsWatcher) {
	s.State.StartSync()
	select {
	case ch := <-w.Changes():
		c.Fatalf("got %#v, expected nothing", ch)
	case <-time.After(50 * time.Millisecond):
	}
}
