// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type uniterV1Suite struct {
	uniterBaseSuite
	*commontesting.EnvironWatcherTest

	uniter *uniter.UniterAPIV1
}

var _ = gc.Suite(&uniterV1Suite{})

func (s *uniterV1Suite) SetUpTest(c *gc.C) {
	s.uniterBaseSuite.setUpTest(c)

	uniterAPIV1, err := uniter.NewUniterAPIV1(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
	s.uniter = uniterAPIV1

	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(
		s.uniter,
		s.State,
		s.resources,
		commontesting.NoSecrets,
	)
}

func (s *uniterV1Suite) TestUniterFailsWithNonUnitAgentUser(c *gc.C) {
	factory := func(st *state.State, res *common.Resources, auth common.Authorizer) error {
		_, err := uniter.NewUniterAPIV1(st, res, auth)
		return err
	}
	s.testUniterFailsWithNonUnitAgentUser(c, factory)
}

func (s *uniterV1Suite) TestSetStatus(c *gc.C) {
	s.testSetStatus(c, s.uniter)
}

func (s *uniterV1Suite) TestLife(c *gc.C) {
	s.testLife(c, s.uniter)
}

func (s *uniterV1Suite) TestEnsureDead(c *gc.C) {
	s.testEnsureDead(c, s.uniter)
}

func (s *uniterV1Suite) TestWatch(c *gc.C) {
	s.testWatch(c, s.uniter)
}

func (s *uniterV1Suite) TestPublicAddress(c *gc.C) {
	s.testPublicAddress(c, s.uniter)
}

func (s *uniterV1Suite) TestPrivateAddress(c *gc.C) {
	s.testPrivateAddress(c, s.uniter)
}

func (s *uniterV1Suite) TestResolved(c *gc.C) {
	s.testResolved(c, s.uniter)
}

func (s *uniterV1Suite) TestClearResolved(c *gc.C) {
	s.testClearResolved(c, s.uniter)
}

func (s *uniterV1Suite) TestGetPrincipal(c *gc.C) {
	factory := func(
		st *state.State,
		resources *common.Resources,
		authorizer common.Authorizer,
	) (getPrincipal, error) {
		return uniter.NewUniterAPIV1(st, resources, authorizer)
	}
	s.testGetPrincipal(c, s.uniter, factory)
}

func (s *uniterV1Suite) TestHasSubordinates(c *gc.C) {
	s.testHasSubordinates(c, s.uniter)
}

func (s *uniterV1Suite) TestDestroy(c *gc.C) {
	s.testDestroy(c, s.uniter)
}

func (s *uniterV1Suite) TestDestroyAllSubordinates(c *gc.C) {
	s.testDestroyAllSubordinates(c, s.uniter)
}

func (s *uniterV1Suite) TestCharmURL(c *gc.C) {
	s.testCharmURL(c, s.uniter)
}

func (s *uniterV1Suite) TestSetCharmURL(c *gc.C) {
	s.testSetCharmURL(c, s.uniter)
}

func (s *uniterV1Suite) TestOpenPorts(c *gc.C) {
	s.testOpenPorts(c, s.uniter)
}

func (s *uniterV1Suite) TestClosePorts(c *gc.C) {
	s.testClosePorts(c, s.uniter)
}

func (s *uniterV1Suite) TestOpenPort(c *gc.C) {
	s.testOpenPort(c, s.uniter)
}

func (s *uniterV1Suite) TestClosePort(c *gc.C) {
	s.testClosePort(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchConfigSettings(c *gc.C) {
	s.testWatchConfigSettings(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchActions(c *gc.C) {
	s.testWatchActions(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchPreexistingActions(c *gc.C) {
	s.testWatchPreexistingActions(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchActionsMalformedTag(c *gc.C) {
	s.testWatchActionsMalformedTag(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchActionsMalformedUnitName(c *gc.C) {
	s.testWatchActionsMalformedUnitName(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchActionsNotUnit(c *gc.C) {
	s.testWatchActionsNotUnit(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchActionsPermissionDenied(c *gc.C) {
	s.testWatchActionsPermissionDenied(c, s.uniter)
}

func (s *uniterV1Suite) TestConfigSettings(c *gc.C) {
	s.testConfigSettings(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchServiceRelations(c *gc.C) {
	s.testWatchServiceRelations(c, s.uniter)
}

func (s *uniterV1Suite) TestCharmArchiveSha256(c *gc.C) {
	s.testCharmArchiveSha256(c, s.uniter)
}

func (s *uniterV1Suite) TestCurrentEnvironUUID(c *gc.C) {
	s.testCurrentEnvironUUID(c, s.uniter)
}

func (s *uniterV1Suite) TestCurrentEnvironment(c *gc.C) {
	s.testCurrentEnvironment(c, s.uniter)
}

func (s *uniterV1Suite) TestActions(c *gc.C) {
	s.testActions(c, s.uniter)
}

func (s *uniterV1Suite) TestActionsNotPresent(c *gc.C) {
	s.testActionsNotPresent(c, s.uniter)
}

func (s *uniterV1Suite) TestActionsWrongUnit(c *gc.C) {
	factory := func(
		st *state.State,
		resources *common.Resources,
		authorizer common.Authorizer,
	) (actions, error) {
		return uniter.NewUniterAPIV1(st, resources, authorizer)
	}
	s.testActionsWrongUnit(c, factory)
}

func (s *uniterV1Suite) TestActionsPermissionDenied(c *gc.C) {
	s.testActionsPermissionDenied(c, s.uniter)
}

func (s *uniterV1Suite) TestFinishActionsSuccess(c *gc.C) {
	s.testFinishActionsSuccess(c, s.uniter)
}

func (s *uniterV1Suite) TestFinishActionsFailure(c *gc.C) {
	s.testFinishActionsFailure(c, s.uniter)
}

func (s *uniterV1Suite) TestFinishActionsAuthAccess(c *gc.C) {
	s.testFinishActionsAuthAccess(c, s.uniter)
}

func (s *uniterV1Suite) TestRelation(c *gc.C) {
	s.testRelation(c, s.uniter)
}

func (s *uniterV1Suite) TestRelationById(c *gc.C) {
	s.testRelationById(c, s.uniter)
}

func (s *uniterV1Suite) TestProviderType(c *gc.C) {
	s.testProviderType(c, s.uniter)
}

func (s *uniterV1Suite) TestEnterScope(c *gc.C) {
	s.testEnterScope(c, s.uniter)
}

func (s *uniterV1Suite) TestLeaveScope(c *gc.C) {
	s.testLeaveScope(c, s.uniter)
}

func (s *uniterV1Suite) TestJoinedRelations(c *gc.C) {
	s.testJoinedRelations(c, s.uniter)
}

func (s *uniterV1Suite) TestReadSettings(c *gc.C) {
	s.testReadSettings(c, s.uniter)
}

func (s *uniterV1Suite) TestReadSettingsWithNonStringValuesFails(c *gc.C) {
	s.testReadSettingsWithNonStringValuesFails(c, s.uniter)
}

func (s *uniterV1Suite) TestReadRemoteSettings(c *gc.C) {
	s.testReadRemoteSettings(c, s.uniter)
}

func (s *uniterV1Suite) TestReadRemoteSettingsWithNonStringValuesFails(c *gc.C) {
	s.testReadRemoteSettingsWithNonStringValuesFails(c, s.uniter)
}

func (s *uniterV1Suite) TestUpdateSettings(c *gc.C) {
	s.testUpdateSettings(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchRelationUnits(c *gc.C) {
	s.testWatchRelationUnits(c, s.uniter)
}

func (s *uniterV1Suite) TestAPIAddresses(c *gc.C) {
	s.testAPIAddresses(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchUnitAddresses(c *gc.C) {
	s.testWatchUnitAddresses(c, s.uniter)
}

func (s *uniterV1Suite) TestAddMetrics(c *gc.C) {
	s.testAddMetrics(c, s.uniter)
}

func (s *uniterV1Suite) TestAddMetricsIncorrectTag(c *gc.C) {
	s.testAddMetricsIncorrectTag(c, s.uniter)
}

func (s *uniterV1Suite) TestAddMetricsUnauthenticated(c *gc.C) {
	s.testAddMetricsUnauthenticated(c, s.uniter)
}

func (s *uniterV1Suite) TestGetMeterStatus(c *gc.C) {
	s.testGetMeterStatus(c, s.uniter)
}

func (s *uniterV1Suite) TestGetMeterStatusUnauthenticated(c *gc.C) {
	s.testGetMeterStatusUnauthenticated(c, s.uniter)
}

func (s *uniterV1Suite) TestGetMeterStatusBadTag(c *gc.C) {
	s.testGetMeterStatusBadTag(c, s.uniter)
}

func (s *uniterV1Suite) TestWatchMeterStatus(c *gc.C) {
	s.testWatchMeterStatus(c, s.uniter)
}

func (s *uniterV1Suite) TestGetOwnerTagV1NotImplemented(c *gc.C) {
	apiservertesting.AssertNotImplemented(c, s.uniter, "GetOwnerTag")
}

func (s *uniterV1Suite) TestServiceOwner(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "service-wordpress"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "service-foo"},
	}}
	result, err := s.uniter.ServiceOwner(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.AdminUserTag(c).String()},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterV1Suite) TestAssignedMachine(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "service-mysql"},
		{Tag: "service-wordpress"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "service-foo"},
		{Tag: "relation-svc1.rel1#svc2.rel2"},
	}}
	result, err := s.uniter.AssignedMachine(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "machine-0"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterV1Suite) TestAllMachinePorts(c *gc.C) {
	// Verify no ports are opened yet on the machine or unit.
	machinePorts, err := s.machine0.AllPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(machinePorts, gc.HasLen, 0)
	unitPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitPorts, gc.HasLen, 0)

	// Add another mysql unit on machine 0.
	mysqlUnit1, err := s.mysql.AddUnit()
	c.Assert(err, gc.IsNil)
	err = mysqlUnit1.AssignToMachine(s.machine0)
	c.Assert(err, gc.IsNil)

	// Open some ports on both units.
	err = s.wordpressUnit.OpenPorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil)
	err = s.wordpressUnit.OpenPorts("udp", 10, 20)
	c.Assert(err, gc.IsNil)
	err = mysqlUnit1.OpenPorts("tcp", 201, 250)
	c.Assert(err, gc.IsNil)
	err = mysqlUnit1.OpenPorts("udp", 1, 8)
	c.Assert(err, gc.IsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-42"},
		{Tag: "service-wordpress"},
	}}
	expectPorts := []params.MachinePortRange{
		{UnitTag: "unit-wordpress-0", PortRange: network.PortRange{100, 200, "tcp"}},
		{UnitTag: "unit-wordpress-0", PortRange: network.PortRange{10, 20, "udp"}},
		{UnitTag: "unit-mysql-1", PortRange: network.PortRange{201, 250, "tcp"}},
		{UnitTag: "unit-mysql-1", PortRange: network.PortRange{1, 8, "udp"}},
	}
	result, err := s.uniter.AllMachinePorts(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.MachinePortsResults{
		Results: []params.MachinePortsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Ports: expectPorts},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterV1Suite) TestRequestReboot(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machine0.Tag().String()},
		{Tag: s.machine1.Tag().String()},
		{Tag: "bogus"},
		{Tag: "nasty-tag"},
	}}
	errResult, err := s.uniter.RequestReboot(args)
	c.Assert(err, gc.IsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		}})

	rFlag, err := s.machine0.GetRebootFlag()
	c.Assert(err, gc.IsNil)
	c.Assert(rFlag, jc.IsTrue)

	rFlag, err = s.machine1.GetRebootFlag()
	c.Assert(err, gc.IsNil)
	c.Assert(rFlag, jc.IsFalse)
}
