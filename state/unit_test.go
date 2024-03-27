// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time" // Only used for time types.

	"github.com/juju/charm/v13"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/testing"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

const (
	contentionErr = ".*state changing too quickly; try again soon"
)

type UnitSuite struct {
	ConnSuite
	charm       *state.Charm
	application *state.Application
	unit        *state.Unit
}

var _ = gc.Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "wordpress")
	s.application = s.AddTestingApplication(c, "wordpress", s.charm)
	var err error
	s.unit, err = s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Base(), jc.DeepEquals, state.Base{OS: "ubuntu", Channel: "12.10/stable"})
	c.Assert(s.unit.ShouldBeAssigned(), jc.IsTrue)
}

func (s *UnitSuite) TestUnitNotFound(c *gc.C) {
	_, err := s.State.Unit("subway/0")
	c.Assert(err, gc.ErrorMatches, `unit "subway/0" not found`)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *UnitSuite) TestApplication(c *gc.C) {
	app, err := s.unit.Application()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Name(), gc.Equals, s.unit.ApplicationName())
}

func (s *UnitSuite) TestCharmStateQuotaLimitWithMissingStateDocument(c *gc.C) {
	// Set initial state with a restrictive limit. Since the state document
	// does not exist yet, this test checks that the insert new document
	// codepath correctly enforces the quota limits.
	newState := new(state.UnitState)
	newState.SetCharmState(map[string]string{
		"answer": "42",
		"data":   "encrypted",
	})

	err := s.unit.SetState(newState, state.UnitStateSizeLimits{
		MaxCharmStateSize: 16,
	})
	c.Assert(err, jc.ErrorIs, errors.QuotaLimitExceeded)

	// Try again with a more generous quota limit
	err = s.unit.SetState(newState, state.UnitStateSizeLimits{
		MaxCharmStateSize: 640,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestCharmStateQuotaLimit(c *gc.C) {
	// Set initial state with a generous limit
	newState := new(state.UnitState)
	newState.SetCharmState(map[string]string{
		"answer": "42",
		"data":   "encrypted",
	})

	err := s.unit.SetState(newState, state.UnitStateSizeLimits{
		MaxCharmStateSize: 640000,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Try to set a new state with a tight limit
	newState.SetCharmState(map[string]string{
		"answer": "unknown",
		"data":   "decrypted",
	})
	err = s.unit.SetState(newState, state.UnitStateSizeLimits{
		MaxCharmStateSize: 10,
	})
	c.Assert(err, jc.ErrorIs, errors.QuotaLimitExceeded)
}

func (s *UnitSuite) TestCombinedUnitStateQuotaLimit(c *gc.C) {
	// Set initial state with a generous limit
	newState := new(state.UnitState)
	newState.SetUniterState("my state is legendary")
	newState.SetRelationState(map[int]string{42: "some serialized blob"})
	newState.SetStorageState("storage is cheap")

	err := s.unit.SetState(newState, state.UnitStateSizeLimits{
		MaxAgentStateSize: 640000,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Try to set a new uniter state where the combined data will trip the quota limit check
	newState.SetUniterState("state")
	newState.SetRelationState(map[int]string{42: "a fresh serialized blob"})
	newState.SetStorageState("storage")
	err = s.unit.SetState(newState, state.UnitStateSizeLimits{
		MaxAgentStateSize: 42,
	})
	c.Assert(err, jc.ErrorIs, errors.QuotaLimitExceeded)
}

func (s *UnitSuite) TestUnitStateWithDualQuotaLimits(c *gc.C) {
	// Set state with dual quota limits and check that both limits are
	// correctly enforced and do not interfere with each other.
	newState := new(state.UnitState)
	newState.SetUniterState("my state is legendary")
	newState.SetRelationState(map[int]string{42: "some serialized blob"})
	newState.SetStorageState("storage is cheap")
	newState.SetCharmState(map[string]string{
		"charm-data": strings.Repeat("lol", 1024),
	})

	err := s.unit.SetState(newState, state.UnitStateSizeLimits{
		MaxCharmStateSize: 4096,
		MaxAgentStateSize: 128,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestUnitStateNotSet(c *gc.C) {
	// Try fetching the state without a state doc present
	uState, err := s.unit.State()
	c.Assert(err, gc.IsNil)
	st, found := uState.CharmState()
	c.Assert(st, gc.IsNil, gc.Commentf("expected to receive a nil map when no state doc is present"))
	c.Assert(found, jc.IsFalse)
	ust, found := uState.UniterState()
	c.Assert(ust, gc.Equals, "")
	c.Assert(found, jc.IsFalse)
	rst, found := uState.RelationState()
	c.Assert(rst, gc.IsNil, gc.Commentf("expected to receive a nil map when no state doc is present"))
	c.Assert(found, jc.IsFalse)
	sst, found := uState.StorageState()
	c.Assert(sst, gc.Equals, "")
	c.Assert(found, jc.IsFalse)
}

func (s *UnitSuite) TestUnitStateExistingDocAddNewRelationData(c *gc.C) {
	initialUniterState := "testing"
	us := state.NewUnitState()
	us.SetUniterState(initialUniterState)
	err := s.unit.SetState(us, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	newRelationState := map[int]string{3: "three"}
	newUS := state.NewUnitState()
	newUS.SetRelationState(newRelationState)
	err = s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestUnitStateMutateState(c *gc.C) {
	// Set initial state; this should create a new unitstate doc
	initState := s.testUnitSuite(c)

	// Mutate state again with an existing state doc
	newCharmState := map[string]string{"foo": "42"}
	newUS := state.NewUnitState()
	newUS.SetCharmState(newCharmState)
	err := s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Ensure state changed
	uState, err := s.unit.State()
	c.Assert(err, gc.IsNil)
	assertUnitStateCharmState(c, uState, newCharmState)

	// Ensure the other state did not.
	assertUnitStateUniterState(c, uState, initState.uniterState)
	assertUnitStateRelationState(c, uState, initState.relationState)
	assertUnitStateStorageState(c, uState, initState.storageState)
}

func (s *UnitSuite) TestUnitStateMutateUniterState(c *gc.C) {
	// Set initial state; this should create a new unitstate doc
	initState := s.testUnitSuite(c)

	// Mutate uniter state again with an existing state doc
	newUniterState := "new"
	newUS := state.NewUnitState()
	newUS.SetUniterState(newUniterState)
	err := s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Ensure uniter state changed
	uState, err := s.unit.State()
	c.Assert(err, gc.IsNil)
	assertUnitStateUniterState(c, uState, newUniterState)

	// Ensure the other state did not.
	assertUnitStateCharmState(c, uState, initState.charmState)
	assertUnitStateRelationState(c, uState, initState.relationState)
	assertUnitStateStorageState(c, uState, initState.storageState)
}

func (s *UnitSuite) TestUnitStateMutateChangeRelationState(c *gc.C) {
	// Set initial state; this should create a new unitstate doc
	initState := s.testUnitSuite(c)

	// Mutate relation state again with an existing state doc
	// by changing a value.
	expectedRelationState := initState.relationState
	expectedRelationState[1] = "five"
	newUS := state.NewUnitState()
	newUS.SetRelationState(expectedRelationState)
	err := s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Ensure relation state changed
	uState, err := s.unit.State()
	c.Assert(err, gc.IsNil)
	assertUnitStateRelationState(c, uState, expectedRelationState)

	// Ensure the other state did not.
	assertUnitStateCharmState(c, uState, initState.charmState)
	assertUnitStateUniterState(c, uState, initState.uniterState)
	assertUnitStateStorageState(c, uState, initState.storageState)
}

func (s *UnitSuite) TestUnitStateMutateDeleteRelationState(c *gc.C) {
	// Set initial state; this should create a new unitstate doc
	initState := s.testUnitSuite(c)

	// Mutate relation state again with an existing state doc
	// by deleting value.
	expectedRelationState := initState.relationState
	delete(expectedRelationState, 2)
	newUS := state.NewUnitState()
	newUS.SetRelationState(expectedRelationState)
	err := s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Ensure relation state changed
	uState, err := s.unit.State()
	c.Assert(err, gc.IsNil)
	assertUnitStateRelationState(c, uState, expectedRelationState)

	// Ensure the other state did not.
	assertUnitStateCharmState(c, uState, initState.charmState)
	assertUnitStateUniterState(c, uState, initState.uniterState)
	assertUnitStateStorageState(c, uState, initState.storageState)
}

func (s *UnitSuite) TestUnitStateMutateStorageState(c *gc.C) {
	// Set initial state; this should create a new unitstate doc
	initState := s.testUnitSuite(c)

	// Mutate storage state again with an existing state doc
	newStorageState := "state"
	newUS := state.NewUnitState()
	newUS.SetStorageState(newStorageState)
	err := s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Ensure storage state changed
	uState, err := s.unit.State()
	c.Assert(err, gc.IsNil)
	assertUnitStateStorageState(c, uState, newStorageState)

	// Ensure the other state did not.
	assertUnitStateCharmState(c, uState, initState.charmState)
	assertUnitStateUniterState(c, uState, initState.uniterState)
	assertUnitStateRelationState(c, uState, initState.relationState)
}

func (s *UnitSuite) TestUnitStateDeleteState(c *gc.C) {
	// Set initial state; this should create a new unitstate doc
	initState := s.testUnitSuite(c)

	// Mutate state again with an existing state doc
	newUS := state.NewUnitState()
	newUS.SetCharmState(map[string]string{})
	err := s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Ensure state changed
	uState, err := s.unit.State()
	c.Assert(err, jc.ErrorIsNil)
	st, _ := uState.CharmState()
	c.Assert(st, gc.IsNil, gc.Commentf("expected to receive a nil map when no state doc is present"))

	// Ensure the other state did not.
	assertUnitStateUniterState(c, uState, initState.uniterState)
	assertUnitStateRelationState(c, uState, initState.relationState)
	assertUnitStateStorageState(c, uState, initState.storageState)
}

func (s *UnitSuite) TestUnitStateDeleteRelationState(c *gc.C) {
	// Set initial state; this should create a new unitstate doc
	initState := s.testUnitSuite(c)

	// Mutate state again with an existing state doc
	newUS := state.NewUnitState()
	newUS.SetRelationState(map[int]string{})
	err := s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Ensure state changed
	uState, err := s.unit.State()
	c.Assert(err, jc.ErrorIsNil)
	st, _ := uState.RelationState()
	c.Assert(st, gc.IsNil, gc.Commentf("expected to receive a nil map when no state doc is present"))

	// Ensure the other state did not.
	assertUnitStateCharmState(c, uState, initState.charmState)
	assertUnitStateUniterState(c, uState, initState.uniterState)
	assertUnitStateStorageState(c, uState, initState.storageState)
}

type initialUnitState struct {
	charmState    map[string]string
	uniterState   string
	relationState map[int]string
	storageState  string
}

func (s *UnitSuite) testUnitSuite(c *gc.C) initialUnitState {
	// Set initial state; this should create a new unitstate doc
	initialCharmState := map[string]string{
		"foo":          "bar",
		"key.with.dot": "must work",
		"key.with.$":   "must work to",
	}
	initialUniterState := "testing"
	initialRelationState := map[int]string{
		1: "one",
		2: "two",
	}
	initialStorageState := "gnitset"

	us := state.NewUnitState()
	us.SetCharmState(initialCharmState)
	us.SetUniterState(initialUniterState)
	us.SetRelationState(initialRelationState)
	us.SetStorageState(initialStorageState)
	err := s.unit.SetState(us, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Read back initial state
	uState, err := s.unit.State()
	c.Assert(err, gc.IsNil)
	obtainedCharmState, found := uState.CharmState()
	c.Assert(found, jc.IsTrue)
	c.Assert(obtainedCharmState, gc.DeepEquals, initialCharmState)
	obtainedUniterState, found := uState.UniterState()
	c.Assert(found, jc.IsTrue)
	c.Assert(obtainedUniterState, gc.Equals, initialUniterState)
	obtainedRelationState, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(obtainedRelationState, gc.DeepEquals, initialRelationState)
	obtainedStorageState, found := uState.StorageState()
	c.Assert(found, jc.IsTrue)
	c.Assert(obtainedStorageState, gc.Equals, initialStorageState)

	return initialUnitState{
		charmState:    initialCharmState,
		uniterState:   initialUniterState,
		relationState: initialRelationState,
		storageState:  initialStorageState,
	}
}

func assertUnitStateCharmState(c *gc.C, uState *state.UnitState, expected map[string]string) {
	obtained, found := uState.CharmState()
	c.Assert(found, jc.IsTrue)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func assertUnitStateUniterState(c *gc.C, uState *state.UnitState, expected string) {
	obtained, found := uState.UniterState()
	c.Assert(found, jc.IsTrue)
	c.Assert(obtained, gc.Equals, expected)
}

func assertUnitStateRelationState(c *gc.C, uState *state.UnitState, expected map[int]string) {
	obtained, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func assertUnitStateStorageState(c *gc.C, uState *state.UnitState, expected string) {
	obtained, found := uState.StorageState()
	c.Assert(found, jc.IsTrue)
	c.Assert(obtained, gc.Equals, expected)
}

func (s *UnitSuite) TestUnitStateNopMutation(c *gc.C) {
	// Set initial state; this should create a new unitstate doc
	initialCharmState := map[string]string{
		"foo":          "bar",
		"key.with.dot": "must work",
		"key.with.$":   "must work to",
	}
	initialUniterState := "unit state"
	initialRelationState := map[int]string{
		1: "one",
		2: "two",
		3: "three",
	}
	initialStorageState := "storage state"
	iUnitState := state.NewUnitState()
	iUnitState.SetCharmState(initialCharmState)
	iUnitState.SetUniterState(initialUniterState)
	iUnitState.SetRelationState(initialRelationState)
	iUnitState.SetStorageState(initialStorageState)
	err := s.unit.SetState(iUnitState, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	// Read revno
	txnDoc := struct {
		TxnRevno int64 `bson:"txn-revno"`
	}{}
	coll := s.Session.DB("juju").C("unitstates")
	err = coll.Find(nil).One(&txnDoc)
	c.Assert(err, gc.IsNil)
	curRevNo := txnDoc.TxnRevno

	// Set state using the same KV pairs; this should be a no-op
	err = s.unit.SetState(iUnitState, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	err = coll.Find(nil).One(&txnDoc)
	c.Assert(err, gc.IsNil)
	c.Assert(txnDoc.TxnRevno, gc.Equals, curRevNo, gc.Commentf("expected state doc revno to remain the same"))

	// Set state using a different set of KV pairs
	sUnitState := state.NewUnitState()
	sUnitState.SetCharmState(map[string]string{"something": "else"})
	err = s.unit.SetState(sUnitState, state.UnitStateSizeLimits{})
	c.Assert(err, gc.IsNil)

	err = coll.Find(nil).One(&txnDoc)
	c.Assert(err, gc.IsNil)
	c.Assert(txnDoc.TxnRevno, jc.GreaterThan, curRevNo, gc.Commentf("expected state doc revno to be bumped"))
}

func (s *UnitSuite) TestConfigSettingsNeedCharmURLSet(c *gc.C) {
	_, err := s.unit.ConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit's charm URL must be set before retrieving config")
}

func (s *UnitSuite) TestConfigSettingsIncludeDefaults(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

func (s *UnitSuite) TestConfigSettingsReflectApplication(c *gc.C) {
	err := s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "no title"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "no title"})

	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "ironic title"})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "ironic title"})
}

func (s *UnitSuite) TestConfigSettingsReflectCharm(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	newCharm := s.AddConfigCharm(c, "wordpress", "options: {}", 123)
	cfg := state.SetCharmConfig{Charm: newCharm, CharmOrigin: defaultCharmOrigin(newCharm.URL())}
	err = s.application.SetCharm(cfg, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	// Settings still reflect charm set on unit.
	settings, err := s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// When the unit has the new charm set, it'll see the new config.
	err = s.unit.SetCharmURL(newCharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	settings, err = s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{})
}

func (s *UnitSuite) TestWatchConfigSettingsNeedsCharmURL(c *gc.C) {
	_, err := s.unit.WatchConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit's charm URL must be set before watching config")
}

func (s *UnitSuite) TestWatchConfigSettings(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w, err := s.unit.WatchConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "superhero paparazzi"})
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application's charm; nothing detected.
	newCharm := s.AddConfigCharm(c, "wordpress", floatConfig, 123)
	cfg := state.SetCharmConfig{Charm: newCharm, CharmOrigin: defaultCharmOrigin(newCharm.URL())}
	err = s.application.SetCharm(cfg, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application config for new charm; nothing detected.
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{
		"key": 42.0,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// NOTE: if we were to change the unit to use the new charm, we'd see
	// another event, because the originally-watched document will become
	// unreferenced and be removed. But I'm not testing that behaviour
	// because it's not very helpful and subject to change.
}

func (s *UnitSuite) TestWatchConfigSettingsHash(c *gc.C) {
	newCharm := s.AddConfigCharm(c, "wordpress", sortableConfig, 123)
	cfg := state.SetCharmConfig{Charm: newCharm, CharmOrigin: defaultCharmOrigin(newCharm.URL())}
	err := s.application.SetCharm(cfg, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetCharmURL(newCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w, err := s.unit.WatchConfigSettingsHash()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Initial event.
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("606cdac123d3d8d3031fe93db3d5afd9c95f709dfbc17e5eade1332b081ec6f9")

	// Non-change is not reported.
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Add an attribute that comes before.
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{
		"alphabetic": 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Sanity check.
	config, err := s.unit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config, gc.DeepEquals, charm.Settings{
		// Ints that get round-tripped through mongo come back as int64.
		"alphabetic": int64(1),
		"blog-title": "sauceror central",
		"zygomatic":  nil,
	})
	wc.AssertChange("886b9586df944be35b189e5678a85ac0b2ed4da46fcb10cecfd8c06b6d1baf0f")

	// And one that comes after.
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"zygomatic": 23})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("9ca0a511647f09db8fab07be3c70f49cc93f38f300ca82d6e26ec7d4a8b1b4c2")

	// Setting a value to int64 instead of int has no effect on the
	// hash (the information always comes back as int64).
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"alphabetic": int64(1)})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application's charm; nothing detected.
	newCharm = s.AddConfigCharm(c, "wordpress", floatConfig, 125)
	cfg = state.SetCharmConfig{Charm: newCharm, CharmOrigin: defaultCharmOrigin(newCharm.URL())}
	err = s.application.SetCharm(cfg, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application config for new charm; nothing detected.
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"key": 42.0})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// NOTE: if we were to change the unit to use the new charm, we'd see
	// another event, because the originally-watched document will become
	// unreferenced and be removed. But I'm not testing that behaviour
	// because it's not very helpful and subject to change.

	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("")
}

func (s *UnitSuite) TestConfigHashesDifferentForDifferentCharms(c *gc.C) {
	// Config hashes should be different if the charm url changes,
	// even if the config is otherwise unchanged. This ensures that
	// config-changed will be run on a unit after its charm is
	// upgraded.
	c.Logf("charm url %s", s.charm.URL())
	err := s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)

	w1, err := s.unit.WatchConfigSettingsHash()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w1)

	wc1 := testing.NewStringsWatcherC(c, w1)
	wc1.AssertChange("2e1f49c3e8b53892b822558401af33589522094681276a98458595114e04c0c1")

	newCharm := s.AddConfigCharm(c, "wordpress", wordpressConfig, 125)
	cfg := state.SetCharmConfig{Charm: newCharm, CharmOrigin: defaultCharmOrigin(newCharm.URL())}
	err = s.application.SetCharm(cfg, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("new charm url %s", newCharm.URL())
	err = s.unit.SetCharmURL(newCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	w3, err := s.unit.WatchConfigSettingsHash()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w3)

	wc3 := testing.NewStringsWatcherC(c, w3)
	wc3.AssertChange("35412457529c9e0b64b7642ad0f76137ee13b104c94136d0c18b2fe54ddf5d36")
}

func (s *UnitSuite) TestWatchApplicationConfigSettingsHash(c *gc.C) {
	w, err := s.unit.WatchApplicationConfigSettingsHash()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("92652ce7679e295c6567a3891c562dcab727c71543f8c1c3a38c3626ce064019")

	schema := environschema.Fields{
		"username":    environschema.Attr{Type: environschema.Tstring},
		"alive":       environschema.Attr{Type: environschema.Tbool},
		"skill-level": environschema.Attr{Type: environschema.Tint},
		"options":     environschema.Attr{Type: environschema.Tattrs},
	}

	err = s.application.UpdateApplicationConfig(config.ConfigAttributes{
		"username":    "abbas",
		"alive":       true,
		"skill-level": 23,
		"options": map[string]string{
			"fortuna": "crescis",
			"luna":    "velut",
			"status":  "malus",
		},
	}, nil, schema, nil)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange("fa3b92db7c37210ed80da08c522abbe8ffffc9982ad2136342f3df8f221b5768")

	// For reasons that I don't understand, application config
	// converts int64s to int while charm config converts ints to
	// int64 - I'm not going to try to untangle that now.
	err = s.application.UpdateApplicationConfig(config.ConfigAttributes{
		"username":    "bob",
		"skill-level": int64(23),
	}, nil, schema, nil)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange("d437a3b548fecf7f8c7748bf8d2acab0960ceb53f316a22803a6ca7442a9b8b3")
}

func (s *UnitSuite) addSubordinateUnit(c *gc.C) *state.Unit {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingApplication(c, "logging", subCharm)
	eps, err := s.State.InferEndpoints("wordpress", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	subUnit, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	return subUnit
}

func (s *UnitSuite) setAssignedMachineAddresses(c *gc.C, u *state.Unit) {
	mid, err := u.AssignedMachineId()
	if errors.Is(err, errors.NotAssigned) {
		err = u.AssignToNewMachine(defaultInstancePrechecker)
		c.Assert(err, jc.ErrorIsNil)
		mid, err = u.AssignedMachineId()
	}
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-exist", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	controllerConfig := coretesting.FakeControllerConfig()
	err = machine.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("private.address.example.com", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("public.address.example.com", network.WithScope(network.ScopePublic)),
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestPublicAddressSubordinate(c *gc.C) {
	// A subordinate unit will never be created without the principal
	// being assigned to a machine.
	s.setAssignedMachineAddresses(c, s.unit)
	subUnit := s.addSubordinateUnit(c)
	address, err := subUnit.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address.Value, gc.Equals, "public.address.example.com")
}

func (s *UnitSuite) TestAllAddresses(c *gc.C) {
	// Only used for CAAS units.
	all, err := s.unit.AllAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *UnitSuite) TestPublicAddress(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.unit.PublicAddress()
	c.Assert(err, jc.Satisfies, network.IsNoAddressError)

	public := network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic))
	private := network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeCloudLocal))

	controllerConfig := coretesting.FakeControllerConfig()

	err = machine.SetProviderAddresses(controllerConfig, public, private)
	c.Assert(err, jc.ErrorIsNil)

	address, err := s.unit.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(address.Value, gc.Equals, "8.8.8.8")
}

func (s *UnitSuite) TestStablePrivateAddress(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig := coretesting.FakeControllerConfig()

	err = machine.SetMachineAddresses(controllerConfig, network.NewSpaceAddress("10.0.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	// Now add an address that would previously have sorted before the
	// default.
	err = machine.SetMachineAddresses(controllerConfig, network.NewSpaceAddress("10.0.0.1"), network.NewSpaceAddress("10.0.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	// Assert the address is unchanged.
	addr, err := s.unit.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "10.0.0.2")
}

func (s *UnitSuite) TestStablePublicAddress(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfig := coretesting.FakeControllerConfig()

	err = machine.SetProviderAddresses(controllerConfig, network.NewSpaceAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)

	// Now add an address that would previously have sorted before the
	// default.
	err = machine.SetProviderAddresses(controllerConfig, network.NewSpaceAddress("8.8.4.4"), network.NewSpaceAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)

	// Assert the address is unchanged.
	addr, err := s.unit.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr.Value, gc.Equals, "8.8.8.8")
}

func (s *UnitSuite) TestPublicAddressMachineAddresses(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	publicProvider := network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic))
	privateProvider := network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeCloudLocal))
	privateMachine := network.NewSpaceAddress("127.0.0.2")

	controllerConfig := coretesting.FakeControllerConfig()

	err = machine.SetProviderAddresses(controllerConfig, privateProvider)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetMachineAddresses(controllerConfig, privateMachine)
	c.Assert(err, jc.ErrorIsNil)
	address, err := s.unit.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(address.Value, gc.Equals, "127.0.0.1")

	err = machine.SetProviderAddresses(controllerConfig, publicProvider, privateProvider)
	c.Assert(err, jc.ErrorIsNil)
	address, err = s.unit.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(address.Value, gc.Equals, "8.8.8.8")
}

func (s *UnitSuite) TestPrivateAddressSubordinate(c *gc.C) {
	// A subordinate unit will never be created without the principal
	// being assigned to a machine.
	s.setAssignedMachineAddresses(c, s.unit)
	subUnit := s.addSubordinateUnit(c)
	address, err := subUnit.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address.Value, gc.Equals, "private.address.example.com")
}

func (s *UnitSuite) TestPrivateAddress(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.unit.PrivateAddress()
	c.Assert(err, jc.Satisfies, network.IsNoAddressError)

	public := network.NewSpaceAddress("8.8.8.8", network.WithScope(network.ScopePublic))
	private := network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeCloudLocal))

	controllerConfig := coretesting.FakeControllerConfig()

	err = machine.SetProviderAddresses(controllerConfig, public, private)
	c.Assert(err, jc.ErrorIsNil)

	address, err := s.unit.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(address.Value, gc.Equals, "127.0.0.1")
}

type destroyMachineTestCase struct {
	target    *state.Unit
	host      *state.Machine
	desc      string
	destroyed bool
}

func (s *UnitSuite) destroyMachineTestCases(c *gc.C) []destroyMachineTestCase {
	var result []destroyMachineTestCase
	var err error

	{
		tc := destroyMachineTestCase{desc: "standalone principal", destroyed: true}
		tc.host, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		tc.target, err = s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)
		result = append(result, tc)
	}
	{
		tc := destroyMachineTestCase{desc: "co-located principals", destroyed: false}
		tc.host, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		tc.target, err = s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)
		colocated, err := s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(colocated.AssignToMachine(tc.host), gc.IsNil)

		result = append(result, tc)
	}
	{
		tc := destroyMachineTestCase{desc: "host has container", destroyed: false}
		tc.host, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		_, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		}, tc.host.Id(), instance.LXD)
		c.Assert(err, jc.ErrorIsNil)
		tc.target, err = s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)

		result = append(result, tc)
	}
	{
		tc := destroyMachineTestCase{desc: "host has vote", destroyed: false}
		tc.host, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		_, _, err = s.State.EnableHA(defaultInstancePrechecker, 1, constraints.Value{}, state.UbuntuBase("12.10"), []string{tc.host.Id()})
		c.Assert(err, jc.ErrorIsNil)
		node, err := s.State.ControllerNode(tc.host.Id())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(node.SetHasVote(true), gc.IsNil)
		tc.target, err = s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)

		result = append(result, tc)
	}
	{
		tc := destroyMachineTestCase{desc: "unassigned unit", destroyed: true}
		tc.host, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		tc.target, err = s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.AssignToMachine(tc.host), gc.IsNil)
		result = append(result, tc)
	}

	return result
}

func (s *UnitSuite) TestRemoveUnitMachineFastForwardDestroy(c *gc.C) {
	for _, tc := range s.destroyMachineTestCases(c) {
		c.Log(tc.desc)
		err := tc.host.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tc.target.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.IsNil)
		if tc.destroyed {
			assertLife(c, tc.host, state.Dying)
			c.Assert(tc.host.EnsureDead(), gc.IsNil)
		} else {
			assertLife(c, tc.host, state.Alive)
			c.Assert(tc.host.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.NotNil)
		}
	}
}

func (s *UnitSuite) TestRemoveUnitMachineNoFastForwardDestroy(c *gc.C) {
	for _, tc := range s.destroyMachineTestCases(c) {
		c.Log(tc.desc)
		err := tc.host.SetProvisioned("inst-id", "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		preventUnitDestroyRemove(c, tc.target)
		c.Assert(tc.target.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.IsNil)
		c.Assert(tc.target.EnsureDead(), gc.IsNil)
		assertLife(c, tc.host, state.Alive)
		c.Assert(tc.target.Remove(state.NewObjectStore(c, s.State.ModelUUID())), gc.IsNil)
		if tc.destroyed {
			assertLife(c, tc.host, state.Dying)
		} else {
			assertLife(c, tc.host, state.Alive)
			c.Assert(tc.host.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.NotNil)
		}
	}
}

func (s *UnitSuite) TestRemoveUnitMachineNoDestroy(c *gc.C) {
	charmWithOut := s.AddTestingCharm(c, "mysql")
	applicationWithOutProfile := s.AddTestingApplication(c, "mysql", charmWithOut)

	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	target, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)

	colocated, err := applicationWithOutProfile.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(colocated.AssignToMachine(host), gc.IsNil)
	c.Assert(colocated.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.IsNil)
	assertLife(c, host, state.Alive)

	c.Assert(host.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.NotNil)
}

func (s *UnitSuite) setControllerVote(c *gc.C, id string, hasVote bool) {
	node, err := s.State.ControllerNode(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.SetHasVote(hasVote), gc.IsNil)
}

func (s *UnitSuite) demoteController(c *gc.C, m *state.Machine) {
	s.setControllerVote(c, m.Id(), false)
	c.Assert(state.SetWantsVote(s.State, m.Id(), false), jc.ErrorIsNil)
	node, err := s.State.ControllerNode(m.Id())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveControllerReference(node)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestRemoveUnitMachineThrashed(c *gc.C) {
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.State.EnableHA(defaultInstancePrechecker, 3, constraints.Value{}, state.UbuntuBase("12.10"), []string{host.Id()})
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.setControllerVote(c, host.Id(), true)
	target, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)
	flip := jujutxn.TestHook{
		Before: func() {
			s.demoteController(c, host)
		},
	}
	flop := jujutxn.TestHook{
		Before: func() {
			s.setControllerVote(c, host.Id(), true)
		},
	}
	state.SetMaxTxnAttempts(c, s.State, 3)
	defer state.SetTestHooks(c, s.State, flip, flop, flip).Check()

	c.Assert(target.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.ErrorMatches, `cannot destroy unit "wordpress/1": state changing too quickly; try again soon`)
}

func (s *UnitSuite) TestRemoveUnitMachineRetryVoter(c *gc.C) {
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.State.EnableHA(defaultInstancePrechecker, 3, constraints.Value{}, state.UbuntuBase("12.10"), []string{host.Id()})
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		s.setControllerVote(c, host.Id(), true)
	}).Check()

	c.Assert(target.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.IsNil)
	assertLife(c, host, state.Alive)
}

func (s *UnitSuite) TestRemoveUnitMachineRetryNoVoter(c *gc.C) {
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.State.EnableHA(defaultInstancePrechecker, 3, constraints.Value{}, state.UbuntuBase("12.10"), []string{host.Id()})
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)
	s.setControllerVote(c, host.Id(), true)

	defer state.SetBeforeHooks(c, s.State, func() {
		s.demoteController(c, host)
	}, nil).Check()

	c.Assert(target.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.IsNil)
	assertLife(c, host, state.Dying)
}

func (s *UnitSuite) TestRemoveUnitMachineRetryContainer(c *gc.C) {
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)
	defer state.SetTestHooks(c, s.State, jujutxn.TestHook{
		Before: func() {
			machine, err := s.State.AddMachineInsideMachine(state.MachineTemplate{
				Base: state.UbuntuBase("12.10"),
				Jobs: []state.MachineJob{state.JobHostUnits},
			}, host.Id(), instance.LXD)
			c.Assert(err, jc.ErrorIsNil)
			assertLife(c, machine, state.Alive)

			// test-setup verification for the disqualifying machine.
			hostHandle, err := s.State.Machine(host.Id())
			c.Assert(err, jc.ErrorIsNil)
			containers, err := hostHandle.Containers()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(containers, gc.HasLen, 1)
		},
	}).Check()

	c.Assert(target.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.IsNil)
	assertLife(c, host, state.Alive)
}

func (s *UnitSuite) TestRemoveUnitMachineRetryOrCond(c *gc.C) {
	host, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.State.EnableHA(defaultInstancePrechecker, 1, constraints.Value{}, state.UbuntuBase("12.10"), []string{host.Id()})
	c.Assert(err, jc.ErrorIsNil)
	err = host.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	target, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(target.AssignToMachine(host), gc.IsNil)

	// This unit will be colocated in the transaction hook to cause a retry.
	colocated, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	s.setControllerVote(c, host.Id(), true)

	defer state.SetTestHooks(c, s.State, jujutxn.TestHook{
		Before: func() {
			// Original assertion preventing host removal is no longer valid
			s.setControllerVote(c, host.Id(), false)

			// But now the host gets a colocated unit, a different condition preventing removal
			hostHandle, err := s.State.Machine(host.Id())
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(colocated.AssignToMachine(hostHandle), gc.IsNil)
		},
	}).Check()

	c.Assert(target.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), gc.IsNil)
	assertLife(c, host, state.Alive)
}

func (s *UnitSuite) TestRemoveUnitWRelationLastUnit(c *gc.C) {
	// This will assign it to a machine, and make it look like the machine agent is active.
	preventUnitDestroyRemove(c, s.unit)
	mysqlCharm := s.AddTestingCharm(c, "mysql")
	mysqlApp := s.AddTestingApplication(c, "mysql", mysqlCharm)
	mysqlUnit, err := mysqlApp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysqlUnit.AssignToNewMachine(defaultInstancePrechecker), jc.ErrorIsNil)
	endpoints, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	relationUnit, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relationUnit.EnterScope(nil), jc.ErrorIsNil)
	c.Assert(s.application.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	assertLife(c, s.application, state.Dying)
	c.Assert(s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	assertLife(c, s.unit, state.Dying)
	assertLife(c, s.application, state.Dying)
	c.Assert(s.unit.EnsureDead(), jc.ErrorIsNil)
	assertLife(c, s.application, state.Dying)
	c.Assert(s.unit.Remove(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{}), jc.ErrorIsNil)
	// Now the application should be gone
	c.Assert(s.application.Refresh(), jc.ErrorIs, errors.NotFound)
}

func (s *UnitSuite) TestRefresh(c *gc.C) {
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.SetPassword("arble-farble-dying-yarble")
	c.Assert(err, jc.ErrorIsNil)
	valid := unit1.PasswordValid("arble-farble-dying-yarble")
	c.Assert(valid, jc.IsFalse)
	err = unit1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	valid = unit1.PasswordValid("arble-farble-dying-yarble")
	c.Assert(valid, jc.IsTrue)

	err = unit1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit1.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = unit1.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *UnitSuite) TestSetCharmURLSuccess(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	curl := s.unit.CharmURL()
	c.Assert(curl, gc.IsNil)

	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)

	curl = s.unit.CharmURL()
	c.Assert(curl, gc.NotNil)
	c.Assert(*curl, gc.Equals, s.charm.URL())
}

func (s *UnitSuite) TestSetCharmURLFailures(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	curl := s.unit.CharmURL()
	c.Assert(curl, gc.IsNil)

	err := s.unit.SetCharmURL("")
	c.Assert(err, gc.ErrorMatches, "cannot set empty charm url")

	err = s.unit.SetCharmURL("ch:missing/one-1")
	c.Assert(err, gc.ErrorMatches, `unknown charm url "ch:missing/one-1"`)

	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.Equals, stateerrors.ErrDead)
}

func (s *UnitSuite) TestSetCharmURLWithRemovedUnit(c *gc.C) {
	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.State, s.unit)

	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.Equals, stateerrors.ErrDead)
}

func (s *UnitSuite) TestSetCharmURLWithDyingUnit(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.unit, state.Dying)

	err = s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)

	curl := s.unit.CharmURL()
	c.Assert(curl, gc.NotNil)
	c.Assert(*curl, gc.Equals, s.charm.URL())
}

func (s *UnitSuite) TestSetCharmURLRetriesWithDeadUnit(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
		c.Assert(err, jc.ErrorIsNil)
		err = s.unit.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.unit, state.Dead)
	}).Check()

	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, gc.Equals, stateerrors.ErrDead)
}

func (s *UnitSuite) TestSetCharmURLRetriesWithDifferentURL(c *gc.C) {
	sch := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Set a different charm to force a retry: first on
				// the application, so the settings are created, then on
				// the unit.
				cfg := state.SetCharmConfig{Charm: sch, CharmOrigin: defaultCharmOrigin(sch.URL())}
				err := s.application.SetCharm(cfg, state.NewObjectStore(c, s.State.ModelUUID()))
				c.Assert(err, jc.ErrorIsNil)
				err = s.unit.SetCharmURL(sch.URL())
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				// Set back the same charm on the application, so the
				// settings refcount is correct..
				cfg := state.SetCharmConfig{Charm: s.charm, CharmOrigin: defaultCharmOrigin(s.charm.URL())}
				err := s.application.SetCharm(cfg, state.NewObjectStore(c, s.State.ModelUUID()))
				c.Assert(err, jc.ErrorIsNil)
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a retry.
			After: func() {
				// Verify it worked after the second attempt.
				err := s.unit.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentURL := s.unit.CharmURL()
				c.Assert(currentURL, gc.NotNil)
				c.Assert(*currentURL, gc.Equals, s.charm.URL())
			},
		},
	).Check()

	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroySetStatusRetry(c *gc.C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.AssignToNewMachine(defaultInstancePrechecker)
		c.Assert(err, jc.ErrorIsNil)
		now := coretesting.NonZeroTime()
		sInfo := status.StatusInfo{
			Status:  status.Idle,
			Message: "",
			Since:   &now,
		}
		err = s.unit.SetAgentStatus(sInfo)
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertLife(c, s.unit, state.Dying)
	}).Check()

	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroySetCharmRetry(c *gc.C) {
	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetCharmURL(s.charm.URL())
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertRemoved(c, s.State, s.unit)
	}).Check()

	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroyChangeCharmRetry(c *gc.C) {
	err := s.unit.SetCharmURL(s.charm.URL())
	c.Assert(err, jc.ErrorIsNil)
	newCharm := s.AddConfigCharm(c, "mysql", "options: {}", 99)
	cfg := state.SetCharmConfig{Charm: newCharm, CharmOrigin: defaultCharmOrigin(newCharm.URL())}
	err = s.application.SetCharm(cfg, state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.SetCharmURL(newCharm.URL())
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertRemoved(c, s.State, s.unit)
	}).Check()

	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroyAssignRetry(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.AssignToMachine(machine)
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertRemoved(c, s.State, s.unit)
		// Also check the unit ref was properly removed from the machine doc --
		// if it weren't, we wouldn't be able to make the machine Dead.
		err := machine.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroyUnassignRetry(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.UnassignFromMachine()
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertRemoved(c, s.State, s.unit)
	}).Check()

	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroyAssignErrorRetry(c *gc.C) {
	now := coretesting.NonZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "failed to assign",
		Since:   &now,
	}
	err := s.unit.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIs, errors.NotAssigned)

	defer state.SetRetryHooks(c, s.State, func() {
		err := s.unit.AssignToNewMachine(defaultInstancePrechecker)
		c.Assert(err, jc.ErrorIsNil)
		now := coretesting.NonZeroTime()
		sInfo := status.StatusInfo{
			Status:  status.Idle,
			Message: "",
			Since:   &now,
		}
		err = s.unit.SetAgentStatus(sInfo)
		c.Assert(err, jc.ErrorIsNil)
	}, func() {
		assertLife(c, s.unit, state.Dying)
	}).Check()
	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestShortCircuitDestroyUnit(c *gc.C) {
	// A unit that has not set any status is removed directly.
	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertRemoved(c, s.State, s.unit)
}

func (s *UnitSuite) TestShortCircuitDestroyUnitNotAssigned(c *gc.C) {
	// A unit that has not been assigned is removed directly.
	now := coretesting.NonZeroTime()
	err := s.unit.SetAgentStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "cannot assign",
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertRemoved(c, s.State, s.unit)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyAssignedUnit(c *gc.C) {
	// This test is similar to TestShortCircuitDestroyUnitNotAssigned but
	// the unit is assigned to a machine.
	err := s.unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.NonZeroTime()
	err = s.unit.SetAgentStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "some error",
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertLife(c, s.unit, state.Dying)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithSubordinates(c *gc.C) {
	// A unit with subordinates is just set to Dying.
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertLife(c, s.unit, state.Dying)
}

func (s *UnitSuite) TestCannotShortCircuitDestroyWithAgentStatus(c *gc.C) {
	for i, test := range []struct {
		status status.Status
		info   string
	}{{
		status.Executing, "blah",
	}, {
		status.Idle, "blah",
	}, {
		status.Failed, "blah",
	}, {
		status.Rebooting, "blah",
	}} {
		c.Logf("test %d: %s", i, test.status)
		unit, err := s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = unit.AssignToNewMachine(defaultInstancePrechecker)
		c.Assert(err, jc.ErrorIsNil)
		now := coretesting.NonZeroTime()
		sInfo := status.StatusInfo{
			Status:  test.status,
			Message: test.info,
			Since:   &now,
		}
		err = unit.SetAgentStatus(sInfo)
		c.Assert(err, jc.ErrorIsNil)
		err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(unit.Life(), gc.Equals, state.Dying)
		assertLife(c, unit, state.Dying)
	}
}

func (s *UnitSuite) TestShortCircuitDestroyWithProvisionedMachine(c *gc.C) {
	// A unit assigned to a provisioned machine is still removed directly so
	// long as it has not set status.
	err := s.unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	mid, err := s.unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mid)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-malive", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Life(), gc.Equals, state.Dying)
	assertRemoved(c, s.State, s.unit)
}

func (s *UnitSuite) TestDestroyRemovesStatusHistory(c *gc.C) {
	err := s.unit.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.NonZeroTime()
	for i := 0; i < 10; i++ {
		info := status.StatusInfo{
			Status:  status.Executing,
			Message: fmt.Sprintf("status %d", i),
			Since:   &now,
		}
		err := s.unit.SetAgentStatus(info)
		c.Assert(err, jc.ErrorIsNil)
		info.Status = status.Active
		err = s.unit.SetStatus(info, status.NoopStatusHistoryRecorder)
		c.Assert(err, jc.ErrorIsNil)

		err = s.unit.SetWorkloadVersion(fmt.Sprintf("v.%d", i), status.NoopStatusHistoryRecorder)
		c.Assert(err, jc.ErrorIsNil)
	}

	filter := status.StatusHistoryFilter{Size: 100}
	agentInfo, err := s.unit.AgentHistory().StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(agentInfo), jc.GreaterThan, 9)

	workloadInfo, err := s.unit.StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(workloadInfo), jc.GreaterThan, 9)

	versionInfo, err := s.unit.WorkloadVersionHistory().StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(versionInfo), jc.GreaterThan, 9)

	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	agentInfo, err = s.unit.AgentHistory().StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agentInfo, gc.HasLen, 0)

	workloadInfo, err = s.unit.StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(workloadInfo, gc.HasLen, 0)

	versionInfo, err = s.unit.WorkloadVersionHistory().StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(versionInfo, gc.HasLen, 0)
}

func assertLife(c *gc.C, entity state.LifeRefresher, life state.Life) {
	c.Assert(entity.Refresh(), gc.IsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func assertRemoved(c *gc.C, st *state.State, entity state.Living) {
	store := state.NewObjectStore(c, st.ModelUUID())

	c.Assert(entity.Refresh(), jc.ErrorIs, errors.NotFound)
	c.Assert(entity.Destroy(store), jc.ErrorIsNil)
	if entity, ok := entity.(state.AgentLiving); ok {
		c.Assert(entity.EnsureDead(), jc.ErrorIsNil)
		if err := entity.Remove(store); err != nil {
			c.Assert(err, gc.ErrorMatches, ".*already removed.*")
		}
		err := entity.Refresh()
		c.Assert(err, jc.ErrorIs, errors.NotFound)
	}
}

func (s *UnitSuite) TestUnitsInError(c *gc.C) {
	now := coretesting.NonZeroTime()
	err := s.unit.SetAgentStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "some error",
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Set a non unit error to ensure it's ignored.
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "some machine error",
		Since:   &now,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Add a unit not in error to ensure it's ignored.
	another, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = another.SetAgentStatus(status.StatusInfo{
		Status: status.Allocating,
		Since:  &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	units, err := s.State.UnitsInError()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	c.Assert(units[0], jc.DeepEquals, s.unit)
}

func (s *UnitSuite) TestTag(c *gc.C) {
	c.Assert(s.unit.Tag().String(), gc.Equals, "unit-wordpress-0")
}

func (s *UnitSuite) TestSetPassword(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testSetPassword(c, s.State, func() (state.Authenticator, error) {
		return s.State.Unit(s.unit.Name())
	})
}

func (s *UnitSuite) TestResolve(c *gc.C) {
	err := s.unit.Resolve(true)
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not in an error state`)
	err = s.unit.Resolve(false)
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" is not in an error state`)

	now := coretesting.NonZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "gaaah",
		Since:   &now,
	}
	err = s.unit.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Resolve(true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Resolve(false)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedRetryHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Resolve(false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Resolve(true)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedNoHooks)
}

func (s *UnitSuite) TestGetSetClearResolved(c *gc.C) {
	mode := s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)

	err := s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)

	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNoHooks)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.unit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)
	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.SetResolved(state.ResolvedNone)
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: ""`)
	err = s.unit.SetResolved(state.ResolvedMode("foo"))
	c.Assert(err, gc.ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: "foo"`)
}

func (s *UnitSuite) TesOpenedPorts(c *gc.C) {
	// Accessing the port ranges for the unit should fail if it's not assigned to a machine.
	_, err := s.unit.OpenedPortRanges()
	c.Assert(errors.Cause(err), jc.ErrorIs, errors.NotAssigned)

	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	// Verify no open ports before activity.
	unitPortRanges, err := s.unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPortRanges.UniquePortRanges(), gc.HasLen, 0)

	// Now open and close ports and ranges and check that they are persisted correctly
	s.assertPortRangesAfterOpenClose(c, s.unit,
		[]network.PortRange{
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("100-200/udp"),
		},
		nil, // close
		[]network.PortRange{
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("100-200/udp"),
		},
	)

	// Open a new port (53/udp)
	s.assertPortRangesAfterOpenClose(c, s.unit,
		[]network.PortRange{
			network.MustParsePortRange("53/udp"),
		},
		nil, // close
		[]network.PortRange{
			network.MustParsePortRange("53/udp"),
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("100-200/udp"),
		},
	)

	// Open same port but different protocol (53/tcp)
	s.assertPortRangesAfterOpenClose(c, s.unit,
		[]network.PortRange{
			network.MustParsePortRange("53/tcp"),
		},
		nil, // close
		[]network.PortRange{
			network.MustParsePortRange("53/tcp"),
			network.MustParsePortRange("53/udp"),
			network.MustParsePortRange("80/tcp"),
			network.MustParsePortRange("100-200/udp"),
		},
	)

	// Close an existing port (80/tcp)
	s.assertPortRangesAfterOpenClose(c, s.unit,
		nil, // open
		[]network.PortRange{
			network.MustParsePortRange("80/tcp"),
		},
		[]network.PortRange{
			network.MustParsePortRange("53/tcp"),
			network.MustParsePortRange("53/udp"),
			network.MustParsePortRange("100-200/udp"),
		},
	)

	// Close another existing port (100-200/udp)
	s.assertPortRangesAfterOpenClose(c, s.unit,
		nil, // open
		[]network.PortRange{
			network.MustParsePortRange("100-200/udp"),
		},
		[]network.PortRange{
			network.MustParsePortRange("53/tcp"),
			network.MustParsePortRange("53/udp"),
		},
	)
}

func (s *UnitSuite) assertPortRangesAfterOpenClose(c *gc.C, u *state.Unit, openRanges, closeRanges, exp []network.PortRange) {
	unitPortRanges, err := u.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	for _, pr := range openRanges {
		unitPortRanges.Open(allEndpoints, pr)
	}
	for _, pr := range closeRanges {
		unitPortRanges.Close(allEndpoints, pr)
	}
	c.Assert(s.State.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)

	// Reload ranges
	unitPortRanges, err = u.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPortRanges.UniquePortRanges(), gc.DeepEquals, exp)
}

func (s *UnitSuite) TestOpenClosePortWhenDying(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	preventUnitDestroyRemove(c, s.unit)

	store := state.NewObjectStore(c, s.State.ModelUUID())

	// Ensure that we use a monotonically increasing port counter to
	// avoid no-op open port operations since the same unit is used for
	// both dying and dead unit states.
	nextPort := 1337
	testWhenDying(c, store, s.unit, noErr, contentionErr, func() error {
		// Open a port range
		unitPortRanges, err := s.unit.OpenedPortRanges()
		if err != nil {
			return errors.Annotatef(err, "cannot open port ranges")
		}

		toOpen := fmt.Sprintf("%d/tcp", nextPort)
		nextPort++
		unitPortRanges.Open(allEndpoints, network.MustParsePortRange(toOpen))
		if err = s.State.ApplyOperation(unitPortRanges.Changes()); err != nil {
			return errors.Annotatef(err, "cannot open port ranges")
		}

		err = s.unit.Refresh()
		if err != nil {
			return err
		}

		// Open another port range
		toOpen = fmt.Sprintf("%d/tcp", nextPort)
		nextPort++
		unitPortRanges, err = s.unit.OpenedPortRanges()
		if err != nil {
			return errors.Annotatef(err, "cannot open port ranges")
		}
		unitPortRanges.Open(allEndpoints, network.MustParsePortRange(toOpen))
		if err = s.State.ApplyOperation(unitPortRanges.Changes()); err != nil {
			return errors.Annotatef(err, "cannot open port ranges")
		}
		return nil
	})
}

func (s *UnitSuite) TestRemoveLastUnitOnMachineRemovesAllPorts(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	machPortRanges, err := machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machPortRanges.UniquePortRanges(), gc.HasLen, 0)

	state.MustOpenUnitPortRange(c, s.State, machine, s.unit.Name(), allEndpoints, network.MustParsePortRange("100-200/tcp"))

	machPortRanges, err = machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machPortRanges.UniquePortRanges(), gc.HasLen, 1)

	// Now remove the unit and check again.
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	// Because that was the only range open, the ports doc will be
	// removed as well.
	machPortRanges, err = machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machPortRanges.UniquePortRanges(), gc.HasLen, 0)
}

func (s *UnitSuite) TestRemoveUnitRemovesItsPortsOnly(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	otherUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = otherUnit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	state.MustOpenUnitPortRange(c, s.State, machine, s.unit.Name(), allEndpoints, network.MustParsePortRange("100-200/tcp"))
	state.MustOpenUnitPortRange(c, s.State, machine, otherUnit.Name(), allEndpoints, network.MustParsePortRange("300-400/udp"))

	machPortRanges, err := machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machPortRanges.UniquePortRanges(), gc.HasLen, 2)

	c.Assert(machPortRanges.ForUnit(s.unit.Name()).UniquePortRanges(), jc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	})
	c.Assert(machPortRanges.ForUnit(otherUnit.Name()).UniquePortRanges(), jc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("300-400/udp"),
	})

	// Now remove the first unit and check again.
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	// Verify only otherUnit still has open ports.
	machPortRanges, err = machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machPortRanges.UniquePortRanges(), gc.HasLen, 1)
	c.Assert(machPortRanges.ForUnit(s.unit.Name()).UniquePortRanges(), gc.HasLen, 0)
	c.Assert(machPortRanges.ForUnit(otherUnit.Name()).UniquePortRanges(), jc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("300-400/udp"),
	})
}

func (s *UnitSuite) TestRemoveUnitDeletesUnitState(c *gc.C) {
	// Create unit state document
	us := state.NewUnitState()
	us.SetCharmState(map[string]string{"speed": "ludicrous"})
	err := s.unit.SetState(us, state.UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)

	coll := s.Session.DB("juju").C("unitstates")
	numDocs, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numDocs, gc.Equals, 1, gc.Commentf("expected a new document for the unit state to be created"))

	// Destroy unit; this should also purge the state doc for the unit
	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	numDocs, err = coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numDocs, gc.Equals, 0, gc.Commentf("expected unit state document to be removed when the unit is destroyed"))

	// Any attempts to read/write a unit's state when not Alive should fail
	_, err = s.unit.State()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	newUS := state.NewUnitState()
	newUS.SetCharmState(map[string]string{"foo": "bar"})
	err = s.unit.SetState(newUS, state.UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *UnitSuite) TestDestroyAlsoDeletesSecretPermissions(c *gc.C) {
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.application.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	// Make a relation for the access scope.
	endpoint1, err := s.application.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	application2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "logging",
		}),
	})
	endpoint2, err := application2.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	rel := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint1, endpoint2},
	})

	unit := s.Factory.MakeUnit(c, nil)
	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       rel.Tag(),
		Subject:     unit.Tag(),
		Role:        secrets.RoleView,
	})
	c.Assert(err, jc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, unit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleView)

	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	access, err = s.State.SecretAccess(uri, unit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleNone)
}

func (s *UnitSuite) TestDestroyAlsoDeletesOwnedSecrets(c *gc.C) {
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   s.unit.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	_, err = store.GetSecret(uri)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	// Create again, no label clash.
	s.unit, err = s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	cp.Owner = s.unit.Tag()
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestDestroyAlsoDeletesConsumerInfo(c *gc.C) {
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	_, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   names.NewUnitTag("mysql/0"),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SaveSecretConsumer(uri, s.unit.UnitTag(), &secrets.SecretConsumerMetadata{CurrentRevision: 666})
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.GetSecretConsumer(uri, s.unit.Tag())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *UnitSuite) TestSetClearResolvedWhenNotAlive(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Resolved(), gc.Equals, state.ResolvedNoHooks)
	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.ErrorMatches, deadErr)
	err = s.unit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestSubordinateChangeInPrincipal(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	for i := 0; i < 2; i++ {
		// Note: subordinate units can only be created as a side effect of a
		// principal entering scope; and a given principal can only have a
		// single subordinate unit of each application.
		name := "logging" + strconv.Itoa(i)
		s.AddTestingApplication(c, name, subCharm)
		eps, err := s.State.InferEndpoints(name, "wordpress")
		c.Assert(err, jc.ErrorIsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, jc.ErrorIsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, jc.ErrorIsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	err := s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	subordinates := s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging0/0", "logging1/0"})

	su1, err := s.State.Unit("logging1/0")
	c.Assert(err, jc.ErrorIsNil)
	err = su1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = su1.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	subordinates = s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging0/0"})
}

func (s *UnitSuite) TestDeathWithSubordinates(c *gc.C) {
	// Check that units can become dead when they've never had subordinates.
	u, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Create a new unit and add a subordinate.
	u, err = s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check the unit cannot become Dead, but can become Dying...
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, stateerrors.ErrUnitHasSubordinates)
	err = u.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	// ...and that it still can't become Dead now it's Dying.
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, stateerrors.ErrUnitHasSubordinates)

	// Make the subordinate Dead and check the principal still cannot be removed.
	sub, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	err = sub.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, gc.Equals, stateerrors.ErrUnitHasSubordinates)

	// remove the subordinate and check the principal can finally become Dead.
	err = sub.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestPrincipalName(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingApplication(c, "logging", subCharm)
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	subordinates := s.unit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging/0"})

	su, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)
	principal, valid := su.PrincipalName()
	c.Assert(valid, jc.IsTrue)
	c.Assert(principal, gc.Equals, s.unit.Name())

	// Calling PrincipalName on a principal unit yields "", false.
	principal, valid = s.unit.PrincipalName()
	c.Assert(valid, jc.IsFalse)
	c.Assert(principal, gc.Equals, "")
}

func (s *UnitSuite) TestConstraintsDefaultArchNotRelevant(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "app",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		}},
	})
	err := app.SetConstraints(constraints.MustParse("instance-type=big"))
	c.Assert(err, jc.ErrorIsNil)
	unit0, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	cons, err := unit0.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), gc.Equals, "instance-type=big")

	app = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "app2",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		}},
		Constraints: constraints.MustParse("arch=s390x"),
	})
	unit1, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	cons, err = unit1.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), gc.Equals, "arch=s390x")
}

func (s *UnitSuite) TestConstraintsDefaultArch(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "app",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		}},
	})
	err := app.SetConstraints(constraints.MustParse("mem=4G"))
	c.Assert(err, jc.ErrorIsNil)
	unit0, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	cons, err := unit0.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), gc.Equals, "arch=arm64 mem=4096M")
}

func (s *UnitSuite) TestRelations(c *gc.C) {
	wordpress0 := s.unit
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	assertEquals := func(actual, expect []*state.Relation) {
		c.Assert(actual, gc.HasLen, len(expect))
		for i, a := range actual {
			c.Assert(a.Id(), gc.Equals, expect[i].Id())
		}
	}
	assertRelationsJoined := func(unit *state.Unit, expect ...*state.Relation) {
		actual, err := unit.RelationsJoined()
		c.Assert(err, jc.ErrorIsNil)
		assertEquals(actual, expect)
	}
	assertRelationsInScope := func(unit *state.Unit, expect ...*state.Relation) {
		actual, err := unit.RelationsInScope()
		c.Assert(err, jc.ErrorIsNil)
		assertEquals(actual, expect)
	}
	assertRelations := func(unit *state.Unit, expect ...*state.Relation) {
		assertRelationsInScope(unit, expect...)
		assertRelationsJoined(unit, expect...)
	}
	assertRelations(wordpress0)
	assertRelations(mysql0)

	mysql0ru, err := rel.Unit(mysql0)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertRelations(wordpress0)
	assertRelations(mysql0, rel)

	wordpress0ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress0ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertRelations(wordpress0, rel)
	assertRelations(mysql0, rel)

	err = mysql0ru.PrepareLeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertRelations(wordpress0, rel)
	assertRelationsInScope(mysql0, rel)
	assertRelationsJoined(mysql0)
}

func (s *UnitSuite) TestRemove(c *gc.C) {
	err := s.unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, gc.ErrorMatches, `cannot remove unit "wordpress/0": unit is not dead`)
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	units, err := s.application.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
	err = s.unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitSuite) TestRemoveUnassignsFromBranch(c *gc.C) {
	// Add unit to a branch
	c.Assert(s.Model.AddBranch("apple", "testuser"), jc.ErrorIsNil)
	branch, err := s.Model.Branch("apple")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(branch.AssignUnit(s.unit.Name()), jc.ErrorIsNil)
	c.Assert(branch.Refresh(), jc.ErrorIsNil)
	c.Assert(branch.AssignedUnits(), gc.DeepEquals, map[string][]string{
		s.application.Name(): {s.unit.Name()},
	})

	// remove the unit
	c.Assert(s.unit.EnsureDead(), jc.ErrorIsNil)
	c.Assert(s.unit.Remove(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)

	// verify branch no longer tracks unit
	c.Assert(branch.Refresh(), jc.ErrorIsNil)
	c.Assert(branch.AssignedUnits(), gc.DeepEquals, map[string][]string{
		s.application.Name(): {},
	})
}

func (s *UnitSuite) TestRemovePathological(c *gc.C) {
	// Add a relation between wordpress and mysql...
	wordpress := s.application
	wordpress0 := s.unit
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// The relation holds a reference to wordpress, but that can't keep
	// wordpress from being removed -- because the relation will be removed
	// if we destroy wordpress.
	// However, if a unit of the *other* application joins the relation, that
	// will add an additional reference and prevent the relation -- and
	// thus wordpress itself -- from being removed when its last unit is.
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysql0ru, err := rel.Unit(mysql0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysql0ru.EnterScope(nil), jc.ErrorIsNil)

	// Destroy wordpress, and remove its last unit.
	c.Assert(wordpress.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	c.Assert(wordpress0.EnsureDead(), jc.ErrorIsNil)
	c.Assert(wordpress0.Remove(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)

	// Check this didn't kill the application or relation yet...
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)
	c.Assert(rel.Refresh(), jc.ErrorIsNil)

	// ...but when the unit on the other side departs the relation, the
	// relation and the other application are cleaned up.
	c.Assert(mysql0ru.LeaveScope(), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{}), jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIs, errors.NotFound)
	c.Assert(rel.Refresh(), jc.ErrorIs, errors.NotFound)
}

func (s *UnitSuite) TestRemovePathologicalWithBuggyUniter(c *gc.C) {
	// Add a relation between wordpress and mysql...
	wordpress := s.application
	wordpress0 := s.unit
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// The relation holds a reference to wordpress, but that can't keep
	// wordpress from being removed -- because the relation will be removed
	// if we destroy wordpress.
	// However, if a unit of the *other* application joins the relation, that
	// will add an additional reference and prevent the relation -- and
	// thus wordpress itself -- from being removed when its last unit is.
	mysql0, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysql0ru, err := rel.Unit(mysql0)
	c.Assert(err, jc.ErrorIsNil)
	err = mysql0ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy wordpress, and remove its last unit.
	c.Assert(wordpress.Destroy(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	c.Assert(wordpress0.EnsureDead(), jc.ErrorIsNil)
	c.Assert(wordpress0.Remove(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)

	// Check this didn't kill the application or relation yet...
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)
	c.Assert(rel.Refresh(), jc.ErrorIsNil)

	// ...and that when the malfunctioning unit agent on the other side
	// sets itself to dead *without* departing the relation, the unit's
	// removal causes the relation and the other application to be cleaned up.
	c.Assert(mysql0.EnsureDead(), jc.ErrorIsNil)
	c.Assert(mysql0.Remove(state.NewObjectStore(c, s.State.ModelUUID())), jc.ErrorIsNil)
	c.Assert(s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{}), jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIs, errors.NotFound)
	c.Assert(rel.Refresh(), jc.ErrorIs, errors.NotFound)
}

func (s *UnitSuite) TestWatchSubordinates(c *gc.C) {
	// TODO(mjs) - ModelUUID - test with multiple models with
	// identically named units and ensure there's no leakage.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.unit.WatchSubordinateUnits()
	defer workertest.CleanKill(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a couple of subordinates, check change.
	subCharm := s.AddTestingCharm(c, "logging")
	var subUnits []*state.Unit
	for i := 0; i < 2; i++ {
		// Note: subordinate units can only be created as a side effect of a
		// principal entering scope; and a given principal can only have a
		// single subordinate unit of each application.
		name := "logging" + strconv.Itoa(i)
		subApp := s.AddTestingApplication(c, name, subCharm)
		eps, err := s.State.InferEndpoints(name, "wordpress")
		c.Assert(err, jc.ErrorIsNil)
		rel, err := s.State.AddRelation(eps...)
		c.Assert(err, jc.ErrorIsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, jc.ErrorIsNil)
		err = ru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
		units, err := subApp.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(units, gc.HasLen, 1)
		subUnits = append(subUnits, units[0])
	}
	// In order to ensure that both the subordinate creation events
	// have all been processed, wait for the model to be idle.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	wc.AssertChange(subUnits[0].Name(), subUnits[1].Name())
	wc.AssertNoChange()

	// Set one to Dying, check change.
	err := subUnits[0].Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(subUnits[0].Name())
	wc.AssertNoChange()

	// Set both to Dead, and remove one; check change.
	err = subUnits[0].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subUnits[1].EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subUnits[1].Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	// In order to ensure that both the dead and remove operations
	// have been processed, we need to wait until the model is idle.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	wc.AssertChange(subUnits[0].Name(), subUnits[1].Name())
	wc.AssertNoChange()

	// Stop watcher, check closed.
	workertest.CleanKill(c, w)
	wc.AssertClosed()

	// Start a new watch, check Dead unit is reported.
	w = s.unit.WatchSubordinateUnits()
	defer workertest.CleanKill(c, w)
	wc = testing.NewStringsWatcherC(c, w)
	wc.AssertChange(subUnits[0].Name())
	wc.AssertNoChange()

	// Remove the leftover, check no change.
	err = subUnits[0].Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *UnitSuite) TestWatchUnits(c *gc.C) {
	loggo.GetLogger("juju.state.pool.txnwatcher").SetLogLevel(loggo.TRACE)
	loggo.GetLogger("juju.state.watcher").SetLogLevel(loggo.TRACE)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.State.WatchUnits()
	defer workertest.CleanKill(c, w)

	// Initial event for unit created in test setup.
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("wordpress/0")

	u, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(u.Name())
	err = u.AssignToNewMachine(defaultInstancePrechecker)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(u.Name())

	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(u.Name())

	// Stop, check closed.
	workertest.CleanKill(c, w)
	wc.AssertClosed()
}

func (s *UnitSuite) TestWatchUnit(c *gc.C) {
	loggo.GetLogger("juju.state.pool.txnwatcher").SetLogLevel(loggo.TRACE)
	loggo.GetLogger("juju.state.watcher").SetLogLevel(loggo.TRACE)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.unit.Watch()
	defer workertest.CleanKill(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	s.setAssignedMachineAddresses(c, unit)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = unit.SetPassword("arble-farble-dying-yarble")
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	preventUnitDestroyRemove(c, unit)
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	workertest.CleanKill(c, w)
	wc.AssertClosed()

	// Remove unit, start new watch, check single event.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w = s.unit.Watch()
	defer workertest.CleanKill(c, w)
	testing.NewNotifyWatcherC(c, w).AssertOneChange()
}

func (s *UnitSuite) TestUnitAgentTools(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testAgentTools(c, state.NewObjectStore(c, s.State.ModelUUID()), s.unit, `unit "wordpress/0"`)
}

func (s *UnitSuite) TestValidActionsAndSpecs(c *gc.C) {
	basicActions := `
snapshot:
  params:
    outfile:
      type: string
      default: "abcd"
`[1:]

	wordpress := s.AddTestingApplication(c, "wordpress-actions", s.AddActionsCharm(c, "wordpress", basicActions, 1))
	unit1, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	specs, err := unit1.ActionSpecs()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(specs, jc.DeepEquals, state.ActionSpecsByName{
		"snapshot": charm.ActionSpec{
			Description: "No description",
			Params: map[string]interface{}{
				"type":                 "object",
				"title":                "snapshot",
				"description":          "No description",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"type":    "string",
						"default": "abcd",
					},
				},
			},
		},
	})

	var tests = []struct {
		actionName      string
		errString       string
		givenPayload    map[string]interface{}
		expectedPayload map[string]interface{}
	}{
		{
			actionName:      "snapshot",
			expectedPayload: map[string]interface{}{"outfile": "abcd"},
		},
		{
			actionName: "juju-exec",
			errString:  `validation failed: \(root\) : "command" property is missing and required, given \{\}; \(root\) : "timeout" property is missing and required, given \{\}`,
		},
		{
			actionName:   "juju-exec",
			givenPayload: map[string]interface{}{"command": "allyourbasearebelongtous"},
			errString:    `validation failed: \(root\) : "timeout" property is missing and required, given \{"command":"allyourbasearebelongtous"\}`,
		},
		{
			actionName:   "juju-exec",
			givenPayload: map[string]interface{}{"timeout": 5 * time.Second},
			// Note: in Go 1.8 the representation of large numbers in JSON changed
			// to use integer rather than exponential notation, hence the pattern.
			errString: `validation failed: \(root\) : "command" property is missing and required, given \{"timeout":5.*\}`,
		},
		{
			actionName:      "juju-exec",
			givenPayload:    map[string]interface{}{"command": "allyourbasearebelongtous", "timeout": 5.0},
			expectedPayload: map[string]interface{}{"command": "allyourbasearebelongtous", "timeout": 5.0},
		},
		{
			actionName: "baiku",
			errString:  `action "baiku" not defined on unit "wordpress-actions/0"`,
		},
	}

	for i, t := range tests {
		c.Logf("running test %d", i)
		operationID, err := s.Model.EnqueueOperation("a test", 1)
		c.Assert(err, jc.ErrorIsNil)
		action, err := s.Model.AddAction(unit1, operationID, t.actionName, t.givenPayload, nil, nil)
		if t.errString != "" {
			c.Assert(err, gc.ErrorMatches, t.errString)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(action.Parameters(), jc.DeepEquals, t.expectedPayload)
			c.Assert(state.ActionOperationId(action), gc.Equals, operationID)
		}
	}
}

func (s *UnitSuite) TestAddActionWithError(c *gc.C) {
	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.Model.AddAction(s.unit, operationID, "benchmark", nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, `action "benchmark" not defined on unit "wordpress/0"`)
	op, err := s.Model.Operation(operationID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.Status(), gc.Equals, state.ActionError)
}

func (s *UnitSuite) TestUnitActionsFindsRightActions(c *gc.C) {
	// An actions.yaml which permits actions by the following names
	basicActions := `
action-a-a:
action-a-b:
action-a-c:
action-b-a:
action-b-b:
`[1:]

	// Add simple application and two units
	dummy := s.AddTestingApplication(c, "dummy", s.AddActionsCharm(c, "dummy", basicActions, 1))

	unit1, err := dummy.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	unit2, err := dummy.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Add 3 actions to first unit, and 2 to the second unit
	operationID, err := s.Model.EnqueueOperation("a test", 5)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.Model.AddAction(unit1, operationID, "action-a-a", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.Model.AddAction(unit1, operationID, "action-a-b", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.Model.AddAction(unit1, operationID, "action-a-c", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.Model.AddAction(unit2, operationID, "action-b-a", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.Model.AddAction(unit2, operationID, "action-b-b", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Verify that calling Actions on unit1 returns only
	// the three actions added to unit1
	actions1, err := unit1.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions1), gc.Equals, 3)
	for _, action := range actions1 {
		c.Assert(action.Name(), gc.Matches, "^action-a-.")
	}

	// Verify that calling Actions on unit2 returns only
	// the two actions added to unit2
	actions2, err := unit2.Actions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions2), gc.Equals, 2)
	for _, action := range actions2 {
		c.Assert(action.Name(), gc.Matches, "^action-b-.")
	}
}

func (s *UnitSuite) TestWorkloadVersion(c *gc.C) {
	ch := state.AddTestingCharm(c, s.State, "dummy")
	app := state.AddTestingApplication(c, s.State, s.objectStore, "alexandrite", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	version, err := unit.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, "")

	err = unit.SetWorkloadVersion("3.combined", status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	version, err = unit.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, "3.combined")

	regotUnit, err := s.State.Unit("alexandrite/0")
	c.Assert(err, jc.ErrorIsNil)
	version, err = regotUnit.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, "3.combined")
}

func (s *UnitSuite) TestDestroyWithForceWorksOnDyingUnit(c *gc.C) {
	// Ensure that a cleanup is scheduled if we force destroy a unit
	// that's already dying.
	ch := state.AddTestingCharm(c, s.State, "dummy")
	app := state.AddTestingApplication(c, s.State, s.objectStore, "alexandrite", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Assign the unit to a machine and set the agent status so
	// removal can't be short-circuited.
	err = s.State.AssignUnit(defaultInstancePrechecker, unit, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetAgentStatus(status.StatusInfo{
		Status: status.Idle,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(unit.Life(), gc.Equals, state.Dying)

	needsCleanup, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsCleanup, gc.Equals, true)

	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)
	needsCleanup, err = s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsCleanup, gc.Equals, false)

	// Force-destroying the unit should schedule a cleanup so we get a
	// chance for the fallback force-cleanup to run.
	removed, opErrs, err := unit.DestroyWithForce(state.NewObjectStore(c, s.State.ModelUUID()), true, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opErrs, gc.IsNil)
	c.Assert(removed, jc.IsFalse)

	// We scheduled the dying unit cleanup again even though the unit was dying already.
	needsCleanup, err = s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needsCleanup, gc.Equals, true)
}

func (s *UnitSuite) TestDestroyWithForceReportsRemoved(c *gc.C) {
	// Ensure that a cleanup is scheduled if we force destroy a unit
	// that's already dying.
	ch := state.AddTestingCharm(c, s.State, "dummy")
	app := state.AddTestingApplication(c, s.State, s.objectStore, "alexandrite", ch)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	removed, opErrs, err := unit.DestroyWithForce(state.NewObjectStore(c, s.State.ModelUUID()), true, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(opErrs, gc.IsNil)
	c.Assert(removed, jc.IsTrue)
}

func unitMachine(c *gc.C, st *state.State, u *state.Unit) *state.Machine {
	machineId, err := u.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := st.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

type CAASUnitSuite struct {
	ConnSuite
	application *state.Application
	operatorApp *state.Application
}

var _ = gc.Suite(&CAASUnitSuite{})

func (s *CAASUnitSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	st := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { st.Close() })

	var err error
	s.Model, err = st.Model()
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})
	s.application = f.MakeApplication(c, &factory.ApplicationParams{
		Name: "gitlab", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{OS: "ubuntu", Channel: "20.04/stable"}},
	})

	basicActions := `
snapshot:
  params:
    outfile:
      type: string
      default: "abcd"
`[1:]
	ch = state.AddCustomCharm(c, st, "elastic-operator", "actions.yaml", basicActions, "focal", 1)
	s.operatorApp = f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})
}

func (s *CAASUnitSuite) TestShortCircuitDestroyUnit(c *gc.C) {
	// A unit that has not been allocated is removed directly.
	unit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Base(), jc.DeepEquals, state.Base{OS: "ubuntu", Channel: "20.04/stable"})
	c.Assert(unit.ShouldBeAssigned(), jc.IsFalse)

	// A unit that has not set any status is removed directly.
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Life(), gc.Equals, state.Dying)
	assertRemoved(c, s.State, unit)
}

func (s *CAASUnitSuite) TestCannotShortCircuitDestroyAllocatedUnit(c *gc.C) {
	// This test is similar to TestShortCircuitDestroyUnit but
	// the unit has been allocated and a pod created.
	unit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	unitState := state.NewUnitState()
	unitState.SetUniterState("error")
	err = unit.SetState(unitState, state.UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Life(), gc.Equals, state.Dying)
	assertLife(c, unit, state.Dying)
}

func (s *CAASUnitSuite) TestUpdateCAASUnitProviderId(c *gc.C) {
	existingUnit, err := s.application.AddUnit(state.AddUnitParams{
		ProviderId: strPtr("unit-uuid"),
		Address:    strPtr("192.168.1.1"),
		Ports:      &[]string{"80"},
	})
	c.Assert(err, jc.ErrorIsNil)
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		existingUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("another-uuid"),
		})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
	info, err := existingUnit.ContainerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Unit(), gc.Equals, existingUnit.Name())
	c.Assert(info.ProviderId(), gc.Equals, "another-uuid")
	addr := network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeMachineLocal))
	c.Assert(info.Address(), gc.DeepEquals, &addr)
	c.Assert(info.Ports(), jc.DeepEquals, []string{"80"})
}

func (s *CAASUnitSuite) TestAddCAASUnitProviderId(c *gc.C) {
	existingUnit, err := s.application.AddUnit(state.AddUnitParams{
		Address: strPtr("192.168.1.1"),
		Ports:   &[]string{"80"},
	})
	c.Assert(err, jc.ErrorIsNil)
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		existingUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("another-uuid"),
		})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
	info, err := existingUnit.ContainerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Unit(), gc.Equals, existingUnit.Name())
	c.Assert(info.ProviderId(), gc.Equals, "another-uuid")
	c.Check(info.Address(), gc.NotNil)
	c.Check(*info.Address(), jc.DeepEquals,
		network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeMachineLocal)))
	c.Assert(info.Ports(), jc.DeepEquals, []string{"80"})
}

func (s *CAASUnitSuite) TestUpdateCAASUnitAddress(c *gc.C) {
	existingUnit, err := s.application.AddUnit(state.AddUnitParams{
		ProviderId: strPtr("unit-uuid"),
		Address:    strPtr("192.168.1.1"),
		Ports:      &[]string{"80"},
	})
	c.Assert(err, jc.ErrorIsNil)
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		existingUnit.UpdateOperation(state.UnitUpdateProperties{
			Address: strPtr("192.168.1.2"),
		})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
	info, err := existingUnit.ContainerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Unit(), gc.Equals, existingUnit.Name())
	c.Assert(info.ProviderId(), gc.Equals, "unit-uuid")
	addr := network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeMachineLocal))
	c.Assert(info.Address(), jc.DeepEquals, &addr)
	c.Assert(info.Ports(), jc.DeepEquals, []string{"80"})
}

func (s *CAASUnitSuite) TestUpdateCAASUnitPorts(c *gc.C) {
	existingUnit, err := s.application.AddUnit(state.AddUnitParams{
		ProviderId: strPtr("unit-uuid"),
		Address:    strPtr("192.168.1.1"),
		Ports:      &[]string{"80"},
	})
	c.Assert(err, jc.ErrorIsNil)
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		existingUnit.UpdateOperation(state.UnitUpdateProperties{
			Ports: &[]string{"443"},
		})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
	info, err := existingUnit.ContainerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Unit(), gc.Equals, existingUnit.Name())
	c.Assert(info.ProviderId(), gc.Equals, "unit-uuid")
	addr := network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeMachineLocal))
	c.Assert(info.Address(), jc.DeepEquals, &addr)
	c.Assert(info.Ports(), jc.DeepEquals, []string{"443"})
}

func (s *CAASUnitSuite) TestRemoveUnitDeletesContainerInfo(c *gc.C) {
	existingUnit, err := s.application.AddUnit(state.AddUnitParams{
		ProviderId: strPtr("unit-uuid"),
		Address:    strPtr("192.168.1.1"),
		Ports:      &[]string{"80"},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = existingUnit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	_, err = existingUnit.ContainerInfo()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *CAASUnitSuite) TestPrivateAddress(c *gc.C) {
	existingUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.UpdateCloudService("", network.SpaceAddresses{
		network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("54.32.1.2", network.WithScope(network.ScopePublic)),
	})
	c.Assert(err, jc.ErrorIsNil)

	addr, err := existingUnit.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeCloudLocal)))
}

func (s *CAASUnitSuite) TestPublicAddress(c *gc.C) {
	existingUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.UpdateCloudService("", []network.SpaceAddress{
		network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("54.32.1.2", network.WithScope(network.ScopePublic)),
	})
	c.Assert(err, jc.ErrorIsNil)

	addr, err := existingUnit.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, network.NewSpaceAddress("54.32.1.2", network.WithScope(network.ScopePublic)))
}

func (s *CAASUnitSuite) TestAllAddresses(c *gc.C) {
	existingUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.UpdateCloudService("", []network.SpaceAddress{
		network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("54.32.1.2", network.WithScope(network.ScopePublic)),
	})
	c.Assert(err, jc.ErrorIsNil)

	var updateUnits state.UpdateUnitsOperation
	local := "10.0.0.1"
	updateUnits.Updates = []*state.UpdateUnitOperation{existingUnit.UpdateOperation(state.UnitUpdateProperties{
		Address: &local,
		Ports:   &[]string{"443"},
	})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := existingUnit.AllAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addrs, jc.DeepEquals, network.SpaceAddresses{
		network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("54.32.1.2", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("10.0.0.1", network.WithScope(network.ScopeMachineLocal)),
	})
}

func (s *CAASUnitSuite) TestWatchContainerAddresses(c *gc.C) {
	unit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := unit.WatchContainerAddresses()
	defer w.Stop()
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// Change the container port: not reported.
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{unit.UpdateOperation(state.UnitUpdateProperties{
		Ports: &[]string{"443"},
	})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Set container addresses: reported.
	addr := "10.0.0.1"
	updateUnits.Updates = []*state.UpdateUnitOperation{unit.UpdateOperation(state.UnitUpdateProperties{
		Address: &addr,
	})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Set different machine addresses: reported.
	addr = "10.0.0.2"
	updateUnits.Updates = []*state.UpdateUnitOperation{unit.UpdateOperation(state.UnitUpdateProperties{
		Address: &addr,
	})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Ensure the following operation to set the unit as Dying
	// is not short circuited to remove the unit.
	unitState := state.NewUnitState()
	unitState.SetUniterState("idle")
	err = unit.SetState(unitState, state.UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)
	// Make it Dying: not reported.
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	// Double check the unit is dying and not removed.
	err = unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Life(), gc.Equals, state.Dying)

	// Make it Dead: not reported.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove it: watcher eventually closed and Err
	// returns an IsNotFound error.
	err = unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("watcher not closed")
	}
	c.Assert(w.Err(), jc.ErrorIs, errors.NotFound)
}

func (s *CAASUnitSuite) TestWatchServiceAddressesHash(c *gc.C) {
	unit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	w := s.application.WatchServiceAddressesHash()
	defer w.Stop()
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange("")

	// Set container addresses: not reported.
	var updateUnits state.UpdateUnitsOperation
	addr := "10.0.0.1"
	updateUnits.Updates = []*state.UpdateUnitOperation{unit.UpdateOperation(state.UnitUpdateProperties{
		Address: &addr,
	})}
	err = s.application.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Set service addresses: reported.
	err = s.application.UpdateCloudService("1", network.SpaceAddresses{network.NewSpaceAddress("10.0.0.2")})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("7fcdfefa54c49ed9dc8132b4a28491a02ec35b03c20b2d4cc95469fead847ff8")

	// Set different container addresses: reported.
	err = s.application.UpdateCloudService("1", network.SpaceAddresses{network.NewSpaceAddress("10.0.0.3")})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("800892c5473f38623ef4856303b3458cfa81d0da803f228db69910949a13f458")

	// Ensure the following operation to set the unit as Dying
	// is not short circuited to remove the unit.
	unitState := state.NewUnitState()
	unitState.SetUniterState("idle")
	err = unit.SetState(unitState, state.UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)
	// Make it Dying: not reported.
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	// Double check the unit is dying and not removed.
	err = unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Life(), gc.Equals, state.Dying)

	// Make it Dead: not reported.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Remove it: watcher eventually closed and Err
	// returns an IsNotFound error.
	err = unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	// App removal requires cluster resources to be cleared.
	err = s.application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	assertCleanupCount(c, s.Model.State(), 2)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("watcher not closed")
	}
	c.Assert(w.Err(), jc.ErrorIs, errors.NotFound)
}

func (s *CAASUnitSuite) TestOperatorAddAction(c *gc.C) {
	unit, err := s.operatorApp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	operationID, err := s.Model.EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.Model.AddAction(unit, operationID, "snapshot", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(action.Parameters(), jc.DeepEquals, map[string]interface{}{
		"outfile": "abcd",
	})
}
