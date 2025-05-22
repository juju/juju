// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationservice "github.com/juju/juju/domain/application/service"
	machineservice "github.com/juju/juju/domain/machine/service"
	portservice "github.com/juju/juju/domain/port/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

const allEndpoints = ""

type uniterLegacySuite struct {
	uniterSuiteBase
	domainServices     services.DomainServices
	machineService     *machineservice.WatchableService
	applicationService *applicationservice.WatchableService
	portService        *portservice.WatchableService
}

func TestUniterLegacySuite(t *testing.T) {
	tc.Run(t, &uniterLegacySuite{})
}

func (s *uniterLegacySuite) SetUpSuite(c *tc.C) {
	c.Skip("Skip factory-based uniter tests. TODO: Re-write without factories")
}

func (s *uniterLegacySuite) SetUpTest(c *tc.C) {
	s.uniterSuiteBase.SetUpTest(c)
	s.domainServices = s.ControllerDomainServices(c)

	s.machineService = s.domainServices.Machine()
	s.applicationService = s.domainServices.Application()
	s.portService = s.domainServices.Port()
}

func (s *uniterLegacySuite) controllerConfig(c *tc.C) (controller.Config, error) {
	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	return controllerConfigService.ControllerConfig(c.Context())
}

func (s *uniterLegacySuite) TestUniterFailsWithNonUnitAgentUser(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("9")
	ctx := s.facadeContext(c)
	ctx.Auth_ = anAuthorizer
	_, err := uniter.NewUniterAPI(c.Context(), ctx)
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *uniterLegacySuite) TestLife(c *tc.C) {
	// Add a relation wordpress-mysql.
	// Make the wordpressUnit dead.
	// Add another unit, so the service will stay dying when we destroy it later.
	// Make the wordpress application dying.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "application-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
		// {Tag: rel.Tag().String()},
		{Tag: "relation-svc1.rel1#svc2.rel2"},
		// {Tag: "relation-blah"},
	}}
	result, err := s.uniter.Life(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dying"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// TODO(dfc) see above
			// {Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterLegacySuite) TestEnsureDead(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatch(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	// Recreate the uniter API with the mocks initialized.
	uniterAPI := s.newUniterAPIv19(c, s.ControllerModel(c).State(), s.authorizer)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "application-foo"},
	}}
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("2", nil)
	result, err := uniterAPI.Watch(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "2"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterLegacySuite) TestWatchNoArgsNoError(c *tc.C) {
	uniterAPI := s.newUniterAPIv19(c, s.ControllerModel(c).State(), s.authorizer)
	result, err := uniterAPI.Watch(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

func (s *uniterLegacySuite) TestApplicationWatch(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	// Recreate the uniter API with the mocks initialized.
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	args := params.Entity{Tag: "application-wordpress"}
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	result, err := uniterAPI.WatchApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})
}

func (s *uniterLegacySuite) TestWatchApplicationBadTag(c *tc.C) {
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	result, err := uniterAPI.WatchApplication(c.Context(), params.Entity{Tag: "bad-tag"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestWatchApplicationNoPermission(c *tc.C) {
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	// Permissions for mysql will be denied by the accessApplication function
	// defined in test set up.
	result, err := uniterAPI.WatchApplication(c.Context(), params.Entity{Tag: "application-mysql"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestUnitWatch(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	// Recreate the uniter API with the mocks initialized.
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	args := params.Entity{Tag: "unit-wordpress-0"}
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	result, err := uniterAPI.WatchUnit(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})
}

func (s *uniterLegacySuite) TestWatchUnitBadTag(c *tc.C) {
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	result, err := uniterAPI.WatchUnit(c.Context(), params.Entity{Tag: "bad-tag"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestWatchUnitNoPermission(c *tc.C) {
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	// Permissions for mysql will be denied by the accessUnit function
	// defined in test set up.
	result, err := uniterAPI.WatchUnit(c.Context(), params.Entity{Tag: "unit-mysql-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestPublicAddress(c *tc.C) {
}

func (s *uniterLegacySuite) TestPrivateAddress(c *tc.C) {
}

func (s *uniterLegacySuite) TestResolvedAPIV6(c *tc.C) {
}

func (s *uniterLegacySuite) TestClearResolved(c *tc.C) {
}

func (s *uniterLegacySuite) TestGetPrincipal(c *tc.C) {
	// Add a subordinate to wordpressUnit.
	// First try it as wordpressUnit's agent.
	// Now try as subordinate's agent.
}

func (s *uniterLegacySuite) TestDestroy(c *tc.C) {
	// Verify wordpressUnit is destroyed and removed.
}

func (s *uniterLegacySuite) TestDestroyAllSubordinates(c *tc.C) {
	// Add two subordinates to wordpressUnit.
	// Verify wordpressUnit's subordinates were destroyed.
}

func (s *uniterLegacySuite) TestWorkloadVersion(c *tc.C) {
}

func (s *uniterLegacySuite) TestSetWorkloadVersion(c *tc.C) {
}

func (s *uniterLegacySuite) TestCharmModifiedVersion(c *tc.C) {
}

func (s *uniterLegacySuite) TestLogActionMessage(c *tc.C) {
}

func (s *uniterLegacySuite) TestLogActionMessageAborting(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatchActionNotifications(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatchPreexistingActions(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatchActionNotificationsMalformedTag(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "ewenit-mysql-0"},
	}}
	results, err := s.uniter.WatchActionNotifications(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.NotNil)
	c.Assert(len(results.Results), tc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.NotNil)
	c.Assert(result.Error.Message, tc.Equals, `invalid actionreceiver tag "ewenit-mysql-0"`)
}

func (s *uniterLegacySuite) TestWatchActionNotificationsMalformedUnitName(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-01"},
	}}
	results, err := s.uniter.WatchActionNotifications(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.NotNil)
	c.Assert(len(results.Results), tc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.NotNil)
	c.Assert(result.Error.Message, tc.Equals, `invalid actionreceiver tag "unit-mysql-01"`)
}

func (s *uniterLegacySuite) TestWatchActionNotificationsNotUnit(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatchActionNotificationsPermissionDenied(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-nonexistentgarbage-0"},
	}}
	results, err := s.uniter.WatchActionNotifications(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.NotNil)
	c.Assert(len(results.Results), tc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.NotNil)
	c.Assert(result.Error.Message, tc.Equals, "permission denied")
}

func (s *uniterLegacySuite) TestCurrentModel(c *tc.C) {
	result, err := s.uniter.CurrentModel(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	modelInfo, err := s.ControllerDomainServices(c).ModelInfo().GetModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	expected := params.ModelResult{
		Name: modelInfo.Name,
		UUID: modelInfo.UUID.String(),
		Type: "iaas",
	}
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *uniterLegacySuite) TestActions(c *tc.C) {
}

func (s *uniterLegacySuite) TestActionsNotPresent(c *tc.C) {
	uuid, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewActionTag(uuid.String()).String(),
		}},
	}
	results, err := s.uniter.Actions(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results.Results, tc.HasLen, 1)
	actionsQueryResult := results.Results[0]
	c.Assert(actionsQueryResult.Error, tc.NotNil)
	c.Assert(actionsQueryResult.Error, tc.ErrorMatches, `action "[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}" not found`)
}

func (s *uniterLegacySuite) TestActionsWrongUnit(c *tc.C) {
	// Action doesn't match unit.
}

func (s *uniterLegacySuite) TestActionsPermissionDenied(c *tc.C) {
}

func (s *uniterLegacySuite) TestFinishActionsSuccess(c *tc.C) {
}

func (s *uniterLegacySuite) TestFinishActionsFailure(c *tc.C) {
}

func (s *uniterLegacySuite) TestFinishActionsAuthAccess(c *tc.C) {
	// Queue up actions from tests

	// Invoke FinishActions

	// Verify permissions errors for actions queued on different unit
}

func (s *uniterLegacySuite) TestBeginActions(c *tc.C) {
}

func (s *uniterLegacySuite) TestProviderType(c *tc.C) {
	modelInfo, err := s.ControllerDomainServices(c).ModelInfo().GetModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.uniter.ProviderType(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResult{Result: modelInfo.CloudType})
}

func (s *uniterLegacySuite) TestWatchRelationUnits(c *tc.C) {
	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.

	// UnitSettings versions are volatile, so we don't check them.
	// We just make sure the keys of the Changed field are as
	// expected.

	// Verify the resource was registered and stop when done

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)

	// Leave scope with mysqlUnit and check it's detected.

	// TODO(jam): 2019-10-21 this test is getting a bit unweildy, but maybe we
	//  should test that changing application data triggers a change here
}

func (s *uniterLegacySuite) TestAPIAddresses(c *tc.C) {
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}

	controllerConfig, err := s.controllerConfig(c)
	c.Assert(err, tc.ErrorIsNil)

	st := s.ControllerModel(c).State()
	err = st.SetAPIHostPorts(controllerConfig, hostPorts, hostPorts)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.uniter.APIAddresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *uniterLegacySuite) TestWatchUnitAddressesHash(c *tc.C) {
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-wordpress"},
	}}
	result, err := s.uniter.WatchUnitAddressesHash(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				// The unit's machine has no network addresses
				// so the expected hash only contains the
				// sorted endpoint to space ID bindings for the
				// wordpress application.
				Changes: []string{"6048d9d417c851eddf006fa5b5435549313ee3046cf45a8223f47244d8c73e03"},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterLegacySuite) TestWatchCAASUnitAddressesHash(c *tc.C) {
	_, cm, _, _ := s.setupCAASModel(c)
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-gitlab-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-gitlab"},
	}}

	uniterAPI := s.newUniterAPI(c, cm.State(), s.authorizer)

	result, err := uniterAPI.WatchUnitAddressesHash(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				// The container doesn't have an address.
				Changes: []string{""},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), tc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterLegacySuite) TestStorageAttachments(c *tc.C) {
	// We need to set up a unit that has storage metadata defined.
}

func (s *uniterLegacySuite) TestOpenedMachinePortRangesByEndpoint(c *tc.C) {
	_, err := s.machineService.CreateMachine(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.applicationService.AddUnits(c.Context(), "mysql",
		applicationservice.AddUnitArg{})
	c.Assert(err, tc.ErrorIsNil)

	wordpressUnitUUID, err := s.applicationService.GetUnitUUID(c.Context(), "wordpress/0")
	c.Assert(err, tc.ErrorIsNil)
	mysqlUnitUUID, err := s.applicationService.GetUnitUUID(c.Context(), "mysql/1")
	c.Assert(err, tc.ErrorIsNil)

	// Open some ports on both units using different endpoints.
	err = s.portService.UpdateUnitPorts(c.Context(), wordpressUnitUUID, network.GroupedPortRanges{
		allEndpoints:      []network.PortRange{network.MustParsePortRange("100-200/tcp")},
		"monitoring-port": []network.PortRange{network.MustParsePortRange("10-20/udp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	err = s.portService.UpdateUnitPorts(c.Context(), mysqlUnitUUID, network.GroupedPortRanges{
		"server": []network.PortRange{network.MustParsePortRange("3306/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	// Get the open port ranges
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-42"},
		{Tag: "application-wordpress"},
	}}
	expectPortRanges := map[string][]params.OpenUnitPortRangesByEndpoint{
		"unit-mysql-1": {
			{
				Endpoint:   "server",
				PortRanges: []params.PortRange{{FromPort: 3306, ToPort: 3306, Protocol: "tcp"}},
			},
		},
		"unit-wordpress-0": {
			{
				Endpoint:   "",
				PortRanges: []params.PortRange{{FromPort: 100, ToPort: 200, Protocol: "tcp"}},
			},
			{
				Endpoint:   "monitoring-port",
				PortRanges: []params.PortRange{{FromPort: 10, ToPort: 20, Protocol: "udp"}},
			},
		},
	}
	result, err := s.uniter.OpenedMachinePortRangesByEndpoint(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.OpenPortRangesByEndpointResults{
		Results: []params.OpenPortRangesByEndpointResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				UnitPortRanges: expectPortRanges,
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterLegacySuite) TestPrivateAddressWithRemoteRelation(c *tc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR preferring private addresses. ")
}

func (s *uniterLegacySuite) TestPrivateAddressWithRemoteRelationNoPublic(c *tc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR when unit does not have " +
		"a public addresses. ")
}

func (s *uniterLegacySuite) TestRelationEgressSubnets(c *tc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR. Use NewRelationEgressNetworks " +
		"to set different egress networks from the model config. ")
}

func (s *uniterLegacySuite) TestModelEgressSubnets(c *tc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR. Egress networks are set " +
		"via model config.")
}

func (s *uniterLegacySuite) makeMysqlUniter(c *tc.C) *uniter.UniterAPI {
	return nil
}

func (s *uniterLegacySuite) TestRefresh(c *tc.C) {
}

func (s *uniterLegacySuite) TestRefreshNoArgs(c *tc.C) {
	results, err := s.uniter.Refresh(c.Context(), params.Entities{Entities: []params.Entity{}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UnitRefreshResults{Results: []params.UnitRefreshResult{}})
}

func (s *uniterLegacySuite) TestOpenedPortRangesByEndpoint(c *tc.C) {
	unitUUID, err := s.applicationService.GetUnitUUID(c.Context(), "mysql/0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.portService.UpdateUnitPorts(c.Context(), unitUUID, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("1000/tcp")},
		"db":         []network.PortRange{network.MustParsePortRange("1111/udp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	// Get the open port ranges
	expectPortRanges := []params.OpenUnitPortRangesByEndpoint{
		{
			Endpoint:   "",
			PortRanges: []params.PortRange{{FromPort: 1000, ToPort: 1000, Protocol: "tcp"}},
		},
		{
			Endpoint:   "db",
			PortRanges: []params.PortRange{{FromPort: 1111, ToPort: 1111, Protocol: "udp"}},
		},
	}

	uniterAPI := s.makeMysqlUniter(c)

	result, err := uniterAPI.OpenedPortRangesByEndpoint(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.OpenPortRangesByEndpointResults{
		Results: []params.OpenPortRangesByEndpointResult{
			{
				UnitPortRanges: map[string][]params.OpenUnitPortRangesByEndpoint{
					"unit-mysql-0": expectPortRanges,
				},
			},
		},
	})
}

func (s *uniterLegacySuite) TestCommitHookChangesWithSecrets(c *tc.C) {
	c.Skip("Rewrite this in the commitHookChangesSuite once other hook commit concerns are in Dqlite")
	// See commitHookChangesSuite
}

func (s *uniterLegacySuite) TestCommitHookChangesWithStorage(c *tc.C) {
	c.Skip("Rewrite this in the commitHookChangesSuite once other hook commit concerns are in Dqlite")

	// Test-suite uses an older API version. Create a new one and override
	// authorizer to allow access to the unit we just created.

	// Verify state
}

func (s *uniterLegacySuite) TestCommitHookChangesWithPortsSidecarApplication(c *tc.C) {
	c.Skip("Rewrite this in the commitHookChangesSuite other hook commit concerns are in Dqlite")
	_, cm, _, unit := s.setupCAASModel(c)

	b := apiuniter.NewCommitHookParamsBuilder(unit.UnitTag())
	b.UpdateNetworkInfo()
	b.UpdateCharmState(map[string]string{"charm-key": "charm-value"})

	b.OpenPortRange("db", network.MustParsePortRange("80/tcp"))
	b.OpenPortRange("db", network.MustParsePortRange("7337/tcp")) // same port closed below; this should be a no-op
	b.ClosePortRange("db", network.MustParsePortRange("7337/tcp"))
	req, _ := b.Build()

	s.authorizer = apiservertesting.FakeAuthorizer{Tag: unit.Tag()}
	uniterAPI, err := uniter.NewUniterAPI(c.Context(), facadetest.ModelContext{
		State_:             cm.State(),
		StatePool_:         s.StatePool(),
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.leadershipChecker,
		DomainServices_:    s.DefaultModelDomainServices(c),
		Logger_:            loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := uniterAPI.CommitHookChanges(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	unitUUID, err := s.applicationService.GetUnitUUID(c.Context(), coreunit.Name(unit.Tag().Id()))
	c.Assert(err, tc.ErrorIsNil)
	portRanges, err := s.portService.GetUnitOpenedPorts(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(portRanges, tc.DeepEquals, network.GroupedPortRanges{
		"db": []network.PortRange{network.MustParsePortRange("80/tcp")},
	})
}

func (s *uniterNetworkInfoSuite) TestCommitHookChangesCAAS(c *tc.C) {
}

func (s *uniterLegacySuite) TestNetworkInfoCAASModelRelation(c *tc.C) {
}

func (s *uniterLegacySuite) TestNetworkInfoCAASModelNoRelation(c *tc.C) {
}

func (s *uniterLegacySuite) TestGetCloudSpecDeniesAccessWhenNotTrusted(c *tc.C) {
	result, err := s.uniter.CloudSpec(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CloudSpecResult{Error: apiservertesting.ErrUnauthorized})
}
