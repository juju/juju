// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"

	"github.com/juju/charm/v12"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type unitStateOpsSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&unitStateOpsSuite{})

// setupUnitWithRelation creates a mysql application with one unit and a
// relation to a remote wordpress application, and returns the relation
// and the unit.
func (s *unitStateOpsSuite) setupUnitWithRelation(c *gc.C) (*Relation, *Unit) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
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

	ch := AddTestingCharm(c, s.state, "mysql")
	app := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := app.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := app.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	return rel, unit
}

// overwriteRelationState replaces the unit's stored relation-state map
// directly, bypassing the write filter as a uniter running pre-filter
// code would have.
func (s *unitStateOpsSuite) overwriteRelationState(c *gc.C, unit *Unit, keys map[string]string) {
	coll, closer, err := s.state.db().GetRawCollection(unitStatesC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	err = coll.UpdateId(s.state.docID(unit.globalKey()),
		bson.M{"$set": bson.M{"relation-state": keys}})
	c.Assert(err, jc.ErrorIsNil)
}

// rawUnitState returns the raw unitstates document for the unit.
func (s *unitStateOpsSuite) rawUnitState(c *gc.C, unit *Unit) bson.M {
	coll, closer, err := s.state.db().GetRawCollection(unitStatesC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	var raw bson.M
	err = coll.FindId(s.state.docID(unit.globalKey())).One(&raw)
	c.Assert(err, jc.ErrorIsNil)
	return raw
}

func (s *unitStateOpsSuite) TestUnitStateWriteBackFiltersStaleRelations(c *gc.C) {
	rel, unit := s.setupUnitWithRelation(c)
	relID := strconv.Itoa(rel.Id())

	// Seed the stored document as a pre-filter uniter would have left
	// it: a key for the live relation plus a stale id.
	first := NewUnitState()
	first.SetRelationState(map[int]string{rel.Id(): "blob"})
	c.Assert(unit.SetState(first, UnitStateSizeLimits{}), jc.ErrorIsNil)
	s.overwriteRelationState(c, unit, map[string]string{
		relID:      "blob",
		"99999999": "stale",
	})

	// The uniter commits an update carrying both keys again; the write
	// filter drops the stale one on the update path, not just on insert.
	writeBack := NewUnitState()
	writeBack.SetRelationState(map[int]string{
		rel.Id(): "new blob",
		99999999: "stale blob",
	})
	c.Assert(unit.SetState(writeBack, UnitStateSizeLimits{}), jc.ErrorIsNil)
	uState, err := unit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{rel.Id(): "new blob"})

	// A write whose relation-state keys are all filtered out removes
	// the whole relation-state field rather than storing an empty map.
	allStale := NewUnitState()
	allStale.SetRelationState(map[int]string{99999999: "stale blob"})
	c.Assert(unit.SetState(allStale, UnitStateSizeLimits{}), jc.ErrorIsNil)
	raw := s.rawUnitState(c, unit)
	_, ok := raw["relation-state"]
	c.Assert(ok, jc.IsFalse)
}

func (s *unitStateOpsSuite) TestUnitStateWriteAssertsRelationExists(c *gc.C) {
	rel, unit := s.setupUnitWithRelation(c)

	// Seed the stored document so the next write takes the update path.
	first := NewUnitState()
	first.SetRelationState(map[int]string{rel.Id(): "blob"})
	c.Assert(unit.SetState(first, UnitStateSizeLimits{}), jc.ErrorIsNil)

	// Build the write transaction, then remove the relation after the
	// ops have been built but before they are applied.
	second := NewUnitState()
	second.SetRelationState(map[int]string{rel.Id(): "new blob"})
	op := unit.SetStateOperation(second, UnitStateSizeLimits{})
	ops, err := op.Build(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Destroy(), jc.ErrorIsNil)

	// The existence assertion on the relation aborts the transaction;
	// the write must not land.
	err = s.state.db().RunTransaction(ops)
	c.Assert(err, gc.Equals, txn.ErrAborted)
	uState, err := unit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{rel.Id(): "blob"})

	// A fresh write after the removal converges: the filter drops the
	// removed relation's key, so no state is written for it.
	c.Assert(unit.SetState(second, UnitStateSizeLimits{}), jc.ErrorIsNil)
	uState, err = unit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found = uState.RelationState()
	c.Assert(found, jc.IsFalse)
	c.Assert(rst, gc.HasLen, 0)
}
