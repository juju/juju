// Copyright 2014 Canonical Ltd.
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
	"github.com/juju/juju/state"
)

type uniterV0Suite struct {
	uniterBaseSuite
	*commontesting.EnvironWatcherTest

	uniter        *uniter.UniterAPIV0
	meteredUniter *uniter.UniterAPIV0
}

var _ = gc.Suite(&uniterV0Suite{})

func (s *uniterV0Suite) SetUpTest(c *gc.C) {
	s.uniterBaseSuite.setUpTest(c)

	uniterAPIV0, err := uniter.NewUniterAPIV0(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.uniter = uniterAPIV0

	meteredAuthorizer := apiservertesting.FakeAuthorizer{
		Tag: s.meteredUnit.Tag(),
	}
	s.meteredUniter, err = uniter.NewUniterAPIV0(
		s.State,
		s.resources,
		meteredAuthorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(
		s.uniter,
		s.State,
		s.resources,
		commontesting.NoSecrets,
	)
}

func (s *uniterV0Suite) TestUniterFailsWithNonUnitAgentUser(c *gc.C) {
	factory := func(st *state.State, res *common.Resources, auth common.Authorizer) error {
		_, err := uniter.NewUniterAPIV0(st, res, auth)
		return err
	}
	s.testUniterFailsWithNonUnitAgentUser(c, factory)
}

func (s *uniterV0Suite) TestSetStatus(c *gc.C) {
	s.testSetStatus(c, s.uniter)
}

func (s *uniterV0Suite) TestLife(c *gc.C) {
	s.testLife(c, s.uniter)
}

func (s *uniterV0Suite) TestEnsureDead(c *gc.C) {
	s.testEnsureDead(c, s.uniter)
}

func (s *uniterV0Suite) TestWatch(c *gc.C) {
	s.testWatch(c, s.uniter)
}

func (s *uniterV0Suite) TestPublicAddress(c *gc.C) {
	s.testPublicAddress(c, s.uniter)
}

func (s *uniterV0Suite) TestPrivateAddress(c *gc.C) {
	s.testPrivateAddress(c, s.uniter)
}

func (s *uniterV0Suite) TestResolved(c *gc.C) {
	s.testResolved(c, s.uniter)
}

func (s *uniterV0Suite) TestClearResolved(c *gc.C) {
	s.testClearResolved(c, s.uniter)
}

func (s *uniterV0Suite) TestGetPrincipal(c *gc.C) {
	factory := func(
		st *state.State,
		resources *common.Resources,
		authorizer common.Authorizer,
	) (getPrincipal, error) {
		return uniter.NewUniterAPIV0(st, resources, authorizer)
	}
	s.testGetPrincipal(c, s.uniter, factory)
}

func (s *uniterV0Suite) TestHasSubordinates(c *gc.C) {
	s.testHasSubordinates(c, s.uniter)
}

func (s *uniterV0Suite) TestDestroy(c *gc.C) {
	s.testDestroy(c, s.uniter)
}

func (s *uniterV0Suite) TestDestroyAllSubordinates(c *gc.C) {
	s.testDestroyAllSubordinates(c, s.uniter)
}

func (s *uniterV0Suite) TestCharmURL(c *gc.C) {
	s.testCharmURL(c, s.uniter)
}

func (s *uniterV0Suite) TestSetCharmURL(c *gc.C) {
	s.testSetCharmURL(c, s.uniter)
}

func (s *uniterV0Suite) TestOpenPorts(c *gc.C) {
	s.testOpenPorts(c, s.uniter)
}

func (s *uniterV0Suite) TestClosePorts(c *gc.C) {
	s.testClosePorts(c, s.uniter)
}

func (s *uniterV0Suite) TestOpenPort(c *gc.C) {
	s.testOpenPort(c, s.uniter)
}

func (s *uniterV0Suite) TestClosePort(c *gc.C) {
	s.testClosePort(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchConfigSettings(c *gc.C) {
	s.testWatchConfigSettings(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchActionNotifications(c *gc.C) {
	s.testWatchActionNotifications(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchPreexistingActions(c *gc.C) {
	s.testWatchPreexistingActions(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchActionNotificationsMalformedTag(c *gc.C) {
	s.testWatchActionNotificationsMalformedTag(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchActionNotificationsMalformedUnitName(c *gc.C) {
	s.testWatchActionNotificationsMalformedUnitName(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchActionNotificationsNotUnit(c *gc.C) {
	s.testWatchActionNotificationsNotUnit(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchActionNotificationsPermissionDenied(c *gc.C) {
	s.testWatchActionNotificationsPermissionDenied(c, s.uniter)
}

func (s *uniterV0Suite) TestConfigSettings(c *gc.C) {
	s.testConfigSettings(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchServiceRelations(c *gc.C) {
	s.testWatchServiceRelations(c, s.uniter)
}

func (s *uniterV0Suite) TestCharmArchiveSha256(c *gc.C) {
	s.testCharmArchiveSha256(c, s.uniter)
}

func (s *uniterV0Suite) TestCharmArchiveURLs(c *gc.C) {
	s.testCharmArchiveURLs(c, s.uniter)
}

func (s *uniterV0Suite) TestCurrentEnvironUUID(c *gc.C) {
	s.testCurrentEnvironUUID(c, s.uniter)
}

func (s *uniterV0Suite) TestCurrentEnvironment(c *gc.C) {
	s.testCurrentEnvironment(c, s.uniter)
}

func (s *uniterV0Suite) TestActions(c *gc.C) {
	s.testActions(c, s.uniter)
}

func (s *uniterV0Suite) TestActionsNotPresent(c *gc.C) {
	s.testActionsNotPresent(c, s.uniter)
}

func (s *uniterV0Suite) TestActionsWrongUnit(c *gc.C) {
	factory := func(
		st *state.State,
		resources *common.Resources,
		authorizer common.Authorizer,
	) (actions, error) {
		return uniter.NewUniterAPIV0(st, resources, authorizer)
	}
	s.testActionsWrongUnit(c, factory)
}

func (s *uniterV0Suite) TestActionsPermissionDenied(c *gc.C) {
	s.testActionsPermissionDenied(c, s.uniter)
}

func (s *uniterV0Suite) TestFinishActionsSuccess(c *gc.C) {
	s.testFinishActionsSuccess(c, s.uniter)
}

func (s *uniterV0Suite) TestFinishActionsFailure(c *gc.C) {
	s.testFinishActionsFailure(c, s.uniter)
}

func (s *uniterV0Suite) TestFinishActionsAuthAccess(c *gc.C) {
	s.testFinishActionsAuthAccess(c, s.uniter)
}

func (s *uniterV0Suite) TestBeginActions(c *gc.C) {
	s.testBeginActions(c, s.uniter)
}

func (s *uniterV0Suite) TestRelation(c *gc.C) {
	s.testRelation(c, s.uniter)
}

func (s *uniterV0Suite) TestRelationById(c *gc.C) {
	s.testRelationById(c, s.uniter)
}

func (s *uniterV0Suite) TestProviderType(c *gc.C) {
	s.testProviderType(c, s.uniter)
}

func (s *uniterV0Suite) TestEnterScope(c *gc.C) {
	s.testEnterScope(c, s.uniter)
}

func (s *uniterV0Suite) TestLeaveScope(c *gc.C) {
	s.testLeaveScope(c, s.uniter)
}

func (s *uniterV0Suite) TestJoinedRelations(c *gc.C) {
	s.testJoinedRelations(c, s.uniter)
}

func (s *uniterV0Suite) TestReadSettings(c *gc.C) {
	s.testReadSettings(c, s.uniter)
}

func (s *uniterV0Suite) TestReadSettingsWithNonStringValuesFails(c *gc.C) {
	s.testReadSettingsWithNonStringValuesFails(c, s.uniter)
}

func (s *uniterV0Suite) TestReadRemoteSettings(c *gc.C) {
	s.testReadRemoteSettings(c, s.uniter)
}

func (s *uniterV0Suite) TestReadRemoteSettingsWithNonStringValuesFails(c *gc.C) {
	s.testReadRemoteSettingsWithNonStringValuesFails(c, s.uniter)
}

func (s *uniterV0Suite) TestUpdateSettings(c *gc.C) {
	s.testUpdateSettings(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchRelationUnits(c *gc.C) {
	s.testWatchRelationUnits(c, s.uniter)
}

func (s *uniterV0Suite) TestAPIAddresses(c *gc.C) {
	s.testAPIAddresses(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchUnitAddresses(c *gc.C) {
	s.testWatchUnitAddresses(c, s.uniter)
}

func (s *uniterV0Suite) TestGetMeterStatus(c *gc.C) {
	s.testGetMeterStatus(c, s.uniter)
}

func (s *uniterV0Suite) TestGetMeterStatusUnauthenticated(c *gc.C) {
	s.testGetMeterStatusUnauthenticated(c, s.uniter)
}

func (s *uniterV0Suite) TestGetMeterStatusBadTag(c *gc.C) {
	s.testGetMeterStatusBadTag(c, s.uniter)
}

func (s *uniterV0Suite) TestWatchMeterStatus(c *gc.C) {
	s.testWatchMeterStatus(c, s.uniter)
}

func (s *uniterV0Suite) TestGetOwnerTag(c *gc.C) {
	tag := s.mysql.Tag().String()
	args := params.Entities{Entities: []params.Entity{
		{Tag: tag},
	}}
	result, err := s.uniter.GetOwnerTag(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{
		Result: s.AdminUserTag(c).String(),
	})
}

func (s *uniterV0Suite) TestServiceOwnerV0NotImplemented(c *gc.C) {
	apiservertesting.AssertNotImplemented(c, s.uniter, "ServiceOwner")
}

func (s *uniterV0Suite) TestAssignedMachineV0NotImplemented(c *gc.C) {
	apiservertesting.AssertNotImplemented(c, s.uniter, "AssignedMachine")
}

func (s *uniterV0Suite) TestAllMachinePortsV0NotImplemented(c *gc.C) {
	apiservertesting.AssertNotImplemented(c, s.uniter, "AllMachinePorts")
}

func (s *uniterV0Suite) TestRequestReboot(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machine0.Tag().String()},
		{Tag: s.machine1.Tag().String()},
		{Tag: "bogus"},
		{Tag: "nasty-tag"},
	}}
	errResult, err := s.uniter.RequestReboot(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		}})

	rFlag, err := s.machine0.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsTrue)

	rFlag, err = s.machine1.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsFalse)
}
