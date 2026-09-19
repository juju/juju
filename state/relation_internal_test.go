// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
)

type RelationSuite struct{}

var _ = gc.Suite(&RelationSuite{})

// TestRelatedEndpoints verifies the behaviour of RelatedEndpoints in
// multi-endpoint peer relations, which are currently not constructable
// by normal means.
func (s *RelationSuite) TestRelatedEndpoints(c *gc.C) {
	rel := charm.Relation{
		Interface: "ifce",
		Name:      "group",
		Role:      charm.RolePeer,
		Scope:     charm.ScopeGlobal,
	}
	eps := []Endpoint{{
		ApplicationName: "jeff",
		Relation:        rel,
	}, {
		ApplicationName: "mike",
		Relation:        rel,
	}, {
		ApplicationName: "mike",
		Relation:        rel,
	}}
	r := &Relation{nil, relationDoc{Endpoints: eps}}
	relatedEps, err := r.RelatedEndpoints("mike")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relatedEps, gc.DeepEquals, eps)
}

type relationCleanupSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&relationCleanupSuite{})

// setupRelation creates a relation between a local mysql application and
// a remote wordpress application.
func (s *relationCleanupSuite) setupRelation(c *gc.C) *Relation {
	return s.setupRelationInState(c, s.state)
}

// setupRelationInState creates a relation between a local mysql
// application and a remote wordpress application in the given model.
func (s *relationCleanupSuite) setupRelationInState(c *gc.C, st *State) *Relation {
	rapp, err := st.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-wordpress",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	ch := AddTestingCharm(c, st, "mysql")
	mysql := AddTestingApplication(c, st, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	return rel
}

func (s *relationCleanupSuite) TestRemoveOpsQueuesScopeCleanupBeforeSettings(c *gc.C) {
	rel := s.setupRelation(c)

	ops, err := rel.removeOps("", "", &ForcedOperation{})
	c.Assert(err, jc.ErrorIsNil)

	var kinds []cleanupKind
	for _, op := range ops {
		if doc, ok := op.Insert.(*cleanupDoc); ok {
			kinds = append(kinds, doc.Kind)
		}
	}
	c.Assert(kinds, gc.DeepEquals, []cleanupKind{
		cleanupRelationScopes, cleanupRelationSettings, cleanupRelationState,
	})
}

func (s *relationCleanupSuite) TestRelationStateCleanup(c *gc.C) {
	rel := s.setupRelation(c)
	relID := rel.Id()

	// Add a unit to the mysql application setupRelation created, then
	// write unit state referencing the relation plus a bogus id.
	app, err := s.state.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	unit, err := app.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	us := NewUnitState()
	us.SetRelationState(map[int]string{
		relID:    "blob",
		99999999: "stale",
	})
	err = unit.SetState(us, UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)
	// Overwrite the map directly to inject the stale key despite the
	// SetUnitState filter.
	coll, closer, err := s.state.db().GetRawCollection("unitstates")
	c.Assert(err, jc.ErrorIsNil)
	err = coll.UpdateId(s.state.docID(unit.globalKey()), bson.M{"$set": bson.M{
		"relation-state": bson.M{
			strconv.Itoa(relID): "blob",
			"99999999":          "stale",
		},
	}})
	closer()
	c.Assert(err, jc.ErrorIsNil)

	// The cleanup change removes the stale key for the removed relation.
	err = s.state.cleanupRelationState(strconv.Itoa(relID))
	c.Assert(err, jc.ErrorIsNil)

	uState, err := unit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{
		99999999: "stale",
	})
}

func (s *relationCleanupSuite) TestForceTeardownCleanupOps(c *gc.C) {
	rel := s.setupRelation(c)

	ops := forceTeardownCleanupOps(rel)
	c.Assert(ops, gc.HasLen, 2)
	c.Assert(ops[0].C, gc.Equals, relationsC)
	assert_, ok := ops[0].Assert.(bson.D)
	c.Assert(ok, jc.IsTrue)
	c.Assert(assert_, gc.DeepEquals, isAliveDoc)
	update, ok := ops[0].Update.(bson.D)
	c.Assert(ok, jc.IsTrue)
	c.Assert(update, gc.DeepEquals, bson.D{{"$set", bson.D{{"life", Dying}}}})
	c.Assert(ops[1].C, gc.Equals, cleanupsC)
	doc, ok := ops[1].Insert.(*cleanupDoc)
	c.Assert(ok, jc.IsTrue)
	c.Assert(doc.Kind, gc.Equals, cleanupForceDestroyedRelation)
	c.Assert(doc.Prefix, gc.Equals, strconv.Itoa(rel.Id()))

	// A relation already Dying gets no write op (setting Dying again
	// would bump txn-revno for a no-op); only the cleanup is queued.
	dyingRel := &Relation{st: rel.st, doc: rel.doc}
	dyingRel.doc.Life = Dying
	ops = forceTeardownCleanupOps(dyingRel)
	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].C, gc.Equals, cleanupsC)
	doc, ok = ops[0].Insert.(*cleanupDoc)
	c.Assert(ok, jc.IsTrue)
	c.Assert(doc.Kind, gc.Equals, cleanupForceDestroyedRelation)
	c.Assert(doc.Prefix, gc.Equals, strconv.Itoa(rel.Id()))
}

func (s *relationCleanupSuite) TestForceRelationRemoveOps(c *gc.C) {
	rel := s.setupRelation(c)
	boom := errors.New("boom")

	// No error: ops returned untouched, isRemove true.
	ops, isRemove, err := forceRelationRemoveOps(rel, []txn.Op{{C: "x"}}, nil, &ForcedOperation{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isRemove, jc.IsTrue)
	c.Assert(ops, gc.HasLen, 1)

	// Non-force error: fatal.
	ops, isRemove, err = forceRelationRemoveOps(rel, nil, boom, &ForcedOperation{})
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(isRemove, jc.IsFalse)
	c.Assert(ops, gc.HasLen, 0)

	// Force + error: the (partial, inconsistent) removal ops are
	// discarded and an async force teardown is queued instead, whether
	// or not partial ops were built.
	for _, removeOps := range [][]txn.Op{nil, {{C: "x"}}} {
		ops, isRemove, err = forceRelationRemoveOps(rel, removeOps, boom, &ForcedOperation{Force: true})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(isRemove, jc.IsTrue)
		c.Assert(ops, gc.HasLen, 2)
		c.Assert(ops[0].C, gc.Equals, relationsC)
		doc, ok := ops[1].Insert.(*cleanupDoc)
		c.Assert(ok, jc.IsTrue)
		c.Assert(doc.Kind, gc.Equals, cleanupForceDestroyedRelation)
		c.Assert(doc.Prefix, gc.Equals, strconv.Itoa(rel.Id()))
	}
}

func (s *relationCleanupSuite) TestRelationStateCleanupIsModelScoped(c *gc.C) {
	otherState := s.newState(c)

	relA := s.setupRelation(c)
	relB := s.setupRelationInState(c, otherState)
	c.Assert(relB.Id(), gc.Equals, relA.Id(),
		gc.Commentf("test requires both models to have a relation with the same numeric id"))

	addUnitWithRelationState := func(st *State, rel *Relation, blob string) *Unit {
		app, err := st.Application("mysql")
		c.Assert(err, jc.ErrorIsNil)
		unit, err := app.AddUnit(AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		us := NewUnitState()
		us.SetRelationState(map[int]string{rel.Id(): blob})
		c.Assert(unit.SetState(us, UnitStateSizeLimits{}), jc.ErrorIsNil)
		return unit
	}
	unitA := addUnitWithRelationState(s.state, relA, "model-a-blob")
	unitB := addUnitWithRelationState(otherState, relB, "model-b-blob")

	// Run the cleanup in this model only.
	err := s.state.cleanupRelationState(strconv.Itoa(relA.Id()))
	c.Assert(err, jc.ErrorIsNil)

	uState, err := unitA.State()
	c.Assert(err, jc.ErrorIsNil)
	// The relation-state entry for this model's relation is gone, and
	// because it was the only entry the whole relation-state field is
	// removed: the uniter observes the map as not set at all.
	rstA, found := uState.RelationState()
	c.Assert(found, jc.IsFalse)
	c.Assert(rstA, gc.HasLen, 0)

	// The stored document has no relation-state field at all.
	coll, closer, err := s.state.db().GetRawCollection("unitstates")
	c.Assert(err, jc.ErrorIsNil)
	var raw bson.M
	err = coll.FindId(s.state.docID(unitA.globalKey())).One(&raw)
	closer()
	c.Assert(err, jc.ErrorIsNil)
	_, ok := raw["relation-state"]
	c.Assert(ok, jc.IsFalse)

	// The other model's same-id relation state is untouched.
	uState, err = unitB.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{relB.Id(): "model-b-blob"})
}

func (s *relationCleanupSuite) TestRelationStateCleanupAbortsOnConcurrentWrite(c *gc.C) {
	rel := s.setupRelation(c)

	// A second live relation for the same local application, so a
	// concurrent unit state write can carry a relation-state key for
	// a still-live relation.
	rapp2, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-wordpress-extra",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid-2",
		Endpoints: []charm.Relation{{
			Interface: "mysql-root",
			Limit:     1,
			Name:      "dbadmin",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP2, err := rapp2.Endpoint("dbadmin")
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.state.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	mysqlAdminEP, err := app.Endpoint("server-admin")
	c.Assert(err, jc.ErrorIsNil)
	rel2, err := s.state.AddRelation(remoteEP2, mysqlAdminEP)
	c.Assert(err, jc.ErrorIsNil)

	// The unit's stored relation state references only the relation
	// the cleanup is about to sweep.
	unit, err := app.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	us := NewUnitState()
	us.SetRelationState(map[int]string{rel.Id(): "blob"})
	c.Assert(unit.SetState(us, UnitStateSizeLimits{}), jc.ErrorIsNil)

	// Snapshot the cleanup operations for that relation. The stored
	// map has a single key, so the whole relation-state field would be
	// unset.
	change := relationStateCleanupChange{Prefix: strconv.Itoa(rel.Id())}
	ops, err := change.Prepare(s.state.db())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 1)

	// A concurrent unit state write lands between the snapshot and the
	// commit, carrying state for the still-live second relation.
	concurrent := NewUnitState()
	concurrent.SetRelationState(map[int]string{rel2.Id(): "live"})
	c.Assert(unit.SetState(concurrent, UnitStateSizeLimits{}), jc.ErrorIsNil)

	// The cleanup transaction must abort on the revision assert rather
	// than wiping the newer write.
	err = s.state.db().RunTransaction(ops)
	c.Assert(err, gc.Equals, txn.ErrAborted)

	// The concurrent write survives, and a fresh cleanup pass finds
	// nothing left to remove for the removed relation.
	c.Assert(s.state.cleanupRelationState(strconv.Itoa(rel.Id())), jc.ErrorIsNil)
	uState, err := unit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{rel2.Id(): "live"})
}

func (s *relationCleanupSuite) TestRelationConvergesViaCleanup(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:            "remote-wordpress",
		SourceModel:     names.NewModelTag("source-model"),
		OfferUUID:       "offer-uuid",
		IsConsumerProxy: true,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlUnit, err := mysql.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru, err := rel.Unit(mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysqlru.EnterScope(nil), jc.ErrorIsNil)
	rru, err := rel.RemoteUnit("remote-wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rru.EnterScope(nil), jc.ErrorIsNil)

	c.Assert(s.state.db().RunTransaction(forceTeardownCleanupOps(rel)), jc.ErrorIsNil)

	// The cleanups worker completes the teardown. Removing the relation
	// queues further scopes/settings/state cleanups, so keep trying
	// until no cleanups are left, exactly as the periodic cleanups
	// worker does.
	noopSecretDeleter := func(*secrets.URI, int) error { return nil }
	for {
		needed, err := s.state.NeedsCleanup()
		c.Assert(err, jc.ErrorIsNil)
		if !needed {
			break
		}
		c.Assert(s.state.Cleanup(noopSecretDeleter), jc.ErrorIsNil)
	}

	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.state.RemoteApplication("remote-wordpress")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	inScope, err := mysqlru.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inScope, jc.IsFalse)
	inScope, err = rru.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inScope, jc.IsFalse)

	// No relation scopes or settings are left behind.
	for _, collName := range []string{relationScopesC, settingsC} {
		coll, closer, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		n, err := coll.Find(bson.M{"_id": bson.M{
			"$regex": fmt.Sprintf("^%s:r#%d#", s.state.ModelUUID(), rel.Id()),
		}}).Count()
		closer()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 0, gc.Commentf("leftover %s docs", collName))
	}
}
