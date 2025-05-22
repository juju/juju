// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/firewaller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

func TestRemoteFirewallerSuite(t *testing.T) {
	tc.Run(t, &RemoteFirewallerSuite{})
}

type RemoteFirewallerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *MockState

	controllerConfigAPI *MockControllerConfigAPI
	watcherRegistry     *facademocks.MockWatcherRegistry
	api                 *firewaller.FirewallerAPI

	controllerConfigService *MockControllerConfigService
	modelConfigService      *MockModelConfigService
	networkService          *MockNetworkService
	applicationService      *MockApplicationService
	machineService          *MockMachineService
	modelInfoService        *MockModelInfoService
}

func (s *RemoteFirewallerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
}

func (s *RemoteFirewallerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.controllerConfigAPI = NewMockControllerConfigAPI(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)

	return ctrl
}

func (s *RemoteFirewallerSuite) setupAPI(c *tc.C) {
	var err error
	s.api, err = firewaller.NewStateFirewallerAPI(
		s.st,
		s.networkService,
		s.resources,
		s.watcherRegistry,
		s.authorizer,
		s.controllerConfigAPI,
		s.controllerConfigService,
		s.modelConfigService,
		s.applicationService,
		s.machineService,
		s.modelInfoService,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *RemoteFirewallerSuite) TestWatchIngressAddressesForRelations(c *tc.C) {
	c.Skip("Re-enable this test whenever CMR will be fully implemented and the related watcher rewired.")
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	db2Relation := newMockRelation(123)
	s.st.EXPECT().KeyRelation("remote-db2:db django:db").Return(db2Relation, nil)

	result, err := s.api.WatchIngressAddressesForRelations(
		c.Context(),
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewRelationTag("remote-db2:db django:db").String(),
		}}},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Changes, tc.SameContents, []string{"1.2.3.4/32"})
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, tc.Equals, "1")

	resource := s.resources.Get("1")
	c.Assert(resource, tc.NotNil)
	c.Assert(resource, tc.Implements, new(state.StringsWatcher))
}

func (s *RemoteFirewallerSuite) TestMacaroonForRelations(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	mac, err := jujutesting.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
	entity := names.NewRelationTag("mysql:db wordpress:db")
	s.st.EXPECT().GetMacaroon(entity).Return(mac, nil)

	result, err := s.api.MacaroonForRelations(
		c.Context(),
		params.Entities{Entities: []params.Entity{{
			Tag: entity.String(),
		}}},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.DeepEquals, mac)
}

func (s *RemoteFirewallerSuite) TestSetRelationStatus(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	db2Relation := newMockRelation(123)
	entity := names.NewRelationTag("remote-db2:db django:db")

	s.st.EXPECT().KeyRelation("remote-db2:db django:db").Return(db2Relation, nil)

	result, err := s.api.SetRelationsStatus(
		c.Context(),
		params.SetStatus{Entities: []params.EntityStatusArgs{{
			Tag:    entity.String(),
			Status: "suspended",
			Info:   "a message",
		}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(db2Relation.status, tc.DeepEquals, status.StatusInfo{Status: status.Suspended, Message: "a message"})
}
func TestFirewallerSuite(t *testing.T) {
	tc.Run(t, &FirewallerSuite{})
}

type FirewallerSuite struct {
	coretesting.BaseSuite

	authorizer *apiservertesting.FakeAuthorizer

	st *MockState

	controllerConfigAPI *MockControllerConfigAPI
	watcherRegistry     *facademocks.MockWatcherRegistry
	api                 *firewaller.FirewallerAPI

	controllerConfigService *MockControllerConfigService
	modelConfigService      *MockModelConfigService
	networkService          *MockNetworkService
	applicationService      *MockApplicationService
	machineService          *MockMachineService
	modelInfoService        *MockModelInfoService
}

func (s *FirewallerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
}

func (s *FirewallerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.controllerConfigAPI = NewMockControllerConfigAPI(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)

	return ctrl
}

func (s *FirewallerSuite) setupAPI(c *tc.C) {
	var err error
	s.api, err = firewaller.NewStateFirewallerAPI(
		s.st,
		s.networkService,
		nil,
		s.watcherRegistry,
		s.authorizer,
		s.controllerConfigAPI,
		s.controllerConfigService,
		s.modelConfigService,
		s.applicationService,
		s.machineService,
		s.modelInfoService,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *FirewallerSuite) TestModelFirewallRules(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.NewConfig(coretesting.ControllerTag.Id(), coretesting.CACert, map[string]interface{}{}))

	modelAttrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		config.SSHAllowKey: "192.168.0.0/24,192.168.1.0/24",
	})
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(config.New(config.UseDefaults, modelAttrs))

	s.modelInfoService.EXPECT().IsControllerModel(gomock.Any()).Return(
		false, nil,
	)

	rules, err := s.api.ModelFirewallRules(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.DeepEquals, params.IngressRulesResult{Rules: []params.IngressRule{{
		PortRange:   params.FromNetworkPortRange(network.MustParsePortRange("22")),
		SourceCIDRs: []string{"192.168.0.0/24", "192.168.1.0/24"},
	}}})
}

func (s *FirewallerSuite) TestModelFirewallRulesController(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	ctrlAttrs := map[string]interface{}{
		controller.APIPort:            17777,
		controller.AutocertDNSNameKey: "example.com",
	}
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.NewConfig(coretesting.ControllerTag.Id(), coretesting.CACert, ctrlAttrs))

	modelAttrs := coretesting.FakeConfig().Merge(map[string]interface{}{
		config.SSHAllowKey: "192.168.0.0/24,192.168.1.0/24",
	})
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(config.New(config.UseDefaults, modelAttrs))
	s.modelInfoService.EXPECT().IsControllerModel(gomock.Any()).Return(
		true, nil,
	)
	rules, err := s.api.ModelFirewallRules(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.DeepEquals, params.IngressRulesResult{Rules: []params.IngressRule{{
		PortRange:   params.FromNetworkPortRange(network.MustParsePortRange("22")),
		SourceCIDRs: []string{"192.168.0.0/24", "192.168.1.0/24"},
	}, {
		PortRange:   params.FromNetworkPortRange(network.MustParsePortRange("17777")),
		SourceCIDRs: []string{"0.0.0.0/0", "::/0"},
	}, {
		PortRange:   params.FromNetworkPortRange(network.MustParsePortRange("80")),
		SourceCIDRs: []string{"0.0.0.0/0", "::/0"},
	}}})
}

func (s *FirewallerSuite) TestWatchModelFirewallRules(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	ch := make(chan []string, 1)
	// initial event
	ch <- []string{}
	w := watchertest.NewMockStringsWatcher(ch)

	s.modelConfigService.EXPECT().Watch().Return(w, nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(config.New(config.UseDefaults, coretesting.FakeConfig()))

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	result, err := s.api.WatchModelFirewallRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.NotifyWatcherId, tc.Equals, "1")
}

func (s *FirewallerSuite) TestAllSpaceInfos(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	// Set up our mocks
	spaceInfos := network.SpaceInfos{
		{
			ID:         "42",
			Name:       "questions-about-the-universe",
			ProviderId: "provider-id-2",
			Subnets: []network.SubnetInfo{
				{
					ID:                "13",
					CIDR:              "1.168.1.0/24",
					ProviderId:        "provider-subnet-id-1",
					ProviderSpaceId:   "provider-space-id-1",
					ProviderNetworkId: "provider-network-id-1",
					VLANTag:           42,
					AvailabilityZones: []string{"az1", "az2"},
					SpaceID:           "42",
					SpaceName:         "questions-about-the-universe",
				},
			}},
		{ID: "99", Name: "special", Subnets: []network.SubnetInfo{
			{ID: "999", CIDR: "192.168.2.0/24"},
		}},
	}
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(spaceInfos, nil)

	// Test call output
	req := params.SpaceInfosParams{
		FilterBySpaceIDs: []string{network.AlphaSpaceId, "42"},
	}
	res, err := s.api.SpaceInfos(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)

	// Hydrate a network.SpaceInfos from the response
	gotSpaceInfos := params.ToNetworkSpaceInfos(res)
	c.Assert(gotSpaceInfos, tc.DeepEquals, spaceInfos[0:1], tc.Commentf("expected to get back a filtered list of the space infos"))
}

func (s *FirewallerSuite) TestWatchSubnets(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.setupAPI(c)

	ch := make(chan []string, 1)
	ch <- []string{"0195847b-95bb-7ca1-a7ee-2211d802d5b3"}
	w := watchertest.NewMockStringsWatcher(ch)
	s.networkService.EXPECT().WatchSubnets(gomock.Any(), set.NewStrings("0195847b-95bb-7ca1-a7ee-2211d802d5b3")).Return(w, nil)

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	entities := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewSubnetTag("0195847b-95bb-7ca1-a7ee-2211d802d5b3").String(),
		}},
	}
	got, err := s.api.WatchSubnets(c.Context(), entities)
	c.Assert(err, tc.ErrorIsNil)
	want := params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0195847b-95bb-7ca1-a7ee-2211d802d5b3"},
	}
	c.Assert(got.StringsWatcherId, tc.Equals, want.StringsWatcherId)
	c.Assert(got.Changes, tc.SameContents, want.Changes)
}
