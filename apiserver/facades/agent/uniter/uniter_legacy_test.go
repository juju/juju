// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	domainapplication "github.com/juju/juju/domain/application"
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

var _ = gc.Suite(&uniterLegacySuite{})

func (s *uniterLegacySuite) SetUpTest(c *gc.C) {
	c.Skip("Skip factory-based uniter tests. TODO: Re-write without factories")

	s.uniterSuiteBase.SetUpTest(c)
	s.domainServices = s.ControllerDomainServices(c)

	s.machineService = s.domainServices.Machine()
	s.applicationService = s.domainServices.Application()
	s.portService = s.domainServices.Port()
}

func (s *uniterLegacySuite) controllerConfig(c *gc.C) (controller.Config, error) {
	controllerDomainServices := s.ControllerDomainServices(c)
	controllerConfigService := controllerDomainServices.ControllerConfig()
	return controllerConfigService.ControllerConfig(context.Background())
}

func (s *uniterLegacySuite) TestUniterFailsWithNonUnitAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("9")
	ctx := s.facadeContext(c)
	ctx.Auth_ = anAuthorizer
	_, err := uniter.NewUniterAPI(context.Background(), ctx)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *uniterLegacySuite) TestLife(c *gc.C) {
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
	result, err := s.uniter.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
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

func (s *uniterLegacySuite) TestEnsureDead(c *gc.C) {
}

func (s *uniterLegacySuite) TestWatch(c *gc.C) {
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
	result, err := uniterAPI.Watch(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
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

func (s *uniterLegacySuite) TestWatchNoArgsNoError(c *gc.C) {
	uniterAPI := s.newUniterAPIv19(c, s.ControllerModel(c).State(), s.authorizer)
	result, err := uniterAPI.Watch(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}

func (s *uniterLegacySuite) TestApplicationWatch(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	// Recreate the uniter API with the mocks initialized.
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	args := params.Entity{Tag: "application-wordpress"}
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	result, err := uniterAPI.WatchApplication(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})
}

func (s *uniterLegacySuite) TestWatchApplicationBadTag(c *gc.C) {
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	result, err := uniterAPI.WatchApplication(context.Background(), params.Entity{Tag: "bad-tag"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestWatchApplicationNoPermission(c *gc.C) {
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	// Permissions for mysql will be denied by the accessApplication function
	// defined in test set up.
	result, err := uniterAPI.WatchApplication(context.Background(), params.Entity{Tag: "application-mysql"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestUnitWatch(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	// Recreate the uniter API with the mocks initialized.
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	args := params.Entity{Tag: "unit-wordpress-0"}
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	result, err := uniterAPI.WatchUnit(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})
}

func (s *uniterLegacySuite) TestWatchUnitBadTag(c *gc.C) {
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	result, err := uniterAPI.WatchUnit(context.Background(), params.Entity{Tag: "bad-tag"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestWatchUnitNoPermission(c *gc.C) {
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	// Permissions for mysql will be denied by the accessUnit function
	// defined in test set up.
	result, err := uniterAPI.WatchUnit(context.Background(), params.Entity{Tag: "unit-mysql-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestPublicAddress(c *gc.C) {
}

func (s *uniterLegacySuite) TestPrivateAddress(c *gc.C) {
}

func (s *uniterLegacySuite) TestResolvedAPIV6(c *gc.C) {
}

func (s *uniterLegacySuite) TestClearResolved(c *gc.C) {
}

func (s *uniterLegacySuite) TestGetPrincipal(c *gc.C) {
	// Add a subordinate to wordpressUnit.
	// First try it as wordpressUnit's agent.
	// Now try as subordinate's agent.
}

func (s *uniterLegacySuite) TestHasSubordinates(c *gc.C) {
	// Try first without any subordinates for wordpressUnit.
	// Add two subordinates to wordpressUnit and try again.
}

func (s *uniterLegacySuite) TestDestroy(c *gc.C) {
	// Verify wordpressUnit is destroyed and removed.
}

func (s *uniterLegacySuite) TestDestroyAllSubordinates(c *gc.C) {
	// Add two subordinates to wordpressUnit.
	// Verify wordpressUnit's subordinates were destroyed.
}

func (s *uniterLegacySuite) TestCharmURL(c *gc.C) {
	// Set wordpressUnit's charm URL first.
	// Make sure wordpress application's charm is what we expect.
}

func (s *uniterLegacySuite) TestSetCharmURL(c *gc.C) {
}

func (s *uniterLegacySuite) TestWorkloadVersion(c *gc.C) {
}

func (s *uniterLegacySuite) TestSetWorkloadVersion(c *gc.C) {
}

func (s *uniterLegacySuite) TestCharmModifiedVersion(c *gc.C) {
}

func (s *uniterLegacySuite) TestWatchConfigSettingsHash(c *gc.C) {
}

func (s *uniterLegacySuite) TestWatchTrustConfigSettingsHash(c *gc.C) {
}

func (s *uniterLegacySuite) TestLogActionMessage(c *gc.C) {
}

func (s *uniterLegacySuite) TestLogActionMessageAborting(c *gc.C) {
}

func (s *uniterLegacySuite) TestWatchActionNotifications(c *gc.C) {
}

func (s *uniterLegacySuite) TestWatchPreexistingActions(c *gc.C) {
}

func (s *uniterLegacySuite) TestWatchActionNotificationsMalformedTag(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "ewenit-mysql-0"},
	}}
	results, err := s.uniter.WatchActionNotifications(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, `invalid actionreceiver tag "ewenit-mysql-0"`)
}

func (s *uniterLegacySuite) TestWatchActionNotificationsMalformedUnitName(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-01"},
	}}
	results, err := s.uniter.WatchActionNotifications(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, `invalid actionreceiver tag "unit-mysql-01"`)
}

func (s *uniterLegacySuite) TestWatchActionNotificationsNotUnit(c *gc.C) {
}

func (s *uniterLegacySuite) TestWatchActionNotificationsPermissionDenied(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-nonexistentgarbage-0"},
	}}
	results, err := s.uniter.WatchActionNotifications(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, "permission denied")
}

func (s *uniterLegacySuite) TestConfigSettings(c *gc.C) {
}

func (s *uniterLegacySuite) TestCurrentModel(c *gc.C) {
	model := s.ControllerModel(c)
	result, err := s.uniter.CurrentModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	expected := params.ModelResult{
		Name: model.Name(),
		UUID: model.UUID(),
		Type: "iaas",
	}
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *uniterLegacySuite) TestActions(c *gc.C) {
}

func (s *uniterLegacySuite) TestActionsNotPresent(c *gc.C) {
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewActionTag(uuid.String()).String(),
		}},
	}
	results, err := s.uniter.Actions(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	actionsQueryResult := results.Results[0]
	c.Assert(actionsQueryResult.Error, gc.NotNil)
	c.Assert(actionsQueryResult.Error, gc.ErrorMatches, `action "[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}" not found`)
}

func (s *uniterLegacySuite) TestActionsWrongUnit(c *gc.C) {
	// Action doesn't match unit.
}

func (s *uniterLegacySuite) TestActionsPermissionDenied(c *gc.C) {
}

func (s *uniterLegacySuite) TestFinishActionsSuccess(c *gc.C) {
}

func (s *uniterLegacySuite) TestFinishActionsFailure(c *gc.C) {
}

func (s *uniterLegacySuite) TestFinishActionsAuthAccess(c *gc.C) {
	// Queue up actions from tests

	// Invoke FinishActions

	// Verify permissions errors for actions queued on different unit
}

func (s *uniterLegacySuite) TestBeginActions(c *gc.C) {
}

func (s *uniterLegacySuite) TestProviderType(c *gc.C) {
	modelInfo, err := s.ControllerDomainServices(c).ModelInfo().GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.ProviderType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{Result: modelInfo.CloudType})
}

func (s *uniterLegacySuite) TestWatchRelationUnits(c *gc.C) {
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

func (s *uniterLegacySuite) TestAPIAddresses(c *gc.C) {
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}

	controllerConfig, err := s.controllerConfig(c)
	c.Assert(err, jc.ErrorIsNil)

	st := s.ControllerModel(c).State()
	err = st.SetAPIHostPorts(controllerConfig, hostPorts, hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.APIAddresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *uniterLegacySuite) TestWatchUnitAddressesHash(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-wordpress"},
	}}
	result, err := s.uniter.WatchUnitAddressesHash(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
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
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterLegacySuite) TestWatchCAASUnitAddressesHash(c *gc.C) {
	_, cm, _, _ := s.setupCAASModel(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-gitlab-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-gitlab"},
	}}

	uniterAPI := s.newUniterAPI(c, cm.State(), s.authorizer)

	result, err := uniterAPI.WatchUnitAddressesHash(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
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
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterLegacySuite) TestStorageAttachments(c *gc.C) {
	// We need to set up a unit that has storage metadata defined.
}

func (s *uniterLegacySuite) TestOpenedMachinePortRangesByEndpoint(c *gc.C) {
	_, err := s.machineService.CreateMachine(context.Background(), "0")
	c.Assert(err, jc.ErrorIsNil)

	err = s.applicationService.AddUnits(context.Background(), "mysql", domainapplication.StorageParentDir,
		applicationservice.AddUnitArg{
			UnitName: "mysql/1",
		})
	c.Assert(err, jc.ErrorIsNil)

	wordpressUnitUUID, err := s.applicationService.GetUnitUUID(context.Background(), "wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	mysqlUnitUUID, err := s.applicationService.GetUnitUUID(context.Background(), "mysql/1")
	c.Assert(err, jc.ErrorIsNil)

	// Open some ports on both units using different endpoints.
	err = s.portService.UpdateUnitPorts(context.Background(), wordpressUnitUUID, network.GroupedPortRanges{
		allEndpoints:      []network.PortRange{network.MustParsePortRange("100-200/tcp")},
		"monitoring-port": []network.PortRange{network.MustParsePortRange("10-20/udp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	err = s.portService.UpdateUnitPorts(context.Background(), mysqlUnitUUID, network.GroupedPortRanges{
		"server": []network.PortRange{network.MustParsePortRange("3306/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

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
	result, err := s.uniter.OpenedMachinePortRangesByEndpoint(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.OpenPortRangesByEndpointResults{
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

func (s *uniterLegacySuite) TestPrivateAddressWithRemoteRelation(c *gc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR preferring private addresses. ")
}

func (s *uniterLegacySuite) TestPrivateAddressWithRemoteRelationNoPublic(c *gc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR when unit does not have " +
		"a public addresses. ")
}

func (s *uniterLegacySuite) TestRelationEgressSubnets(c *gc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR. Use NewRelationEgressNetworks " +
		"to set different egress networks from the model config. ")
}

func (s *uniterLegacySuite) TestModelEgressSubnets(c *gc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR. Egress networks are set " +
		"via model config.")
}

func (s *uniterLegacySuite) makeMysqlUniter(c *gc.C) *uniter.UniterAPI {
	return nil
}

func (s *uniterLegacySuite) TestRefresh(c *gc.C) {
}

func (s *uniterLegacySuite) TestRefreshNoArgs(c *gc.C) {
	results, err := s.uniter.Refresh(context.Background(), params.Entities{Entities: []params.Entity{}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UnitRefreshResults{Results: []params.UnitRefreshResult{}})
}

func (s *uniterLegacySuite) TestOpenedPortRangesByEndpoint(c *gc.C) {
	unitUUID, err := s.applicationService.GetUnitUUID(context.Background(), "mysql/0")
	c.Assert(err, jc.ErrorIsNil)

	err = s.portService.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		allEndpoints: []network.PortRange{network.MustParsePortRange("1000/tcp")},
		"db":         []network.PortRange{network.MustParsePortRange("1111/udp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

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

	result, err := uniterAPI.OpenedPortRangesByEndpoint(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.OpenPortRangesByEndpointResults{
		Results: []params.OpenPortRangesByEndpointResult{
			{
				UnitPortRanges: map[string][]params.OpenUnitPortRangesByEndpoint{
					"unit-mysql-0": expectPortRanges,
				},
			},
		},
	})
}

func (s *uniterLegacySuite) TestCommitHookChangesWithSecrets(c *gc.C) {
	c.Skip("Rewrite this in the commitHookChangesSuite once other hook commit concerns are in Dqlite")
	// See commitHookChangesSuite
}

func (s *uniterLegacySuite) TestCommitHookChangesWithStorage(c *gc.C) {
	c.Skip("Rewrite this in the commitHookChangesSuite once other hook commit concerns are in Dqlite")

	// Test-suite uses an older API version. Create a new one and override
	// authorizer to allow access to the unit we just created.

	// Verify state
}

func (s *uniterLegacySuite) TestCommitHookChangesWithPortsSidecarApplication(c *gc.C) {
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
	uniterAPI, err := uniter.NewUniterAPI(context.Background(), facadetest.ModelContext{
		State_:             cm.State(),
		StatePool_:         s.StatePool(),
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.leadershipChecker,
		DomainServices_:    s.DefaultModelDomainServices(c),
		Logger_:            loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := uniterAPI.CommitHookChanges(context.Background(), req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	unitUUID, err := s.applicationService.GetUnitUUID(context.Background(), coreunit.Name(unit.Tag().Id()))
	c.Assert(err, jc.ErrorIsNil)
	portRanges, err := s.portService.GetUnitOpenedPorts(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(portRanges, jc.DeepEquals, network.GroupedPortRanges{
		"db": []network.PortRange{network.MustParsePortRange("80/tcp")},
	})
}

func (s *uniterNetworkInfoSuite) TestCommitHookChangesCAAS(c *gc.C) {
}

func (s *uniterLegacySuite) TestNetworkInfoCAASModelRelation(c *gc.C) {
}

func (s *uniterLegacySuite) TestNetworkInfoCAASModelNoRelation(c *gc.C) {
}

func (s *uniterLegacySuite) TestGetCloudSpecDeniesAccessWhenNotTrusted(c *gc.C) {
	result, err := s.uniter.CloudSpec(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.CloudSpecResult{Error: apiservertesting.ErrUnauthorized})
}
