// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/remotefirewaller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&addressWatcherSuite{})

type addressWatcherSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *remotefirewaller.FirewallerAPI
}

type nopSyncStarter struct{}

func (nopSyncStarter) StartSync() {}

func (s *addressWatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState(coretesting.ModelTag.Id())
	api, err := remotefirewaller.NewRemoteFirewallerAPI(s.st, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *addressWatcherSuite) setupRelation(c *gc.C, addr string) *mockRelation {
	rel := newMockRelation(123)
	rel.ruwApp = "testapp"
	s.st.relations["remote-db2:db django:db"] = rel
	app := newMockApplication("django")
	s.st.applications["django"] = app
	unit := newMockUnit("django/0")
	unit.publicAddress = network.Address{Value: addr}
	s.st.units["django/0"] = unit
	return rel
}

func (s *addressWatcherSuite) TestUnitEntersScope(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := remotefirewaller.NewIngressAddressWatcher(s.st, rel, "testapp")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertChange("54.1.2.3/32")
	wc.AssertNoChange()

	// A not found unit doesn't trigger an event.
	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"unknown/0": {},
		},
	}
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestTwoUnitsEntersScope(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := remotefirewaller.NewIngressAddressWatcher(s.st, rel, "testapp")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	unit := newMockUnit("django/1")
	unit.publicAddress = network.Address{Value: "54.4.5.6"}
	s.st.units["django/1"] = unit
	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}
	wc.AssertChange("54.1.2.3/32", "54.4.5.6/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestAnotherUnitsEntersScope(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := remotefirewaller.NewIngressAddressWatcher(s.st, rel, "testapp")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertChange("54.1.2.3/32")
	wc.AssertNoChange()

	unit := newMockUnit("django/1")
	unit.publicAddress = network.Address{Value: "54.4.5.6"}
	s.st.units["django/1"] = unit
	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/1": {},
		},
	}
	wc.AssertChange("54.1.2.3/32", "54.4.5.6/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitEntersScopeNoPublicAddress(c *gc.C) {
	rel := s.setupRelation(c, "")
	w, err := remotefirewaller.NewIngressAddressWatcher(s.st, rel, "testapp")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
		},
	}

	// Even though the unit has no public address,
	// we still expect the initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	// This time no event.
	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitEntersScopeNotAssigned(c *gc.C) {
	rel := s.setupRelation(c, "")
	s.st.units["django/0"].assigned = false
	w, err := remotefirewaller.NewIngressAddressWatcher(s.st, rel, "testapp")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
		},
	}

	// Even though the unit is not assigned,
	// we still expect the initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	// This time no event.
	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitLeavesScopeInitial(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := remotefirewaller.NewIngressAddressWatcher(s.st, rel, "testapp")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	rel.ruw.changes <- params.RelationUnitsChange{
		Departed: []string{"django/0"},
	}

	// Even though the unit has not been seen via enter scope,
	// we still expect the initial event.
	wc.AssertChange()
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitLeavesScope(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := remotefirewaller.NewIngressAddressWatcher(s.st, rel, "testapp")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	unit := newMockUnit("django/1")
	unit.publicAddress = network.Address{Value: "54.4.5.6"}
	s.st.units["django/1"] = unit
	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}
	wc.AssertChange("54.1.2.3/32", "54.4.5.6/32")
	wc.AssertNoChange()

	rel.ruw.changes <- params.RelationUnitsChange{
		Departed: []string{"django/0"},
	}

	wc.AssertChange("54.4.5.6/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestTwoUnitsSameAddressOneLeaves(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := remotefirewaller.NewIngressAddressWatcher(s.st, rel, "testapp")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	unit := newMockUnit("django/1")
	unit.publicAddress = network.Address{Value: "54.1.2.3"}
	s.st.units["django/1"] = unit
	rel.ruw.changes <- params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}
	wc.AssertChange("54.1.2.3/32")
	wc.AssertNoChange()

	// One leaves, no change.
	rel.ruw.changes <- params.RelationUnitsChange{
		Departed: []string{"django/0"},
	}

	wc.AssertNoChange()

	// Last one leaves.
	rel.ruw.changes <- params.RelationUnitsChange{
		Departed: []string{"django/1"},
	}

	wc.AssertChange()
	wc.AssertNoChange()
}
