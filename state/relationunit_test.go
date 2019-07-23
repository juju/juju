// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type RUs []*state.RelationUnit

type RelationUnitSuite struct {
	ConnSuite
}

var _ = gc.Suite(&RelationUnitSuite{})

func assertInScope(c *gc.C, ru *state.RelationUnit) {
	ok, err := ru.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func assertNotInScope(c *gc.C, ru *state.RelationUnit) {
	assertNotJoined(c, ru)
	ok, err := ru.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsFalse)
}

func assertJoined(c *gc.C, ru *state.RelationUnit) {
	assertInScope(c, ru)
	ok, err := ru.Joined()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func assertNotJoined(c *gc.C, ru *state.RelationUnit) {
	ok, err := ru.Joined()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsFalse)
}

func (s *RelationUnitSuite) TestReadSettingsErrors(c *gc.C) {
	riak := s.AddTestingApplication(c, "riak", s.AddTestingCharm(c, "riak"))
	u0, err := riak.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.EndpointsRelation(riakEP)
	c.Assert(err, jc.ErrorIsNil)
	ru0, err := rel.Unit(u0)
	c.Assert(err, jc.ErrorIsNil)

	_, err = ru0.ReadSettings("nonsense")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "nonsense" in relation "riak:ring": "nonsense" is not a valid unit name`)
	_, err = ru0.ReadSettings("unknown/0")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "unknown/0" in relation "riak:ring": application "unknown" is not a member of "riak:ring"`)
	_, err = ru0.ReadSettings("riak/pressure")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/pressure" in relation "riak:ring": "riak/pressure" is not a valid unit name`)
	_, err = ru0.ReadSettings("riak/1")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/1" in relation "riak:ring": unit "riak/1": settings not found`)
}

func (s *RelationUnitSuite) TestPeerSettings(c *gc.C) {
	pr := newPeerRelation(c, s.State)
	rus := RUs{pr.ru0, pr.ru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("riak/0")
		c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": unit "riak/0": settings not found`)
	}

	// Add settings for one RU.
	assertNotInScope(c, pr.ru0)
	err := pr.ru0.EnterScope(map[string]interface{}{"gene": "kelly"})
	c.Assert(err, jc.ErrorIsNil)
	node, err := pr.ru0.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node.Set("meme", "socially-awkward-penguin")
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	normal := map[string]interface{}{
		"gene": "kelly",
		"meme": "socially-awkward-penguin",
	}

	// Check settings can be read by every RU.
	assertSettings := func(u *state.Unit, expect map[string]interface{}) {
		for _, ru := range rus {
			m, err := ru.ReadSettings(u.Name())
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(m, gc.DeepEquals, expect)
		}
	}
	assertSettings(pr.u0, normal)
	assertJoined(c, pr.ru0)

	// Check that EnterScope when scope already entered does not touch
	// settings at all.
	changed := map[string]interface{}{"foo": "bar"}
	err = pr.ru0.EnterScope(changed)
	c.Assert(err, jc.ErrorIsNil)
	assertSettings(pr.u0, normal)
	assertJoined(c, pr.ru0)

	// Leave scope, check settings are still as accessible as before.
	err = pr.ru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertSettings(pr.u0, normal)
	assertNotInScope(c, pr.ru0)

	// Re-enter scope with changed settings, and check they completely overwrite
	// the old ones.
	err = pr.ru0.EnterScope(changed)
	c.Assert(err, jc.ErrorIsNil)
	assertSettings(pr.u0, changed)
	assertJoined(c, pr.ru0)

	// Leave and re-enter with nil nettings, and check they overwrite to become
	// an empty map.
	err = pr.ru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, pr.ru0)
	err = pr.ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertSettings(pr.u0, map[string]interface{}{})
	assertJoined(c, pr.ru0)

	// Check that entering scope for the first time with nil settings works correctly.
	assertNotInScope(c, pr.ru1)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertSettings(pr.u1, map[string]interface{}{})
	assertJoined(c, pr.ru1)
}

func (s *RelationUnitSuite) TestRemoteUnitErrors(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql1",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	_, err = rel.RemoteUnit("mysql")
	c.Assert(err, gc.ErrorMatches, `"mysql" is not a valid unit name`)

	_, err = rel.RemoteUnit("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `remote application "wordpress" not found`)

	_, err = rel.RemoteUnit("mysql1/0")
	c.Assert(err, gc.ErrorMatches, `application "mysql1" is not a member of "wordpress:db mysql:server"`)
}

func (s *RelationUnitSuite) TestAllRemoteUnits(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "another",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	ru1 := addRemoteRU(c, rel, "mysql/0")
	err = ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	ru2 := addRemoteRU(c, rel, "mysql/1")
	err = ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = rel.AllRemoteUnits("wordpress")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = rel.AllRemoteUnits("another")
	c.Assert(err, gc.ErrorMatches, `application "another" is not a member of "wordpress:db mysql:server"`)
	all, err := rel.AllRemoteUnits("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 2)
	c.Assert(all, jc.SameContents, []*state.RelationUnit{ru1, ru2})
}

func (s *RelationUnitSuite) TestProReqSettings(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	s.testProReqSettings(c, prr.pru0, prr.pru1, prr.rru0, prr.rru1)
}

func (s *RelationUnitSuite) TestRemoteProReqSettings(c *gc.C) {
	prr := newRemoteProReqRelation(c, &s.ConnSuite)
	s.testProReqSettings(c, prr.pru0, prr.pru1, prr.rru0, prr.rru1)
}

func (s *RelationUnitSuite) testProReqSettings(c *gc.C, pru0, pru1, rru0, rru1 *state.RelationUnit) {
	rus := RUs{pru0, pru1, rru0, rru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("mysql/0")
		c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "mysql/0" in relation "wordpress:db mysql:server": unit "mysql/0": settings not found`)
	}

	// Add settings for one RU.
	assertNotInScope(c, pru0)
	err := pru0.EnterScope(map[string]interface{}{"gene": "simmons"})
	c.Assert(err, jc.ErrorIsNil)
	node, err := pru0.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node.Set("meme", "foul-bachelor-frog")
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pru0)

	// Check settings can be read by every RU.
	for _, ru := range rus {
		m, err := ru.ReadSettings("mysql/0")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m["gene"], gc.Equals, "simmons")
		c.Assert(m["meme"], gc.Equals, "foul-bachelor-frog")
	}
}

func (s *RelationUnitSuite) TestContainerSettings(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	rus := RUs{prr.pru0, prr.pru1, prr.rru0, prr.rru1}

	// Check missing settings cannot be read by any RU.
	for _, ru := range rus {
		_, err := ru.ReadSettings("logging/0")
		c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "logging/0" in relation "logging:info mysql:juju-info": unit "logging/0": settings not found`)
	}

	// Add settings for one RU.
	assertNotInScope(c, prr.pru0)
	err := prr.pru0.EnterScope(map[string]interface{}{"gene": "hackman"})
	c.Assert(err, jc.ErrorIsNil)
	node, err := prr.pru0.Settings()
	c.Assert(err, jc.ErrorIsNil)
	node.Set("meme", "foul-bachelor-frog")
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, prr.pru0)

	// Check settings can be read by RUs in the same container.
	rus0 := RUs{prr.pru0, prr.rru0}
	for _, ru := range rus0 {
		m, err := ru.ReadSettings("mysql/0")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m["gene"], gc.Equals, "hackman")
		c.Assert(m["meme"], gc.Equals, "foul-bachelor-frog")
	}

	// Check settings are still inaccessible to RUs outside that container
	rus1 := RUs{prr.pru1, prr.rru1}
	for _, ru := range rus1 {
		_, err := ru.ReadSettings("mysql/0")
		c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "mysql/0" in relation "logging:info mysql:juju-info": unit "mysql/0": settings not found`)
	}
}

func (s *RelationUnitSuite) TestContainerCreateSubordinate(c *gc.C) {
	papp := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	rapp := s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	punit, err := papp.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	pru, err := rel.Unit(punit)
	c.Assert(err, jc.ErrorIsNil)

	// Check that no units of the subordinate application exist.
	assertSubCount := func(expect int) []*state.Unit {
		runits, err := rapp.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(runits, gc.HasLen, expect)
		return runits
	}
	assertSubCount(0)

	// Enter principal's scope and check a subordinate was created.
	assertNotInScope(c, pru)
	err = pru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertSubCount(1)
	assertJoined(c, pru)

	// Enter principal scope again and check no more subordinates created.
	err = pru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertSubCount(1)
	assertJoined(c, pru)

	// Leave principal scope, then re-enter, and check that still no further
	// subordinates are created.
	err = pru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, pru)
	err = pru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	runits := assertSubCount(1)
	assertJoined(c, pru)

	// Set the subordinate to Dying, and enter scope again; because the scope
	// is already entered, no error is returned.
	runit := runits[0]
	err = runit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = pru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pru)

	// Leave scope, then try to enter again with the Dying subordinate.
	err = pru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, pru)
	err = pru.EnterScope(nil)
	c.Assert(err, gc.Equals, state.ErrCannotEnterScopeYet)
	assertNotInScope(c, pru)

	// Remove the subordinate, and enter scope again; this should work, and
	// create a new subordinate.
	err = runit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = runit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	assertSubCount(0)
	assertNotInScope(c, pru)
	err = pru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertSubCount(1)
	assertJoined(c, pru)
}

func (s *RelationUnitSuite) TestDestroyRelationWithUnitsInScope(c *gc.C) {
	pr := newPeerRelation(c, s.State)
	preventPeerUnitsDestroyRemove(c, pr)
	rel := pr.ru0.Relation()

	// Enter two units, and check that Destroying the application sets the
	// relation to Dying (rather than removing it directly).
	assertNotInScope(c, pr.ru0)
	err := pr.ru0.EnterScope(map[string]interface{}{"some": "settings"})
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pr.ru0)
	assertNotInScope(c, pr.ru1)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pr.ru1)
	err = pr.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// Check that we can't add a new unit now.
	assertNotInScope(c, pr.ru2)
	err = pr.ru2.EnterScope(nil)
	c.Assert(err, gc.Equals, state.ErrCannotEnterScope)
	assertNotInScope(c, pr.ru2)

	// Check that we created no settings for the unit we failed to add.
	_, err = pr.ru0.ReadSettings("riak/2")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/2" in relation "riak:ring": unit "riak/2": settings not found`)

	// ru0 leaves the scope; check that application Destroy is still a no-op.
	assertJoined(c, pr.ru0)
	err = pr.ru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, pr.ru0)
	err = pr.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Check that unit settings for the original unit still exist, and have
	// not yet been marked for deletion.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	assertSettings := func() {
		settings, err := pr.ru1.ReadSettings("riak/0")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(settings, gc.DeepEquals, map[string]interface{}{"some": "settings"})
	}
	assertSettings()

	// The final unit leaves the scope, and cleans up after itself.
	assertJoined(c, pr.ru1)
	err = pr.ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, pr.ru1)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// The settings were not themselves actually deleted yet...
	assertSettings()

	// ...but they were scheduled for deletion.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	_, err = pr.ru1.ReadSettings("riak/0")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "riak/0" in relation "riak:ring": unit "riak/0": settings not found`)
}

func (s *RelationUnitSuite) TestAliveRelationScope(c *gc.C) {
	pr := newPeerRelation(c, s.State)
	rel := pr.ru0.Relation()

	// Two units enter...
	assertNotInScope(c, pr.ru0)
	err := pr.ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pr.ru0)
	assertNotInScope(c, pr.ru1)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pr.ru1)

	// One unit becomes Dying, then re-enters the scope; this is not an error,
	// because the state is already as requested.
	err = pr.u0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = pr.ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pr.ru0)

	// Two units leave...
	err = pr.ru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, pr.ru0)
	err = pr.ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, pr.ru1)

	// The relation scope is empty, but the relation is still alive...
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Alive)

	// ...and new units can still join it...
	assertNotInScope(c, pr.ru2)
	err = pr.ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pr.ru2)

	// ...but Dying units cannot.
	err = pr.u3.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, pr.ru3)
	err = pr.ru3.EnterScope(nil)
	c.Assert(err, gc.Equals, state.ErrCannotEnterScope)
	assertNotInScope(c, pr.ru3)
}

func (s *StateSuite) TestWatchWatchScopeDiesOnStateClose(c *gc.C) {
	testWatcherDiesWhenStateCloses(c, s.Session, s.modelTag, s.State.ControllerTag(), func(c *gc.C, st *state.State) waiter {
		pr := newPeerRelation(c, st)
		w := pr.ru0.WatchScope()
		<-w.Changes()
		return w
	})
}

func (s *RelationUnitSuite) TestPeerWatchScope(c *gc.C) {
	pr := newPeerRelation(c, s.State)

	// Test empty initial event.
	w0 := pr.ru0.WatchScope()
	defer testing.AssertStop(c, w0)
	s.assertScopeChange(c, w0, nil, nil)
	s.assertNoScopeChange(c, w0)

	// ru0 enters; check no change, but settings written.
	assertNotInScope(c, pr.ru0)
	err := pr.ru0.EnterScope(map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoScopeChange(c, w0)
	node, err := pr.ru0.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.Map(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
	assertJoined(c, pr.ru0)

	// ru1 enters; check change is observed.
	assertNotInScope(c, pr.ru1)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, w0, []string{"riak/1"}, nil)
	s.assertNoScopeChange(c, w0)
	assertJoined(c, pr.ru1)

	// ru1 enters again, check no problems and no changes.
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoScopeChange(c, w0)
	assertJoined(c, pr.ru1)

	// Stop watching; ru2 enters.
	testing.AssertStop(c, w0)
	assertNotInScope(c, pr.ru2)
	err = pr.ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pr.ru2)

	// Start watch again, check initial event.
	w0 = pr.ru0.WatchScope()
	defer testing.AssertStop(c, w0)
	s.assertScopeChange(c, w0, []string{"riak/1", "riak/2"}, nil)
	s.assertNoScopeChange(c, w0)

	// ru1 leaves; check event.
	assertJoined(c, pr.ru1)
	err = pr.ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, w0, nil, []string{"riak/1"})
	s.assertNoScopeChange(c, w0)
	assertNotInScope(c, pr.ru1)

	// ru1 leaves again; check no problems and no changes.
	err = pr.ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoScopeChange(c, w0)
	assertNotInScope(c, pr.ru1)
}

func (s *RelationUnitSuite) TestProReqWatchScope(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	s.testProReqWatchScope(c, prr.pru0, prr.pru1, prr.rru0, prr.rru1, prr.watches)
}

func (s *RelationUnitSuite) TestRemoteProReqWatchScope(c *gc.C) {
	prr := newRemoteProReqRelation(c, &s.ConnSuite)
	s.testProReqWatchScope(c, prr.pru0, prr.pru1, prr.rru0, prr.rru1, prr.watches)
}

func (s *RelationUnitSuite) testProReqWatchScope(
	c *gc.C, pru0, pru1, rru0, rru1 *state.RelationUnit,
	watches func() []*state.RelationScopeWatcher,
) {
	// Test empty initial events for all RUs.
	ws := watches()
	for _, w := range ws {
		defer testing.AssertStop(c, w)
	}
	for _, w := range ws {
		s.assertScopeChange(c, w, nil, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 enters; check detected only by req RUs.
	assertNotInScope(c, pru0)
	err := pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	rws := func() []*state.RelationScopeWatcher {
		return []*state.RelationScopeWatcher{ws[2], ws[3]}
	}
	for _, w := range rws() {
		s.assertScopeChange(c, w, []string{"mysql/0"}, nil)
	}
	s.assertNoScopeChange(c, ws...)
	assertJoined(c, pru0)

	// req0 enters; check detected only by pro RUs.
	assertNotInScope(c, rru0)
	err = rru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	pws := func() []*state.RelationScopeWatcher {
		return []*state.RelationScopeWatcher{ws[0], ws[1]}
	}
	for _, w := range pws() {
		s.assertScopeChange(c, w, []string{"wordpress/0"}, nil)
	}
	s.assertNoScopeChange(c, ws...)
	assertJoined(c, rru0)

	// Stop watches; remaining RUs enter.
	for _, w := range ws {
		testing.AssertStop(c, w)
	}
	assertNotInScope(c, pru1)
	err = pru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, pru1)
	assertNotInScope(c, rru1)
	err = rru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertJoined(c, rru0)

	// Start new watches, check initial events.
	ws = watches()
	for _, w := range ws {
		defer testing.AssertStop(c, w)
	}
	for _, w := range pws() {
		s.assertScopeChange(c, w, []string{"wordpress/0", "wordpress/1"}, nil)
	}
	for _, w := range rws() {
		s.assertScopeChange(c, w, []string{"mysql/0", "mysql/1"}, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 leaves; check detected only by req RUs.
	assertJoined(c, pru0)
	err = pru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	for _, w := range rws() {
		s.assertScopeChange(c, w, nil, []string{"mysql/0"})
	}
	s.assertNoScopeChange(c, ws...)
	assertNotInScope(c, pru0)

	// rru0 leaves; check detected only by pro RUs.
	assertJoined(c, rru0)
	err = rru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	for _, w := range pws() {
		s.assertScopeChange(c, w, nil, []string{"wordpress/0"})
	}
	s.assertNoScopeChange(c, ws...)
	assertNotInScope(c, rru0)
}

func (s *RelationUnitSuite) TestContainerWatchScope(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)

	// Test empty initial events for all RUs.
	ws := prr.watches()
	for _, w := range ws {
		defer testing.AssertStop(c, w)
	}
	for _, w := range ws {
		s.assertScopeChange(c, w, nil, nil)
	}
	s.assertNoScopeChange(c, ws...)

	// pru0 enters; check detected only by same-container req.
	assertNotInScope(c, prr.pru0)
	err := prr.pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, ws[2], []string{"mysql/0"}, nil)
	s.assertNoScopeChange(c, ws...)
	assertJoined(c, prr.pru0)

	// req1 enters; check detected only by same-container pro.
	assertNotInScope(c, prr.rru1)
	err = prr.rru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, ws[1], []string{"logging/1"}, nil)
	s.assertNoScopeChange(c, ws...)
	assertJoined(c, prr.rru1)

	// Stop watches; remaining RUs enter scope.
	for _, w := range ws {
		testing.AssertStop(c, w)
	}
	assertNotInScope(c, prr.pru1)
	err = prr.pru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	assertNotInScope(c, prr.rru0)
	err = prr.rru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Start new watches, check initial events.
	ws = prr.watches()
	for _, w := range ws {
		defer testing.AssertStop(c, w)
	}
	s.assertScopeChange(c, ws[0], []string{"logging/0"}, nil)
	s.assertScopeChange(c, ws[1], []string{"logging/1"}, nil)
	s.assertScopeChange(c, ws[2], []string{"mysql/0"}, nil)
	s.assertScopeChange(c, ws[3], []string{"mysql/1"}, nil)
	s.assertNoScopeChange(c, ws...)
	assertJoined(c, prr.pru1)
	assertJoined(c, prr.rru0)

	// pru0 leaves; check detected only by same-container req.
	assertJoined(c, prr.pru0)
	err = prr.pru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, ws[2], nil, []string{"mysql/0"})
	s.assertNoScopeChange(c, ws...)
	assertNotInScope(c, prr.pru0)

	// rru0 leaves; check detected only by same-container pro.
	assertJoined(c, prr.rru0)
	err = prr.rru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, ws[0], nil, []string{"logging/0"})
	s.assertNoScopeChange(c, ws...)
	assertNotInScope(c, prr.rru0)
}

func (s *RelationUnitSuite) TestCoalesceWatchScope(c *gc.C) {
	pr := newPeerRelation(c, s.State)

	// Test empty initial event.
	w0 := pr.ru0.WatchScope()
	defer testing.AssertStop(c, w0)
	s.assertScopeChange(c, w0, nil, nil)
	s.assertNoScopeChange(c, w0)

	// ru1 and ru2 enter; check changes observed together.
	err := pr.ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = pr.ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertScopeChange(c, w0, []string{"riak/1", "riak/2"}, nil)
	s.assertNoScopeChange(c, w0)

	// ru1 leaves and re-enters; check no change observed.
	err = pr.ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = pr.ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoScopeChange(c, w0)

	// ru1 and ru2 leave; check changes observed together.
	err = pr.ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = pr.ru2.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, w0, nil, []string{"riak/1", "riak/2"})
	s.assertNoScopeChange(c, w0)
}

func (s *RelationUnitSuite) TestPrepareLeaveScope(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	s.testPrepareLeaveScope(c, prr.rel, prr.pru0, prr.pru1, prr.rru0, prr.rru1)
}

func (s *RelationUnitSuite) TestPrepareLeaveScopeRemote(c *gc.C) {
	prr := newRemoteProReqRelation(c, &s.ConnSuite)
	s.testPrepareLeaveScope(c, prr.rel, prr.pru0, prr.pru1, prr.rru0, prr.rru1)
}

func (s *RelationUnitSuite) testPrepareLeaveScope(c *gc.C, rel *state.Relation, pru0, pru1, rru0, rru1 *state.RelationUnit) {
	// Test empty initial event.
	w0 := pru0.WatchScope()
	defer testing.AssertStop(c, w0)
	s.assertScopeChange(c, w0, nil, nil)
	s.assertNoScopeChange(c, w0)

	// rru0 and rru1 enter; check changes.
	err := rru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = rru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, w0, []string{"wordpress/0", "wordpress/1"}, nil)
	s.assertNoScopeChange(c, w0)

	// rru0 notifies that it will leave soon; it's reported as departed by the
	// watcher, but InScope remains accurate.
	err = rru0.PrepareLeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, w0, nil, []string{"wordpress/0"})
	s.assertNoScopeChange(c, w0)
	assertInScope(c, rru0)
	assertNotJoined(c, rru0)

	// rru1 leaves, and the relation is destroyed; it's not removed, because
	// rru0 keeps it alive until it really leaves scope.
	err = rru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertScopeChange(c, w0, nil, []string{"wordpress/1"})
	s.assertNoScopeChange(c, w0)
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	// rru0 really leaves; the relation is cleaned up.
	err = rru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoScopeChange(c, w0)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *RelationUnitSuite) assertScopeChange(c *gc.C, w *state.RelationScopeWatcher, entered, left []string) {
	s.State.StartSync()
	select {
	case ch, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		sort.Strings(entered)
		sort.Strings(ch.Entered)
		c.Assert(ch.Entered, gc.DeepEquals, entered)
		sort.Strings(left)
		sort.Strings(ch.Left)
		c.Assert(ch.Left, gc.DeepEquals, left)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no change")
	}
}

func (s *RelationUnitSuite) assertNoScopeChange(c *gc.C, ws ...*state.RelationScopeWatcher) {
	s.State.StartSync()
	for _, w := range ws {
		select {
		case ch, ok := <-w.Changes():
			c.Fatalf("got unwanted change: %#v, %t", ch, ok)
		case <-time.After(coretesting.ShortWait):
		}
	}
}

func (s *RelationUnitSuite) TestNetworksForRelation(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	err := prr.pu0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.pu0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
		network.NewScopedAddress("4.3.2.1", network.ScopePublic),
	)

	boundSpace, ingress, egress, err := state.NetworksForRelation("", prr.pu0, prr.rel, nil, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, "")
	c.Assert(ingress, gc.DeepEquals, []string{"1.2.3.4"})
	c.Assert(egress, gc.DeepEquals, []string{"1.2.3.4/32"})
}

func (s *RelationUnitSuite) addDevicesWithAddresses(c *gc.C, machine *state.Machine, addresses ...string) {
	for _, address := range addresses {
		name := fmt.Sprintf("e%x", rand.Int31())
		deviceArgs := state.LinkLayerDeviceArgs{
			Name: name,
			Type: state.EthernetDevice,
		}
		err := machine.SetLinkLayerDevices(deviceArgs)
		c.Assert(err, jc.ErrorIsNil)
		device, err := machine.LinkLayerDevice(name)
		c.Assert(err, jc.ErrorIsNil)

		addressesArg := state.LinkLayerDeviceAddress{
			DeviceName:   name,
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  address,
		}
		err = machine.SetDevicesAddresses(addressesArg)
		c.Assert(err, jc.ErrorIsNil)
		deviceAddresses, err := device.Addresses()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(deviceAddresses, gc.HasLen, 1)
	}
}

func (s *RelationUnitSuite) TestNetworksForRelationWithSpaces(c *gc.C) {
	s.State.AddSubnet(corenetwork.SubnetInfo{CIDR: "1.2.0.0/16"})
	s.State.AddSpace("space-1", "pid-1", []string{"1.2.0.0/16"}, false)
	s.State.AddSubnet(corenetwork.SubnetInfo{CIDR: "2.2.0.0/16"})
	s.State.AddSpace("space-2", "pid-2", []string{"2.2.0.0/16"}, false)
	s.State.AddSubnet(corenetwork.SubnetInfo{CIDR: "3.2.0.0/16"})
	s.State.AddSpace("space-3", "pid-3", []string{"2.2.0.0/16"}, false)
	s.State.AddSubnet(corenetwork.SubnetInfo{CIDR: "4.3.0.0/16"})
	s.State.AddSpace("public-4", "pid-4", []string{"4.3.0.0/16"}, true)

	// We want to have all bindings set so that no actual binding is
	// really set to the default.
	bindings := map[string]string{
		"":             "space-3",
		"server-admin": "space-1",
		"server":       "space-2",
	}

	prr := newProReqRelationWithBindings(c, &s.ConnSuite, charm.ScopeGlobal, bindings, nil)
	err := prr.pu0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.pu0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	addresses := []network.Address{
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
		network.NewScopedAddress("2.2.3.4", network.ScopeCloudLocal),
		network.NewScopedAddress("3.2.3.4", network.ScopeCloudLocal),
		network.NewScopedAddress("4.3.2.1", network.ScopePublic),
	}
	err = machine.SetProviderAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)

	s.addDevicesWithAddresses(c, machine, "1.2.3.4/16", "2.2.3.4/16", "3.2.3.4/16", "4.3.2.1/16")

	boundSpace, ingress, egress, err := state.NetworksForRelation("", prr.pu0, prr.rel, nil, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, "space-3")
	c.Assert(ingress, gc.DeepEquals, []string{"2.2.3.4"})
	c.Assert(egress, gc.DeepEquals, []string{"2.2.3.4/32"})
}

func (s *RelationUnitSuite) TestNetworksForRelationRemoteRelation(c *gc.C) {
	prr := newRemoteProReqRelation(c, &s.ConnSuite)
	err := prr.ru0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
		network.NewScopedAddress("4.3.2.1", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)

	boundSpace, ingress, egress, err := state.NetworksForRelation("", prr.ru0, prr.rel, nil, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, "")
	c.Assert(ingress, gc.DeepEquals, []string{"4.3.2.1"})
	c.Assert(egress, gc.DeepEquals, []string{"4.3.2.1/32"})
}

func (s *RelationUnitSuite) TestNetworksForRelationRemoteRelationNoPublicAddr(c *gc.C) {
	prr := newRemoteProReqRelation(c, &s.ConnSuite)
	err := prr.ru0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	err = machine.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
	)
	c.Assert(err, jc.ErrorIsNil)

	boundSpace, ingress, egress, err := state.NetworksForRelation("", prr.ru0, prr.rel, nil, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, "")
	c.Assert(ingress, gc.DeepEquals, []string{"1.2.3.4"})
	c.Assert(egress, gc.DeepEquals, []string{"1.2.3.4/32"})
}

func (s *RelationUnitSuite) TestNetworksForRelationRemoteRelationDelayedPublicAddress(c *gc.C) {
	clk := testclock.NewClock(time.Now())
	attemptMade := make(chan struct{}, 10)
	s.PatchValue(&state.PreferredAddressRetryArgs, func() retry.CallArgs {
		return retry.CallArgs{
			Clock:       clk,
			Delay:       3 * time.Second,
			MaxDuration: 30 * time.Second,
			NotifyFunc: func(lastError error, attempt int) {
				attemptMade <- struct{}{}
			},
		}
	})
	prr := newRemoteProReqRelation(c, &s.ConnSuite)
	err := prr.ru0.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	id, err := prr.ru0.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, jc.ErrorIsNil)

	// Set up a public address after at least one attempt
	// is made to get it.We record the err for checking later
	// as gc.C is not thread safe.
	var funcErr error
	wg := sync.WaitGroup{}
	go func() {
		wg.Add(1)
		defer wg.Done()
		funcErr = clk.WaitAdvance(15*time.Second, time.Second, 1)
		if funcErr != nil {
			return
		}
		// Ensure we have a failed attempt to get public address.
		select {
		case <-attemptMade:
			funcErr = clk.WaitAdvance(10*time.Second, time.Second, 1)
			if funcErr != nil {
				return
			}
		case <-time.After(coretesting.LongWait):
			c.Fatal("waiting for public address attempt")
		}

		// Now set up the public address.
		funcErr = machine.SetProviderAddresses(
			network.NewScopedAddress("4.3.2.1", network.ScopePublic),
		)
		if funcErr != nil {
			return
		}
		funcErr = clk.WaitAdvance(10*time.Second, time.Second, 1)
		if funcErr != nil {
			return
		}
	}()

	boundSpace, ingress, egress, err := state.NetworksForRelation("", prr.ru0, prr.rel, nil, true)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure there we no errors in the go routine.
	wg.Wait()
	c.Assert(funcErr, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, "")
	c.Assert(ingress, gc.DeepEquals, []string{"4.3.2.1"})
	c.Assert(egress, gc.DeepEquals, []string{"4.3.2.1/32"})
}

func (s *RelationUnitSuite) TestNetworksForRelationCAASModel(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	gitlabch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	mysqlch := f.MakeCharm(c, &factory.CharmParams{Name: "mysql", Series: "kubernetes"})
	gitlab := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: gitlabch})
	mysql := f.MakeApplication(c, &factory.ApplicationParams{Name: "mysql", Charm: mysqlch})

	prr := newProReqRelationForApps(c, st, mysql, gitlab)

	// First no address.
	boundSpace, ingress, egress, err := state.NetworksForRelation("", prr.pu0, prr.rel, nil, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(boundSpace, gc.Equals, "")
	c.Assert(ingress, gc.HasLen, 0)
	c.Assert(egress, gc.HasLen, 0)

	// Add a application address.
	err = mysql.UpdateCloudService("", []network.Address{
		{Value: "1.2.3.4", Scope: network.ScopeCloudLocal},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pu0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	boundSpace, ingress, egress, err = state.NetworksForRelation("", prr.pu0, prr.rel, nil, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(boundSpace, gc.Equals, "")
	c.Assert(ingress, gc.DeepEquals, []string{"1.2.3.4"})
	c.Assert(egress, gc.DeepEquals, []string{"1.2.3.4/32"})
}

func (s *RelationUnitSuite) TestValidYes(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	rus := []*state.RelationUnit{prr.pru0, prr.pru1, prr.rru0, prr.rru1}
	for _, ru := range rus {
		result, err := ru.Valid()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, jc.IsTrue)
	}
}

func (s *RelationUnitSuite) TestValidNo(c *gc.C) {
	mysqlLogging := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	wpApp := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wpLogging := newProReqRelationForApps(c, s.State, wpApp, mysqlLogging.rapp)

	// We have logging related to mysql and to wordpress. We can
	// create an invalid RU by taking a logging unit from
	// mysql-logging and getting the wp-logging RU for it.
	ru, err := wpLogging.rel.Unit(mysqlLogging.ru0)
	c.Assert(err, jc.ErrorIsNil)

	result, err := ru.Valid()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.IsFalse)
}

func (s *RelationUnitSuite) TestValidSubordinateToSubordinate(c *gc.C) {
	// Relate mysql and logging...
	mysqlLogging := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	monApp := s.AddTestingApplication(c, "monitoring", s.AddTestingCharm(c, "monitoring"))
	// Relate mysql and monitoring - relation to a non-subordinate
	// needed to trigger creation of monitoring unit.
	mysqlMonitoring := newProReqRelationForApps(c, s.State, mysqlLogging.papp, monApp)
	// Monitor the logging app (newProReqRelationForApps assumes that
	// the providing app is a principal, so we need to do it by hand).
	loggingApp := mysqlLogging.rapp

	// Can't infer endpoints because they're ambiguous.
	ep1, err := loggingApp.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	ep2, err := monApp.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	rel, err := s.State.AddRelation(ep1, ep2)
	c.Assert(err, jc.ErrorIsNil)
	prr := &ProReqRelation{rel: rel, papp: loggingApp, rapp: monApp}

	// The units already exist, create the relation units.
	prr.pu0 = mysqlLogging.ru0
	prr.pru0, err = rel.Unit(prr.pu0)
	c.Assert(err, jc.ErrorIsNil)

	prr.ru0 = mysqlMonitoring.ru0
	prr.rru0, err = rel.Unit(prr.ru0)
	c.Assert(err, jc.ErrorIsNil)

	// Logging monitoring relation units should be valid.
	res, err := prr.rru0.Valid()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.IsTrue)
	res, err = prr.pru0.Valid()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.IsTrue)
}

type PeerRelation struct {
	rel                *state.Relation
	app                *state.Application
	u0, u1, u2, u3     *state.Unit
	ru0, ru1, ru2, ru3 *state.RelationUnit
}

func preventPeerUnitsDestroyRemove(c *gc.C, pr *PeerRelation) {
	preventUnitDestroyRemove(c, pr.u0)
	preventUnitDestroyRemove(c, pr.u1)
	preventUnitDestroyRemove(c, pr.u2)
	preventUnitDestroyRemove(c, pr.u3)
}

func newPeerRelation(c *gc.C, st *state.State) *PeerRelation {
	app := state.AddTestingApplication(c, st, "riak", state.AddTestingCharm(c, st, "riak"))
	ep, err := app.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)
	pr := &PeerRelation{rel: rel, app: app}
	pr.u0, pr.ru0 = addRU(c, app, rel, nil)
	pr.u1, pr.ru1 = addRU(c, app, rel, nil)
	pr.u2, pr.ru2 = addRU(c, app, rel, nil)
	pr.u3, pr.ru3 = addRU(c, app, rel, nil)
	return pr
}

type ProReqRelation struct {
	rel                    *state.Relation
	papp, rapp             *state.Application
	pu0, pu1, ru0, ru1     *state.Unit
	pru0, pru1, rru0, rru1 *state.RelationUnit
}

func preventProReqUnitsDestroyRemove(c *gc.C, prr *ProReqRelation) {
	preventUnitDestroyRemove(c, prr.pu0)
	preventUnitDestroyRemove(c, prr.pu1)
	preventUnitDestroyRemove(c, prr.ru0)
	preventUnitDestroyRemove(c, prr.ru1)
}

func newProReqRelation(c *gc.C, s *ConnSuite, scope charm.RelationScope) *ProReqRelation {
	papp := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	var rapp *state.Application
	if scope == charm.ScopeGlobal {
		rapp = s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	} else {
		rapp = s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	}
	return newProReqRelationForApps(c, s.State, papp, rapp)
}

func newProReqRelationWithBindings(c *gc.C, s *ConnSuite, scope charm.RelationScope, pbindings, rbindings map[string]string) *ProReqRelation {
	papp := s.AddTestingApplicationWithBindings(c, "mysql", s.AddTestingCharm(c, "mysql"), pbindings)
	var rapp *state.Application
	if scope == charm.ScopeGlobal {
		rapp = s.AddTestingApplicationWithBindings(c, "wordpress", s.AddTestingCharm(c, "wordpress"), rbindings)
	} else {
		rapp = s.AddTestingApplicationWithBindings(c, "logging", s.AddTestingCharm(c, "logging"), rbindings)
	}
	return newProReqRelationForApps(c, s.State, papp, rapp)
}

func newProReqRelationForApps(c *gc.C, st *state.State, proApp, reqApp *state.Application) *ProReqRelation {
	eps, err := st.InferEndpoints(proApp.Name(), reqApp.Name())
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	prr := &ProReqRelation{rel: rel, papp: proApp, rapp: reqApp}
	prr.pu0, prr.pru0 = addRU(c, proApp, rel, nil)
	prr.pu1, prr.pru1 = addRU(c, proApp, rel, nil)
	if eps[0].Scope == charm.ScopeGlobal {
		prr.ru0, prr.rru0 = addRU(c, reqApp, rel, nil)
		prr.ru1, prr.rru1 = addRU(c, reqApp, rel, nil)
	} else {
		prr.ru0, prr.rru0 = addRU(c, reqApp, rel, prr.pu0)
		prr.ru1, prr.rru1 = addRU(c, reqApp, rel, prr.pu1)
	}
	return prr
}

func (prr *ProReqRelation) watches() []*state.RelationScopeWatcher {
	return []*state.RelationScopeWatcher{
		prr.pru0.WatchScope(), prr.pru1.WatchScope(),
		prr.rru0.WatchScope(), prr.rru1.WatchScope(),
	}
}

func (prr *ProReqRelation) allEnterScope(c *gc.C) {
	err := prr.pru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.pru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.rru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = prr.rru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
}

func addRU(c *gc.C, app *state.Application, rel *state.Relation, principal *state.Unit) (*state.Unit, *state.RelationUnit) {
	// Given the application app in the relation rel, add a unit of app and create
	// a RelationUnit with rel. If principal is supplied, app is assumed to be
	// subordinate and the unit will be created by temporarily entering the
	// relation's scope as the principal.
	var u *state.Unit
	if principal == nil {
		unit, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		u = unit
	} else {
		origUnits, err := app.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		pru, err := rel.Unit(principal)
		c.Assert(err, jc.ErrorIsNil)
		err = pru.EnterScope(nil) // to create the subordinate
		c.Assert(err, jc.ErrorIsNil)
		err = pru.LeaveScope() // to reset to initial expected state
		c.Assert(err, jc.ErrorIsNil)
		newUnits, err := app.AllUnits()
		c.Assert(err, jc.ErrorIsNil)
		for _, unit := range newUnits {
			found := false
			for _, old := range origUnits {
				if unit.Name() == old.Name() {
					found = true
					break
				}
			}
			if !found {
				u = unit
				break
			}
		}
		c.Assert(u, gc.NotNil)
	}
	ru, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	return u, ru
}

type RemoteProReqRelation struct {
	rel                    *state.Relation
	papp                   *state.RemoteApplication
	rapp                   *state.Application
	pru0, pru1, rru0, rru1 *state.RelationUnit
	ru0, ru1               *state.Unit
}

func newRemoteProReqRelation(c *gc.C, s *ConnSuite) *RemoteProReqRelation {
	papp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	rapp := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	prr := &RemoteProReqRelation{rel: rel, papp: papp, rapp: rapp}
	prr.pru0 = addRemoteRU(c, rel, "mysql/0")
	prr.pru1 = addRemoteRU(c, rel, "mysql/1")
	prr.ru0, prr.rru0 = addRU(c, rapp, rel, nil)
	prr.ru1, prr.rru1 = addRU(c, rapp, rel, nil)
	return prr
}

func (prr *RemoteProReqRelation) watches() []*state.RelationScopeWatcher {
	return []*state.RelationScopeWatcher{
		prr.pru0.WatchScope(), prr.pru1.WatchScope(),
		prr.rru0.WatchScope(), prr.rru1.WatchScope(),
	}
}

func addRemoteRU(c *gc.C, rel *state.Relation, unitName string) *state.RelationUnit {
	// Add a remote unit with the given name to rel.
	ru, err := rel.RemoteUnit(unitName)
	c.Assert(err, jc.ErrorIsNil)
	return ru
}

type WatchScopeSuite struct {
	ConnSuite
}

var _ = gc.Suite(&WatchScopeSuite{})

func (s *WatchScopeSuite) TestPeer(c *gc.C) {
	// Create an application and get a peer relation.
	riak := s.AddTestingApplication(c, "riak", s.AddTestingCharm(c, "riak"))
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rels, err := riak.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)
	rel := rels[0]

	// Add some units to the application and set their private addresses; get
	// the relevant RelationUnits.
	// (Private addresses should be set by their unit agents on
	// startup; this test does not include that, but Join expects
	// the information to be available, and uses it to populate the
	// relation settings node.)
	addUnit := func(i int) *state.RelationUnit {
		unit, err := riak.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = unit.AssignToNewMachine()
		c.Assert(err, jc.ErrorIsNil)
		mId, err := unit.AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		machine, err := s.State.Machine(mId)
		c.Assert(err, jc.ErrorIsNil)
		privateAddr := network.NewScopedAddress(
			fmt.Sprintf("riak%d.example.com", i),
			network.ScopeCloudLocal,
		)
		machine.SetProviderAddresses(privateAddr)
		ru, err := rel.Unit(unit)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ru.Endpoint(), gc.Equals, riakEP)
		return ru
	}
	ru0 := addUnit(0)
	ru1 := addUnit(1)
	ru2 := addUnit(2)

	// ---------- Single unit ----------

	// Start watching the relation from the perspective of the first unit.
	w0 := ru0.Watch()
	defer testing.AssertStop(c, w0)
	w0c := testing.NewRelationUnitsWatcherC(c, s.State, w0)
	w0c.AssertChange(nil, nil)
	w0c.AssertNoChange()

	// Join the first unit to the relation, and change the settings, and
	// check that nothing apparently happens.
	err = ru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	changeSettings(c, ru0)
	w0c.AssertNoChange()

	// ---------- Two units ----------

	// Now join another unit to the relation...
	err = ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// ...and check that the first relation unit sees the change.
	expectChanged := []string{"riak/1"}
	w0c.AssertChange(expectChanged, nil)
	w0c.AssertNoChange()

	// Join again, check it's a no-op.
	err = ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	w0c.AssertNoChange()

	// Start watching the relation from the perspective of the second unit,
	// and check that it sees the right state.
	w1 := ru1.Watch()
	defer testing.AssertStop(c, w1)
	w1c := testing.NewRelationUnitsWatcherC(c, s.State, w1)
	expectChanged = []string{"riak/0"}
	w1c.AssertChange(expectChanged, nil)
	w1c.AssertNoChange()

	// ---------- Three units ----------

	// Whoa, it works. Ok, check the third unit's opinion of the state.
	w2 := ru2.Watch()
	defer testing.AssertStop(c, w2)
	w2c := testing.NewRelationUnitsWatcherC(c, s.State, w2)
	expectChanged = []string{"riak/0", "riak/1"}
	w2c.AssertChange(expectChanged, nil)
	w2c.AssertNoChange()

	// Join the third unit, and check the first and second units see it.
	err = ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	expectChanged = []string{"riak/2"}
	w0c.AssertChange(expectChanged, nil)
	w0c.AssertNoChange()
	w1c.AssertChange(expectChanged, nil)
	w1c.AssertNoChange()

	// Change the second unit's settings, and check that only
	// the first and third see changes.
	changeSettings(c, ru1)
	w1c.AssertNoChange()
	expectChanged = []string{"riak/1"}
	w0c.AssertChange(expectChanged, nil)
	w0c.AssertNoChange()
	w2c.AssertChange(expectChanged, nil)
	w2c.AssertNoChange()

	// ---------- Two units again ----------

	// Depart the second unit, and check that the first and third detect it.
	err = ru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	expectDeparted := []string{"riak/1"}
	w0c.AssertChange(nil, expectDeparted)
	w0c.AssertNoChange()
	w2c.AssertChange(nil, expectDeparted)
	w2c.AssertNoChange()

	// Change its settings, and check the others don't observe anything.
	changeSettings(c, ru1)
	w0c.AssertNoChange()
	w2c.AssertNoChange()

	// Check no spurious events showed up on the second unit's watch, and check
	// it closes cleanly.
	w1c.AssertNoChange()
	testing.AssertStop(c, w1)

	// OK, we're done here. Cleanup, and error detection during same,
	// will be handled by the deferred kill/stop calls. Phew.
}

func (s *WatchScopeSuite) TestProviderRequirerGlobal(c *gc.C) {
	// Create a pair of application and a relation between them.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, jc.ErrorIsNil)

	// Add some units to the application and set their private addresses.
	addUnit := func(srv *state.Application, sub string, ep state.Endpoint) *state.RelationUnit {
		unit, err := srv.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ru.Endpoint(), gc.Equals, ep)
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
	defer testing.AssertStop(c, msw0)
	msw0c := testing.NewRelationUnitsWatcherC(c, s.State, msw0)
	msw0c.AssertChange(nil, nil)
	msw0c.AssertNoChange()

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	err = msru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	changeSettings(c, msru0)
	msw0c.AssertNoChange()

	// Join the second provider unit, start its watch, and check what it thinks the
	// state of the relation is.
	err = msru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	msw1 := msru1.Watch()
	defer testing.AssertStop(c, msw1)
	msw1c := testing.NewRelationUnitsWatcherC(c, s.State, msw1)
	msw1c.AssertChange(nil, nil)
	msw1c.AssertNoChange()

	// Change the unit's settings, and check that neither provider unit
	// observes any change.
	changeSettings(c, msru1)
	msw1c.AssertNoChange()
	msw0c.AssertNoChange()

	// ---------- Two roles active ----------

	// Start watches from both requirer units' perspectives, and check that
	// they see the provider units.
	expectChanged := []string{"mysql/0", "mysql/1"}
	wpw0 := wpru0.Watch()
	defer testing.AssertStop(c, wpw0)
	wpw0c := testing.NewRelationUnitsWatcherC(c, s.State, wpw0)
	wpw0c.AssertChange(expectChanged, nil)
	wpw0c.AssertNoChange()
	wpw1 := wpru1.Watch()
	defer testing.AssertStop(c, wpw1)
	wpw1c := testing.NewRelationUnitsWatcherC(c, s.State, wpw1)
	wpw1c.AssertChange(expectChanged, nil)
	wpw1c.AssertNoChange()

	// Join the first requirer unit, and check the provider units see it.
	err = wpru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	expectChanged = []string{"wordpress/0"}
	msw0c.AssertChange(expectChanged, nil)
	msw0c.AssertNoChange()
	msw1c.AssertChange(expectChanged, nil)
	msw1c.AssertNoChange()

	// Join again, check no-op.
	err = wpru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	msw0c.AssertNoChange()
	msw1c.AssertNoChange()

	// Join the second requirer, and check the provider units see the change.
	err = wpru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	expectChanged = []string{"wordpress/1"}
	msw0c.AssertChange(expectChanged, nil)
	msw0c.AssertNoChange()
	msw1c.AssertChange(expectChanged, nil)
	msw1c.AssertNoChange()

	// Verify that neither requirer has observed any change to the relation.
	wpw0c.AssertNoChange()
	wpw1c.AssertNoChange()

	// Change settings for the first requirer, check providers see it...
	changeSettings(c, wpru0)
	expectChanged = []string{"wordpress/0"}
	msw0c.AssertChange(expectChanged, nil)
	msw0c.AssertNoChange()
	msw1c.AssertChange(expectChanged, nil)
	msw1c.AssertNoChange()

	// ...and requirers don't.
	wpw0c.AssertNoChange()
	wpw1c.AssertNoChange()

	// Depart the second requirer and check the providers see it...
	err = wpru1.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	expectDeparted := []string{"wordpress/1"}
	msw0c.AssertChange(nil, expectDeparted)
	msw0c.AssertNoChange()
	msw1c.AssertChange(nil, expectDeparted)
	msw1c.AssertNoChange()

	// ...and the requirers don't.
	wpw0c.AssertNoChange()
	wpw1c.AssertNoChange()

	// Cleanup handled by defers as before.
}

func (s *WatchScopeSuite) TestProviderRequirerContainer(c *gc.C) {
	// Create a pair of application and a relation between them.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	logging := s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(mysqlEP, loggingEP)
	c.Assert(err, jc.ErrorIsNil)

	// Change mysqlEP to match the endpoint that will actually be used by the relation.
	mysqlEP.Scope = charm.ScopeContainer

	// Add some units to the application and set their private addresses.
	addUnits := func(i int) (*state.RelationUnit, *state.RelationUnit) {
		msu, err := mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		msru, err := rel.Unit(msu)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(msru.Endpoint(), gc.Equals, mysqlEP)
		err = msru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
		err = msru.LeaveScope()
		c.Assert(err, jc.ErrorIsNil)
		lgu, err := s.State.Unit("logging/" + strconv.Itoa(i))
		c.Assert(err, jc.ErrorIsNil)
		lgru, err := rel.Unit(lgu)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(lgru.Endpoint(), gc.Equals, loggingEP)
		return msru, lgru
	}
	msru0, lgru0 := addUnits(0)
	msru1, lgru1 := addUnits(1)

	// ---------- Single role active ----------

	// Start watching the relation from the perspective of the first unit, and
	// check the initial event.
	msw0 := msru0.Watch()
	defer testing.AssertStop(c, msw0)
	msw0c := testing.NewRelationUnitsWatcherC(c, s.State, msw0)
	msw0c.AssertChange(nil, nil)
	msw0c.AssertNoChange()

	// Join the unit to the relation, change its settings, and check that
	// nothing apparently happens.
	err = msru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	changeSettings(c, msru0)
	msw0c.AssertNoChange()

	// Watch the relation from the perspective of the second provider, and
	// check initial event.
	msw1 := msru1.Watch()
	defer testing.AssertStop(c, msw1)
	msw1c := testing.NewRelationUnitsWatcherC(c, s.State, msw1)
	msw1c.AssertChange(nil, nil)
	msw1c.AssertNoChange()

	// Join the second provider unit to the relation, and check that neither
	// watching unit observes any change.
	err = msru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	msw1c.AssertNoChange()
	msw0c.AssertNoChange()

	// Change the unit's settings, and check that nothing apparently happens.
	changeSettings(c, msru1)
	msw1c.AssertNoChange()
	msw0c.AssertNoChange()

	// ---------- Two roles active ----------

	// Start a watch from the first requirer unit's perspective, and check it
	// only sees the first provider (with which it shares a container).
	lgw0 := lgru0.Watch()
	defer testing.AssertStop(c, lgw0)
	lgw0c := testing.NewRelationUnitsWatcherC(c, s.State, lgw0)
	expectChanged := []string{"mysql/0"}
	lgw0c.AssertChange(expectChanged, nil)
	lgw0c.AssertNoChange()

	// Join the first requirer unit, and check that only the first provider
	// observes the change.
	err = lgru0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	expectChanged = []string{"logging/0"}
	msw0c.AssertChange(expectChanged, nil)
	msw0c.AssertNoChange()
	msw1c.AssertNoChange()
	lgw0c.AssertNoChange()

	// Watch from the second requirer's perspective, and check it only sees the
	// second provider.
	lgw1 := lgru1.Watch()
	defer testing.AssertStop(c, lgw1)
	lgw1c := testing.NewRelationUnitsWatcherC(c, s.State, lgw1)
	expectChanged = []string{"mysql/1"}
	lgw1c.AssertChange(expectChanged, nil)
	lgw1c.AssertNoChange()

	// Join the second requirer, and check that the first provider observes it...
	err = lgru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	expectChanged = []string{"logging/1"}
	msw1c.AssertChange(expectChanged, nil)
	msw1c.AssertNoChange()

	// ...and that nothing else sees anything.
	msw0c.AssertNoChange()
	lgw0c.AssertNoChange()
	lgw1c.AssertNoChange()

	// Change the second provider's settings and check that the second
	// requirer notices...
	changeSettings(c, msru1)
	expectChanged = []string{"mysql/1"}
	lgw1c.AssertChange(expectChanged, nil)
	lgw1c.AssertNoChange()

	// ...but that nothing else does.
	msw0c.AssertNoChange()
	msw1c.AssertNoChange()
	msw0c.AssertNoChange()

	// Finally, depart the first provider, and check that only the first
	// requirer observes any change.
	err = msru0.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	expectDeparted := []string{"mysql/0"}
	lgw0c.AssertChange(nil, expectDeparted)
	lgw0c.AssertNoChange()
	lgw1c.AssertNoChange()
	msw0c.AssertNoChange()
	msw1c.AssertNoChange()

	// Again, I think we're done, and can be comfortable that the appropriate
	// connections are in place.
}

type WatchUnitsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&WatchUnitsSuite{})

func (s *WatchUnitsSuite) TestProviderRequirerGlobal(c *gc.C) {
	// Create a pair of applications and a relation between them.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, jc.ErrorIsNil)

	// Add some units to the applications and set their private addresses.
	addUnit := func(srv *state.Application) *state.RelationUnit {
		unit, err := srv.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		ru, err := rel.Unit(unit)
		c.Assert(err, jc.ErrorIsNil)
		return ru
	}
	mysql0 := addUnit(mysql)
	wordpress0 := addUnit(wordpress)

	wordpressWatcher, err := rel.WatchUnits("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	defer testing.AssertStop(c, wordpressWatcher)
	wordpressWatcherC := testing.NewRelationUnitsWatcherC(c, s.State, wordpressWatcher)
	wordpressWatcherC.AssertChange(nil, nil)
	wordpressWatcherC.AssertNoChange()

	// Join the mysql unit to the relation, change settings, and check
	// that only the mysql relation units watcher triggers.
	err = mysql0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	changeSettings(c, mysql0)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	mysqlWatcher, err := rel.WatchUnits("mysql")
	c.Assert(err, jc.ErrorIsNil)
	defer testing.AssertStop(c, mysqlWatcher)
	mysqlWatcherC := testing.NewRelationUnitsWatcherC(c, s.State, mysqlWatcher)
	mysqlWatcherC.AssertChange([]string{"mysql/0"}, nil)
	mysqlWatcherC.AssertNoChange()
	wordpressWatcherC.AssertNoChange()

	// Now join the wordpress unit to the relation, and check that only
	// the wordpress relation units watcher triggers.
	err = wordpress0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	wordpressWatcherC.AssertChange([]string{"wordpress/0"}, nil)
	wordpressWatcherC.AssertNoChange()
	mysqlWatcherC.AssertNoChange()
}

func (s *WatchUnitsSuite) TestProviderRequirerContainer(c *gc.C) {
	// Create a pair of applications and a relation between them.
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	logging := s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(mysqlEP, loggingEP)
	c.Assert(err, jc.ErrorIsNil)

	_, err = rel.WatchUnits("mysql")
	c.Assert(err, gc.ErrorMatches, `"juju-info" endpoint is not globally scoped`)
	_, err = rel.WatchUnits("logging")
	c.Assert(err, gc.ErrorMatches, `"info" endpoint is not globally scoped`)
}

func changeSettings(c *gc.C, ru *state.RelationUnit) {
	node, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	value, _ := node.Get("value")
	v, _ := value.(int)
	node.Set("value", v+1)
	_, err = node.Write()
	c.Assert(err, jc.ErrorIsNil)
}
