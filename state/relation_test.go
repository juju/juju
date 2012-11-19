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
}

var _ = Suite(&RelationSuite{})

func (s *RelationSuite) TestRelationErrors(c *C) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, IsNil)

	// Check we can't add a relation with services that don't exist.
	msep2 := mysqlEP
	msep2.ServiceName = "yoursql"
	_, err = s.State.AddRelation(msep2, wordpressEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db yoursql:server": .*`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that interfaces have to match.
	msep3 := mysqlEP
	msep3.Interface = "roflcopter"
	_, err = s.State.AddRelation(msep3, wordpressEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db mysql:server": endpoints do not relate`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check a variety of surprising endpoint combinations.
	_, err = s.State.AddRelation(wordpressEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db": single endpoint must be a peer relation`)
	assertNoRelations(c, wordpress)

	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(riakEP, wordpressEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db riak:ring": endpoints do not relate`)
	assertNoRelations(c, riak)
	assertNoRelations(c, wordpress)

	_, err = s.State.AddRelation(riakEP, riakEP)
	c.Assert(err, ErrorMatches, `cannot add relation "riak:ring riak:ring": endpoints do not relate`)
	assertNoRelations(c, riak)

	_, err = s.State.AddRelation()
	c.Assert(err, ErrorMatches, `cannot add relation "": cannot relate 0 endpoints`)
	_, err = s.State.AddRelation(mysqlEP, wordpressEP, riakEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db mysql:server riak:ring": cannot relate 3 endpoints`)
}

func (s *RelationSuite) TestRetrieveSuccess(c *C) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, IsNil)
	expect, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, IsNil)
	rel, err := s.State.EndpointsRelation(wordpressEP, mysqlEP)
	check := func() {
		c.Assert(err, IsNil)
		c.Assert(rel.Id(), Equals, expect.Id())
		c.Assert(rel.String(), Equals, expect.String())
	}
	check()
	rel, err = s.State.EndpointsRelation(mysqlEP, wordpressEP)
	check()
	rel, err = s.State.Relation(expect.Id())
	check()
}

func (s *RelationSuite) TestRetrieveNotFound(c *C) {
	subway := state.Endpoint{"subway", "mongodb", "db", state.RoleRequirer, charm.ScopeGlobal}
	mongo := state.Endpoint{"mongo", "mongodb", "server", state.RoleProvider, charm.ScopeGlobal}
	_, err := s.State.EndpointsRelation(subway, mongo)
	c.Assert(err, ErrorMatches, `relation "subway:db mongo:server" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	_, err = s.State.Relation(999)
	c.Assert(err, ErrorMatches, `relation 999 not found`)
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *RelationSuite) TestProviderRequirerRelation(c *C) {
	// Add a relation, and check we can only do so once.
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db mysql:server": .*`)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)

	// Destroy the relation, and check we can destroy again without error.
	err = rel.Destroy()
	c.Assert(err, IsNil)
	err = rel.Destroy()
	c.Assert(err, IsNil)
	err = rel.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)

	// Check that we can add it again if we want to; but this time,
	// give one of the endpoints container scope and check that both
	// resulting service relations get that scope.
	wordpressEP.RelationScope = charm.ScopeContainer
	_, err = s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, IsNil)
	// After adding relation, make mysqlEP container-scoped as well, for
	// simplicity of testing.
	mysqlEP.RelationScope = charm.ScopeContainer
	assertOneRelation(c, mysql, 2, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 2, wordpressEP, mysqlEP)
}

func (s *RelationSuite) TestRefresh(c *C) {
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)

	rels, err := riak.Relations()
	c.Assert(err, IsNil)
	rel1 := rels[0]
	err = rel.Destroy()
	c.Assert(err, IsNil)

	c.Assert(rel1.Life(), Equals, state.Alive)
	err = rel1.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *RelationSuite) TestDestroy(c *C) {
	// Add a relation, and check we can only do so once.
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)

	err = rel.Destroy()
	c.Assert(err, IsNil)
	_, err = s.State.Relation(rel.Id())
	c.Assert(state.IsNotFound(err), Equals, true)
	_, err = s.State.EndpointsRelation(riakEP)
	c.Assert(state.IsNotFound(err), Equals, true)
	rels, err := riak.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 0)
}

func (s *RelationSuite) TestPeerRelation(c *C) {
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	assertNoRelations(c, riak)

	// Add a relation, and check we can only do so once.
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(riakEP)
	c.Assert(err, ErrorMatches, `cannot add relation "riak:ring": .*`)
	assertOneRelation(c, riak, 0, riakEP)

	// Remove the relation, and check a second removal succeeds.
	err = rel.Destroy()
	c.Assert(err, IsNil)
	assertNoRelations(c, riak)
	err = rel.Destroy()
	c.Assert(err, IsNil)
}

func assertNoRelations(c *C, srv *state.Service) {
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 0)
}

func assertOneRelation(c *C, srv *state.Service, relId int, endpoints ...state.Endpoint) {
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
	c.Assert(eps, DeepEquals, []state.Endpoint{expectEp})
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
		_, err := ru.ReadSettings("riak/0")
		c.Assert(err, ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": settings not found`)
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
		m, err := ru.ReadSettings("riak/0")
		c.Assert(err, IsNil)
		c.Assert(m["meme"], Equals, "socially-awkward-penguin")
	}
}

func (s *RelationUnitSuite) TestProReqSettings(c *C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	rus := RUs{prr.pru0, prr.pru1, prr.rru0, prr.rru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("mysql/0")
		c.Assert(err, ErrorMatches, `cannot read settings for unit "mysql/0" in relation "wordpress:db mysql:server": settings not found`)
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
		m, err := ru.ReadSettings("mysql/0")
		c.Assert(err, IsNil)
		c.Assert(m["meme"], Equals, "foul-bachelor-frog")
	}
}

func (s *RelationUnitSuite) TestContainerSettings(c *C) {
	prr := NewProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	rus := RUs{prr.pru0, prr.pru1, prr.rru0, prr.rru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("mysql/0")
		c.Assert(err, ErrorMatches, `cannot read settings for unit "mysql/0" in relation "logging:info mysql:juju-info": settings not found`)
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
		m, err := ru.ReadSettings("mysql/0")
		c.Assert(err, IsNil)
		c.Assert(m["meme"], Equals, "foul-bachelor-frog")
	}

	// Check settings are still inaccessible to RUs outside that container
	rus1 := RUs{prr.pru1, prr.rru1}
	for _, ru := range rus1 {
		_, err := ru.ReadSettings("mysql/0")
		c.Assert(err, ErrorMatches, `cannot read settings for unit "mysql/0" in relation "logging:info mysql:juju-info": settings not found`)
	}
}

func (s *RelationUnitSuite) TestDestroyRelationWithUnitsInScope(c *C) {
	pr := NewPeerRelation(c, &s.ConnSuite)
	rel := pr.ru0.Relation()

	// Enter two units, and check that Destroy sets the relation to Dying.
	err := pr.ru0.EnterScope()
	c.Assert(err, IsNil)
	err = pr.ru1.EnterScope()
	c.Assert(err, IsNil)
	err = rel.Destroy()
	c.Assert(err, IsNil)
	c.Assert(rel.Life(), Equals, state.Dying)

	// Check a subsequent Destroy is ignored.
	err = rel.Destroy()
	c.Assert(err, IsNil)

	// Check that we can't add a new unit now.
	err = pr.ru2.EnterScope()
	c.Assert(err, Equals, state.ErrRelationNotAlive)

	// Check that we created no settings for the unit we failed to add.
	_, err = pr.ru0.ReadSettings("riak/2")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "riak/2" in relation "riak:ring": settings not found`)

	// ru0 leaves the scope; check that Destroy is still a no-op.
	err = pr.ru0.LeaveScope()
	c.Assert(err, IsNil)
	err = rel.Destroy()
	c.Assert(err, IsNil)

	// Check that unit settings for the original unit still exist, and have
	// not yet been marked for deletion.
	err = s.State.Cleanup()
	c.Assert(err, IsNil)
	assertSettings := func() {
		settings, err := pr.ru1.ReadSettings("riak/0")
		c.Assert(err, IsNil)
		c.Assert(settings, DeepEquals, map[string]interface{}{
			"private-address": "riak-0.example.com",
		})
	}
	assertSettings()

	// The final unit leaves the scope, and cleans up after itself.
	err = pr.ru1.LeaveScope()
	c.Assert(err, IsNil)
	err = rel.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)

	// The settings were not themselves actually deleted yet...
	assertSettings()

	// ...but they were scheduled for deletion.
	err = s.State.Cleanup()
	c.Assert(err, IsNil)
	_, err = pr.ru1.ReadSettings("riak/0")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": settings not found`)

	// Because this is the only sensible place, check that a further call
	// to Cleanup does not error out.
	err = s.State.Cleanup()
	c.Assert(err, Equals, nil)
}

func (s *RelationUnitSuite) TestAliveRelationScope(c *C) {
	pr := NewPeerRelation(c, &s.ConnSuite)
	rel := pr.ru0.Relation()

	// Two units enter...
	err := pr.ru0.EnterScope()
	c.Assert(err, IsNil)
	err = pr.ru1.EnterScope()
	c.Assert(err, IsNil)

	// Two units leave...
	err = pr.ru0.LeaveScope()
	c.Assert(err, IsNil)
	err = pr.ru1.LeaveScope()
	c.Assert(err, IsNil)

	// The relation scope is empty, but the relation is still alive...
	err = rel.Refresh()
	c.Assert(err, IsNil)
	c.Assert(rel.Life(), Equals, state.Alive)

	// ...and new units can still join it.
	err = pr.ru2.EnterScope()
	c.Assert(err, IsNil)
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
	c.Assert(node.Map(), DeepEquals, map[string]interface{}{"private-address": "riak-0.example.com"})

	// ru1 enters; check change is observed.
	err = pr.ru1.EnterScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, w0, []string{"riak/1"}, nil)
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
	s.assertScopeChange(c, w0, []string{"riak/1", "riak/2"}, nil)
	s.assertNoScopeChange(c, w0)

	// ru1 leaves; check event.
	err = pr.ru1.LeaveScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, w0, nil, []string{"riak/1"})
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
		s.assertScopeChange(c, w, []string{"mysql/0"}, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// req0 enters; check detected only by pro RUs.
	err = prr.rru0.EnterScope()
	c.Assert(err, IsNil)
	pws := func() []*state.RelationScopeWatcher {
		return []*state.RelationScopeWatcher{ws[0], ws[1]}
	}
	for _, w := range pws() {
		s.assertScopeChange(c, w, []string{"wordpress/0"}, nil)
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
		s.assertScopeChange(c, w, []string{"wordpress/0", "wordpress/1"}, nil)
	}
	for _, w := range rws() {
		s.assertScopeChange(c, w, []string{"mysql/0", "mysql/1"}, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 leaves; check detected only by req RUs.
	err = prr.pru0.LeaveScope()
	c.Assert(err, IsNil)
	for _, w := range rws() {
		s.assertScopeChange(c, w, nil, []string{"mysql/0"})
	}
	s.assertNoScopeChange(c, ws...)

	// rru0 leaves; check detected only by pro RUs.
	err = prr.rru0.LeaveScope()
	c.Assert(err, IsNil)
	for _, w := range pws() {
		s.assertScopeChange(c, w, nil, []string{"wordpress/0"})
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
	s.assertScopeChange(c, ws[2], []string{"mysql/0"}, nil)
	s.assertNoScopeChange(c, ws...)

	// req1 enters; check detected only by same-container pro.
	err = prr.rru1.EnterScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, ws[1], []string{"logging/1"}, nil)
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
	s.assertScopeChange(c, ws[0], []string{"logging/0"}, nil)
	s.assertScopeChange(c, ws[1], []string{"logging/1"}, nil)
	s.assertScopeChange(c, ws[2], []string{"mysql/0"}, nil)
	s.assertScopeChange(c, ws[3], []string{"mysql/1"}, nil)
	s.assertNoScopeChange(c, ws...)

	// pru0 leaves; check detected only by same-container req.
	err = prr.pru0.LeaveScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, ws[2], nil, []string{"mysql/0"})
	s.assertNoScopeChange(c, ws...)

	// rru0 leaves; check detected only by same-container pro.
	err = prr.rru0.LeaveScope()
	c.Assert(err, IsNil)
	s.assertScopeChange(c, ws[0], nil, []string{"logging/0"})
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
	svc           *state.Service
	u0, u1, u2    *state.Unit
	ru0, ru1, ru2 *state.RelationUnit
}

func NewPeerRelation(c *C, s *ConnSuite) *PeerRelation {
	svc, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	ep, err := svc.Endpoint("ring")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(ep)
	c.Assert(err, IsNil)
	pr := &PeerRelation{svc: svc}
	pr.u0, pr.ru0 = addRU(c, svc, rel, nil)
	pr.u1, pr.ru1 = addRU(c, svc, rel, nil)
	pr.u2, pr.ru2 = addRU(c, svc, rel, nil)
	return pr
}

type ProReqRelation struct {
	psvc, rsvc             *state.Service
	pu0, pu1, ru0, ru1     *state.Unit
	pru0, pru1, rru0, rru1 *state.RelationUnit
}

func NewProReqRelation(c *C, s *ConnSuite, scope charm.RelationScope) *ProReqRelation {
	psvc, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	var rsvc *state.Service
	if scope == charm.ScopeGlobal {
		rsvc, err = s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	} else {
		rsvc, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	}
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"mysql", rsvc.Name()})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	prr := &ProReqRelation{psvc: psvc, rsvc: rsvc}
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
}

var _ = Suite(&OriginalRelationUnitSuite{})

func (s *OriginalRelationUnitSuite) TestRelationUnitEnterScopeError(c *C) {
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)
	u0, err := riak.AddUnit()
	c.Assert(err, IsNil)
	ru0, err := rel.Unit(u0)
	c.Assert(err, IsNil)
	err = u0.EnsureDead()
	c.Assert(err, IsNil)
	err = riak.RemoveUnit(u0)
	c.Assert(err, IsNil)
	err = ru0.EnterScope()
	c.Assert(err, ErrorMatches, `cannot initialize state for unit "riak/0" in relation "riak:ring": private address of unit "riak/0" not found`)

	u1, err := riak.AddUnit()
	c.Assert(err, IsNil)
	err = u1.SetPrivateAddress("u1.example.com")
	c.Assert(err, IsNil)
	ru1, err := rel.Unit(u1)
	c.Assert(err, IsNil)
	err = rel.Destroy()
	c.Assert(err, IsNil)
	err = ru1.EnterScope()
	c.Assert(err, Equals, state.ErrRelationNotAlive)
}

func (s *OriginalRelationUnitSuite) TestRelationUnitReadSettings(c *C) {
	// Create a peer service with a relation and two units.
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)
	u0, err := riak.AddUnit()
	c.Assert(err, IsNil)
	ru0, err := rel.Unit(u0)
	c.Assert(err, IsNil)
	u1, err := riak.AddUnit()
	c.Assert(err, IsNil)
	ru1, err := rel.Unit(u1)
	c.Assert(err, IsNil)

	// Check various errors.
	_, err = ru0.ReadSettings("nonsense")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "nonsense" in relation "riak:ring": "nonsense" is not a valid unit name`)
	_, err = ru0.ReadSettings("unknown/0")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "unknown/0" in relation "riak:ring": service "unknown" is not a member of "riak:ring"`)
	_, err = ru0.ReadSettings("riak/pressure")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "riak/pressure" in relation "riak:ring": "riak/pressure" is not a valid unit name`)
	_, err = ru0.ReadSettings("riak/1")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "riak/1" in relation "riak:ring": settings not found`)

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
		settings, err := ru.ReadSettings("riak/1")
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
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)

	// Add some units to the service and set their private addresses; get
	// the relevant RelationUnits.
	// (Private addresses should be set by their unit agents on
	// startup; this test does not include that, but Join expects
	// the information to be available, and uses it to populate the
	// relation settings node.)
	addUnit := func(i int) *state.RelationUnit {
		unit, err := riak.AddUnit()
		c.Assert(err, IsNil)
		err = unit.SetPrivateAddress(fmt.Sprintf("riak%d.example.com", i))
		c.Assert(err, IsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, IsNil)
		c.Assert(ru.Endpoint(), Equals, riakEP)
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
		"riak/1": s1.Map(),
	}
	s.assertChange(c, w0, []string{"riak/1"}, expectChanged, nil)
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
		"riak/0": s0.Map(),
	}
	s.assertChange(c, w1, []string{"riak/0"}, expectChanged, nil)
	s.assertNoChange(c, w1)

	// ---------- Three units ----------

	// Whoa, it works. Ok, check the third unit's opinion of the state.
	w2 := ru2.Watch()
	defer stop(c, w2)
	expectChanged = map[string]map[string]interface{}{
		"riak/0": s0.Map(),
		"riak/1": s1.Map(),
	}
	s.assertChange(c, w2, []string{"riak/0", "riak/1"}, expectChanged, nil)
	s.assertNoChange(c, w2)

	// Join the third unit, and check the first and second units see it.
	err = ru2.EnterScope()
	c.Assert(err, IsNil)
	s2, err := ru2.Settings()
	c.Assert(err, IsNil)
	expectChanged = map[string]map[string]interface{}{
		"riak/2": s2.Map(),
	}
	s.assertChange(c, w0, []string{"riak/2"}, expectChanged, nil)
	s.assertNoChange(c, w0)
	s.assertChange(c, w1, []string{"riak/2"}, expectChanged, nil)
	s.assertNoChange(c, w1)

	// Change the second unit's settings, and check that only
	// the first and third see changes.
	s1 = changeSettings(c, ru1)
	s.assertNoChange(c, w1)
	expectChanged = map[string]map[string]interface{}{
		"riak/1": s1.Map(),
	}
	s.assertChange(c, w0, nil, expectChanged, nil)
	s.assertNoChange(c, w0)
	s.assertChange(c, w2, nil, expectChanged, nil)
	s.assertNoChange(c, w2)

	// ---------- Two units again ----------

	// Depart the second unit, and check that the first and third detect it.
	err = ru1.LeaveScope()
	c.Assert(err, IsNil)
	expectDeparted := []string{"riak/1"}
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
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, IsNil)
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, IsNil)

	// Add some units to the services and set their private addresses.
	addUnit := func(srv *state.Service, sub string, ep state.Endpoint) *state.RelationUnit {
		unit, err := srv.AddUnit()
		c.Assert(err, IsNil)
		err = unit.SetPrivateAddress(fmt.Sprintf("%s.example.com", sub))
		c.Assert(err, IsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, IsNil)
		c.Assert(ru.Endpoint(), Equals, ep)
		return ru
	}
	msru0 := addUnit(mysql, "ms0", mysqlEP)
	msru1 := addUnit(mysql, "ms1", mysqlEP)
	wpru0 := addUnit(wordpress, "wp0", wordpressEP)
	wpru1 := addUnit(wordpress, "wp1", wordpressEP)

	// ---------- Single role active ----------

	// Watch the relation from the perspective of the first provider unit and
	// check initial event.
	msw0 := msru0.Watch()
	defer stop(c, msw0)
	s.assertChange(c, msw0, nil, nil, nil)
	s.assertNoChange(c, msw0)

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	err = msru0.EnterScope()
	c.Assert(err, IsNil)
	mss0 := changeSettings(c, msru0)
	s.assertNoChange(c, msw0)

	// Join the second provider unit, start its watch, and check what it thinks the
	// state of the relation is.
	err = msru1.EnterScope()
	c.Assert(err, IsNil)
	msw1 := msru1.Watch()
	defer stop(c, msw1)
	s.assertChange(c, msw1, nil, nil, nil)
	s.assertNoChange(c, msw1)

	// Change the unit's settings, and check that neither provider unit
	// observes any change.
	mss1 := changeSettings(c, msru1)
	s.assertNoChange(c, msw1)
	s.assertNoChange(c, msw0)

	// ---------- Two roles active ----------

	// Start watches from both requirer units' perspectives, and check that
	// they see the provider units.
	expectJoined := []string{"mysql/0", "mysql/1"}
	expectChanged := map[string]map[string]interface{}{
		"mysql/0": mss0.Map(),
		"mysql/1": mss1.Map(),
	}
	wpw0 := wpru0.Watch()
	defer stop(c, wpw0)
	s.assertChange(c, wpw0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, wpw0)
	wpw1 := wpru1.Watch()
	defer stop(c, wpw1)
	s.assertChange(c, wpw1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, wpw1)

	// Join the first requirer unit, and check the provider units see it.
	err = wpru0.EnterScope()
	c.Assert(err, IsNil)
	wps0, err := wpru0.Settings()
	c.Assert(err, IsNil)
	expectJoined = []string{"wordpress/0"}
	expectChanged = map[string]map[string]interface{}{
		"wordpress/0": wps0.Map(),
	}
	s.assertChange(c, msw0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, msw0)
	s.assertChange(c, msw1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, msw1)

	// Join again, check no-op.
	err = wpru0.EnterScope()
	c.Assert(err, IsNil)
	s.assertNoChange(c, msw0)
	s.assertNoChange(c, msw1)

	// Join the second requirer, and check the provider units see the change.
	err = wpru1.EnterScope()
	c.Assert(err, IsNil)
	wps1, err := wpru1.Settings()
	c.Assert(err, IsNil)
	expectJoined = []string{"wordpress/1"}
	expectChanged = map[string]map[string]interface{}{
		"wordpress/1": wps1.Map(),
	}
	s.assertChange(c, msw0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, msw0)
	s.assertChange(c, msw1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, msw1)

	// Verify that neither requirer has observed any change to the relation.
	s.assertNoChange(c, wpw0)
	s.assertNoChange(c, wpw1)

	// Change settings for the first requirer, check providers see it...
	wps0 = changeSettings(c, wpru0)
	expectChanged = map[string]map[string]interface{}{
		"wordpress/0": wps0.Map(),
	}
	s.assertChange(c, msw0, nil, expectChanged, nil)
	s.assertNoChange(c, msw0)
	s.assertChange(c, msw1, nil, expectChanged, nil)
	s.assertNoChange(c, msw1)

	// ...and requirers don't.
	s.assertNoChange(c, wpw0)
	s.assertNoChange(c, wpw1)

	// Depart the second requirer and check the providers see it...
	err = wpru1.LeaveScope()
	c.Assert(err, IsNil)
	expectDeparted := []string{"wordpress/1"}
	s.assertChange(c, msw0, nil, nil, expectDeparted)
	s.assertNoChange(c, msw0)
	s.assertChange(c, msw1, nil, nil, expectDeparted)
	s.assertNoChange(c, msw1)

	// ...and the requirers don't.
	s.assertNoChange(c, wpw0)
	s.assertNoChange(c, wpw1)

	// Cleanup handled by defers as before.
}

func (s *OriginalRelationUnitSuite) TestContainerProReqRelationUnit(c *C) {
	// Create a pair of services and a relation between them.
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("juju-info")
	c.Assert(err, IsNil)
	logging, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(mysqlEP, loggingEP)
	c.Assert(err, IsNil)

	// Change mysqlEP to match the endpoint that will actually be used by the relation.
	mysqlEP.RelationScope = charm.ScopeContainer

	// Add some units to the services and set their private addresses.
	addUnits := func(i int) (*state.RelationUnit, *state.RelationUnit) {
		msu, err := mysql.AddUnit()
		c.Assert(err, IsNil)
		err = msu.SetPrivateAddress(fmt.Sprintf("mysql%d.example.com", i))
		c.Assert(err, IsNil)
		msru, err := rel.Unit(msu)
		c.Assert(err, IsNil)
		c.Assert(msru.Endpoint(), Equals, mysqlEP)
		lgu, err := logging.AddUnitSubordinateTo(msu)
		c.Assert(err, IsNil)
		err = lgu.SetPrivateAddress(fmt.Sprintf("logging%d.example.com", i))
		c.Assert(err, IsNil)
		lgru, err := rel.Unit(lgu)
		c.Assert(err, IsNil)
		c.Assert(lgru.Endpoint(), Equals, loggingEP)
		return msru, lgru
	}
	msru0, lgru0 := addUnits(0)
	msru1, lgru1 := addUnits(1)

	// ---------- Single role active ----------

	// Start watching the relation from the perspective of the first unit, and
	// check the initial event.
	msw0 := msru0.Watch()
	defer stop(c, msw0)
	s.assertChange(c, msw0, nil, nil, nil)
	s.assertNoChange(c, msw0)

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	err = msru0.EnterScope()
	c.Assert(err, IsNil)
	mss0 := changeSettings(c, msru0)
	s.assertNoChange(c, msw0)

	// Watch the relation from the perspective of the second provider, and
	// check initial event.
	msw1 := msru1.Watch()
	defer stop(c, msw1)
	s.assertChange(c, msw1, nil, nil, nil)
	s.assertNoChange(c, msw1)

	// Join the second provider unit to the relation, and check that neither
	// watching unit observes any change.
	err = msru1.EnterScope()
	c.Assert(err, IsNil)
	s.assertNoChange(c, msw1)
	s.assertNoChange(c, msw0)

	// Change the unit's settings, and check that nothing apparently happens.
	mss1 := changeSettings(c, msru1)
	s.assertNoChange(c, msw1)
	s.assertNoChange(c, msw0)

	// ---------- Two roles active ----------

	// Start a watch from the first requirer unit's perspective, and check it
	// only sees the first provider (with which it shares a container).
	lgw0 := lgru0.Watch()
	defer stop(c, lgw0)
	expectJoined := []string{"mysql/0"}
	expectChanged := map[string]map[string]interface{}{
		"mysql/0": mss0.Map(),
	}
	s.assertChange(c, lgw0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, lgw0)

	// Join the first requirer unit, and check that only the first provider
	// observes the change.
	err = lgru0.EnterScope()
	c.Assert(err, IsNil)
	lgs0, err := lgru0.Settings()
	c.Assert(err, IsNil)
	expectJoined = []string{"logging/0"}
	expectChanged = map[string]map[string]interface{}{
		"logging/0": lgs0.Map(),
	}
	s.assertChange(c, msw0, expectJoined, expectChanged, nil)
	s.assertNoChange(c, msw0)
	s.assertNoChange(c, msw1)
	s.assertNoChange(c, lgw0)

	// Watch from the second requirer's perspective, and check it only sees the
	// second provider.
	lgw1 := lgru1.Watch()
	defer stop(c, lgw1)
	expectJoined = []string{"mysql/1"}
	expectChanged = map[string]map[string]interface{}{
		"mysql/1": mss1.Map(),
	}
	s.assertChange(c, lgw1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, lgw1)

	// Join the second requirer, and check that the first provider observes it...
	err = lgru1.EnterScope()
	c.Assert(err, IsNil)
	lgs1, err := lgru1.Settings()
	c.Assert(err, IsNil)
	expectJoined = []string{"logging/1"}
	expectChanged = map[string]map[string]interface{}{
		"logging/1": lgs1.Map(),
	}
	s.assertChange(c, msw1, expectJoined, expectChanged, nil)
	s.assertNoChange(c, msw1)

	// ...and that nothing else sees anything.
	s.assertNoChange(c, msw0)
	s.assertNoChange(c, lgw0)
	s.assertNoChange(c, lgw1)

	// Change the second provider's settings and check that the second
	// requirer notices...
	mss1 = changeSettings(c, msru1)
	expectChanged = map[string]map[string]interface{}{
		"mysql/1": mss1.Map(),
	}
	s.assertChange(c, lgw1, nil, expectChanged, nil)
	s.assertNoChange(c, lgw1)

	// ...but that nothing else does.
	s.assertNoChange(c, msw0)
	s.assertNoChange(c, msw1)
	s.assertNoChange(c, msw0)

	// Finally, depart the first provider, and check that only the first
	// requirer observes any change.
	err = msru0.LeaveScope()
	c.Assert(err, IsNil)
	expectDeparted := []string{"mysql/0"}
	s.assertChange(c, lgw0, nil, nil, expectDeparted)
	s.assertNoChange(c, lgw0)
	s.assertNoChange(c, lgw1)
	s.assertNoChange(c, msw0)
	s.assertNoChange(c, msw1)

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
