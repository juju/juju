// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/firewall"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&addressWatcherSuite{})

type addressWatcherSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
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
}

func (s *addressWatcherSuite) setupRelation(c *gc.C, addr string) *mockRelation {
	rel := newMockRelation(123)
	rel.ruwApp = "django"
	// Initial event.
	rel.ew.changes <- []string{}
	s.st.relations["remote-db2:db django:db"] = rel
	unit := newMockUnit("django/0")
	unit.publicAddress = network.NewSpaceAddress(addr)
	unit.machineId = "0"
	s.st.units["django/0"] = unit
	app := newMockApplication("django")
	app.units = []*mockUnit{unit}
	s.st.applications["django"] = app
	s.st.machines["0"] = newMockMachine("0")
	return rel
}

func (s *addressWatcherSuite) TestInitial(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	// django/0 is initially in scope
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}

	wc.AssertChange("54.1.2.3/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitEntersScope(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	rel.ruw.changes <- watcher.RelationUnitsChange{}

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertChange("54.1.2.3/32")
	wc.AssertNoChange()

	// A not found unit doesn't trigger an event.
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"unknown/0": {},
		},
	}
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestTwoUnitsEntersScope(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	rel.ruw.changes <- watcher.RelationUnitsChange{}

	unit := newMockUnit("django/1")
	unit.publicAddress = network.NewSpaceAddress("54.4.5.6")
	unit.machineId = "1"
	s.st.units["django/1"] = unit
	s.st.machines["1"] = newMockMachine("1")

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}
	wc.AssertChange("54.1.2.3/32", "54.4.5.6/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestAnotherUnitsEntersScope(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	rel.ruw.changes <- watcher.RelationUnitsChange{}

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertChange("54.1.2.3/32")
	wc.AssertNoChange()

	unit := newMockUnit("django/1")
	unit.publicAddress = network.NewSpaceAddress("54.4.5.6")
	unit.machineId = "1"
	s.st.units["django/1"] = unit
	s.st.machines["1"] = newMockMachine("1")
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/1": {},
		},
	}
	wc.AssertChange("54.1.2.3/32", "54.4.5.6/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitEntersScopeNoPublicAddress(c *gc.C) {
	rel := s.setupRelation(c, "")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}

	// Even though the unit has no public address,
	// we still expect the initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	// This time no event.
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitEntersScopeNotAssigned(c *gc.C) {
	rel := s.setupRelation(c, "")
	s.st.units["django/0"].assigned = false
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}

	// Even though the unit is not assigned,
	// we still expect the initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	// This time no event.
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitLeavesScopeInitial(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Departed: []string{"django/0"},
	}

	// Even though the unit has not been seen via enter scope,
	// we still expect the initial event.
	wc.AssertChange()
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestUnitLeavesScope(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	rel.ruw.changes <- watcher.RelationUnitsChange{}

	unit := newMockUnit("django/1")
	unit.publicAddress = network.NewSpaceAddress("54.4.5.6")
	unit.machineId = "1"
	s.st.units["django/1"] = unit
	s.st.machines["1"] = newMockMachine("1")

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}
	wc.AssertChange("54.1.2.3/32", "54.4.5.6/32")
	wc.AssertNoChange()

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Departed: []string{"django/0"},
	}

	wc.AssertChange("54.4.5.6/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestTwoUnitsSameAddressOneLeaves(c *gc.C) {
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	rel.ruw.changes <- watcher.RelationUnitsChange{}

	unit := newMockUnit("django/1")
	unit.publicAddress = network.NewSpaceAddress("54.1.2.3")
	unit.machineId = "0"
	s.st.units["django/1"] = unit

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}
	wc.AssertChange("54.1.2.3/32")
	wc.AssertNoChange()

	// One leaves, no change.
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Departed: []string{"django/0"},
	}

	wc.AssertNoChange()

	// Last one leaves.
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Departed: []string{"django/1"},
	}

	wc.AssertChange()
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestSecondUnitJoinsOnSameMachine(c *gc.C) {
	rel := s.setupRelation(c, "55.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	// django/0 is initially in scope
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}

	wc.AssertChange("55.1.2.3/32")
	wc.AssertNoChange()

	// Another unit joins on the same machine.
	unit := newMockUnit("django/1")
	unit.machineId = "0"
	s.st.units["django/1"] = unit

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/1": {},
		},
	}
	// No new addresses.
	wc.AssertNoChange()

	// Machine 0 changes address.
	s.st.units["django/0"].updateAddress("56.1.2.3")
	s.st.units["django/1"].updateAddress("56.1.2.3")
	s.st.machines["0"].watcher.changes <- struct{}{}

	wc.AssertChange("56.1.2.3/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestSeesMachineAddressChanges(c *gc.C) {
	rel := s.setupRelation(c, "2.3.4.5")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	// django/0 is initially in scope
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}

	wc.AssertChange("2.3.4.5/32")
	wc.AssertNoChange()

	s.st.units["django/0"].updateAddress("5.4.3.3")
	s.st.machines["0"].watcher.changes <- struct{}{}

	wc.AssertChange("5.4.3.3/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestHandlesMachineAddressChangesWithNoEffect(c *gc.C) {
	rel := s.setupRelation(c, "2.3.4.5")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	// django/0 is initially in scope
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}

	wc.AssertChange("2.3.4.5/32")
	wc.AssertNoChange()

	// Public address for the unit stays the same (maybe some other address changed).
	s.st.machines["0"].watcher.changes <- struct{}{}

	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestHandlesUnitGoneWhenMachineAddressChanges(c *gc.C) {
	rel := s.setupRelation(c, "2.3.4.5")
	unit := newMockUnit("django/1")
	unit.publicAddress = network.NewSpaceAddress("2.3.4.5")
	unit.machineId = "0"
	s.st.units["django/1"] = unit
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
			"django/1": {},
		},
	}

	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)

	wc.AssertChange("2.3.4.5/32")
	wc.AssertNoChange()

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Departed: []string{"django/1"},
	}
	s.st.units["django/0"].updateAddress("6.7.8.9")
	s.st.machines["0"].watcher.changes <- struct{}{}

	wc.AssertChange("6.7.8.9/32")
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestModelEgressAddressUsed(c *gc.C) {
	s.st.configAttrs["egress-subnets"] = "10.0.0.1/16"
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	rel.ruw.changes <- watcher.RelationUnitsChange{}

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertChange("10.0.0.1/16")
	wc.AssertNoChange()

	// Change user configured egress addresses.
	s.st.configAttrs["egress-subnets"] = "192.168.0.1/16"
	s.st.modelWatcher.changes <- struct{}{}
	wc.AssertChange("192.168.0.1/16")
	wc.AssertNoChange()

	// Reset user configured egress addresses.
	s.st.configAttrs["egress-subnets"] = ""
	s.st.modelWatcher.changes <- struct{}{}
	wc.AssertChange("54.1.2.3/32")
	wc.AssertNoChange()

	// A not found unit doesn't trigger an event.
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"unknown/0": {},
		},
	}
	wc.AssertNoChange()
}

func (s *addressWatcherSuite) TestRelationEgressAddressUsed(c *gc.C) {
	// Set up a model egress-address to ensure it is ignored when a relation one is used.
	s.st.configAttrs["egress-subnets"] = "10.0.0.1/16"
	rel := s.setupRelation(c, "54.1.2.3")
	w, err := firewall.NewEgressAddressWatcher(s.st, rel, "django")
	c.Assert(err, jc.ErrorIsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, nopSyncStarter{}, w)
	rel.ruw.changes <- watcher.RelationUnitsChange{}

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	// New relation ingress cidr.
	rel.ew.changes <- []string{"192.168.0.0/8"}

	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"django/0": {},
		},
	}
	wc.AssertChange("192.168.0.0/8")
	wc.AssertNoChange()

	// Change model egress addresses, no change since relation overrides.
	s.st.configAttrs["egress-subnets"] = "192.168.0.1/16"
	s.st.modelWatcher.changes <- struct{}{}
	wc.AssertNoChange()

	rel.ew.changes <- []string{"10.1.2.0/8"}
	wc.AssertChange("10.1.2.0/8")
	wc.AssertNoChange()

	// A not found unit doesn't trigger an event.
	rel.ruw.changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			"unknown/0": {},
		},
	}
	wc.AssertNoChange()
}
