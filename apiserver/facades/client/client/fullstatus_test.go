// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	domainnetwork "github.com/juju/juju/domain/network"
	service "github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/internal/testhelpers"
	internaluuid "github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type fullStatusSuite struct {
	testhelpers.IsolationSuite

	modelUUID model.UUID
	clock     clock.Clock

	authorizer *MockAuthorizer

	applicationService        *MockApplicationService
	blockDeviceService        *MockBlockDeviceService
	crossModelRelationService *MockCrossModelRelationService
	machineService            *MockMachineService
	modelInfoService          *MockModelInfoService
	networkService            *MockNetworkService
	portService               *MockPortService
	relationService           *MockRelationService
	statusService             *MockStatusService
}

func TestFullStatusSuite(t *testing.T) {
	tc.Run(t, &fullStatusSuite{})
}

func (s *fullStatusSuite) SetUpTest(c *tc.C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
	s.clock = testclock.NewClock(time.Now())
	c.Cleanup(func() {
		s.clock = nil
		s.modelUUID = ""
	})
}

// TestFullStatusOffersIncluded tests that network interfaces are included in the
// machine status.
func (s *fullStatusSuite) TestFullStatusNetworkInterfaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	client := s.client(false)
	s.expectCheckCanRead(client, true)

	s.modelInfoService.EXPECT().GetModelInfo(c.Context()).Return(model.ModelInfo{
		Cloud:     "dummy",
		CloudType: "dummy",
		Type:      model.IAAS,
	}, nil)

	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status:  status.Available,
		Message: "testing",
	}, nil)

	s.statusService.EXPECT().GetMachineFullStatuses(gomock.Any()).Return(map[machine.Name]service.Machine{
		"0": {
			Name:        "0",
			IPAddresses: []string{"172.16.0.0"},
			InstanceID:  "i-12345",
		},
	}, nil)

	macAddr := "aa:bb:cc:dd:ee:ff"
	gatewayAddr := "10.0.0.1"
	s.networkService.EXPECT().GetAllDevicesByMachineNames(gomock.Any()).Return(map[machine.Name][]domainnetwork.NetInterface{
		"0": {
			// eth0 has an empty mac and gateway address.
			{
				Name: "eth0",
				Addrs: []domainnetwork.NetAddr{
					{
						AddressValue: "172.16.0.0",
						AddressType:  "ipv4",
						Space:        "",
					},
				},
				MACAddress:     nil,
				GatewayAddress: nil,
				DNSAddresses:   nil,
				IsEnabled:      false,
			},
			// eth1 has a valid mac and gateway address.
			{
				Name: "eth1",
				Addrs: []domainnetwork.NetAddr{
					{
						AddressValue: "172.16.0.1",
						AddressType:  "ipv4",
						Space:        "space1",
					},
				},
				MACAddress:     &macAddr,
				GatewayAddress: &gatewayAddr,
				DNSAddresses:   []string{"8.8.8.8"},
				IsEnabled:      true,
			},
		},
	}, nil)

	s.applicationService.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(nil, nil)
	s.portService.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)
	s.crossModelRelationService.EXPECT().GetOffers(gomock.Any(), gomock.Any()).Return(nil, nil)

	s.expectCheckIsAdmin(client, true)

	// Act
	output, err := client.FullStatus(c.Context(), params.StatusParams{})

	// Assert
	c.Assert(err, tc.IsNil)
	machine0 := output.Machines["0"]

	c.Check(machine0.NetworkInterfaces, tc.DeepEquals,
		map[string]params.NetworkInterface{
			"eth0": {
				IPAddresses:    []string{"172.16.0.0"},
				MACAddress:     "",
				Gateway:        "",
				DNSNameservers: nil,
				Space:          "",
				IsUp:           false,
			},
			"eth1": {
				IPAddresses:    []string{"172.16.0.1"},
				MACAddress:     macAddr,
				Gateway:        gatewayAddr,
				DNSNameservers: []string{"8.8.8.8"},
				Space:          "space1",
				IsUp:           true,
			},
		},
	)
}

// TestFullStatusOffersIncluded tests that offers are included if the
// api user is a superuser or model admin.
func (s *fullStatusSuite) TestFullStatusOffersIncluded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	client := s.client(false)
	s.expectCheckCanRead(client, true)
	s.applicationService.EXPECT().GetUnitsK8sPodInfo(gomock.Any()).Return(nil, nil)

	s.modelInfoService.EXPECT().GetModelInfo(c.Context()).Return(model.ModelInfo{
		Cloud:     "k8s",
		CloudType: "k8s",
		Type:      model.CAAS, // skip fetching machines
	}, nil)

	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status:  status.Available,
		Message: "testing",
	}, nil)
	s.expectEmptyModelModuloOffers(c)

	// If this is a model admin, GetOffers.
	s.expectCheckIsAdmin(client, true)
	charmLocator := charm.CharmLocator{
		Name:         "app",
		Revision:     42,
		Source:       charm.CharmHubSource,
		Architecture: architecture.AMD64,
	}
	offerDetail := &crossmodelrelation.OfferDetail{
		OfferName:       "one",
		ApplicationName: "test-app",
		CharmLocator:    charmLocator,
		Endpoints: []crossmodelrelation.OfferEndpoint{
			{Name: "db"},
		},
	}

	s.crossModelRelationService.EXPECT().GetOffers(gomock.Any(), gomock.Any()).Return(
		[]*crossmodelrelation.OfferDetail{offerDetail}, nil)

	// Act
	output, err := client.FullStatus(c.Context(), params.StatusParams{})
	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(output.Offers, tc.DeepEquals,
		map[string]params.ApplicationOfferStatus{
			offerDetail.OfferName: {
				OfferName:       offerDetail.OfferName,
				ApplicationName: offerDetail.ApplicationName,
				Endpoints: map[string]params.RemoteEndpoint{
					"db": {
						Name: "db",
					},
				},
				CharmURL: "ch:amd64/app-42",
			},
		})
}

// TestFullStatusOffersIncluded tests that offers are not included when the
// api user only has read access on the model.
func (s *fullStatusSuite) TestFullStatusOffersNotIncluded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	client := s.client(false)
	s.expectCheckCanRead(client, true)
	// Not a model admin, GetOffers won't be called.
	s.expectCheckIsAdmin(client, false)

	s.modelInfoService.EXPECT().GetModelInfo(c.Context()).Return(model.ModelInfo{
		Cloud:     "k8s",
		CloudType: "k8s",
		Type:      model.CAAS, // skip fetching machines
	}, nil)
	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status:  status.Available,
		Message: "testing",
	}, nil)
	s.expectEmptyModelModuloOffers(c)
	s.applicationService.EXPECT().GetUnitsK8sPodInfo(gomock.Any()).Return(nil, nil)

	// Act
	output, err := client.FullStatus(c.Context(), params.StatusParams{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(output.Offers, tc.DeepEquals, map[string]params.ApplicationOfferStatus{})
}

func (s *fullStatusSuite) client(isControllerModel bool) *Client {
	return &Client{
		controllerTag:     names.NewControllerTag(internaluuid.MustNewUUID().String()),
		isControllerModel: isControllerModel,
		modelTag:          names.NewModelTag(s.modelUUID.String()),
		clock:             s.clock,
		auth:              s.authorizer,

		applicationService:        s.applicationService,
		blockDeviceService:        s.blockDeviceService,
		crossModelRelationService: s.crossModelRelationService,
		machineService:            s.machineService,
		modelInfoService:          s.modelInfoService,
		networkService:            s.networkService,
		portService:               s.portService,
		relationService:           s.relationService,
		statusService:             s.statusService,
	}
}

func (s *fullStatusSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = NewMockAuthorizer(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.blockDeviceService = NewMockBlockDeviceService(ctrl)
	s.crossModelRelationService = NewMockCrossModelRelationService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.portService = NewMockPortService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)

	c.Cleanup(func() {
		s.authorizer = nil
		s.applicationService = nil
		s.blockDeviceService = nil
		s.crossModelRelationService = nil
		s.machineService = nil
		s.modelInfoService = nil
		s.networkService = nil
		s.portService = nil
		s.relationService = nil
		s.statusService = nil
	})

	return ctrl
}

func (s *fullStatusSuite) expectCheckCanRead(client *Client, read bool) {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, client.controllerTag).Return(authentication.ErrorEntityMissingPermission)
	if read {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, client.modelTag).Return(nil)
	} else {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, client.modelTag).Return(authentication.ErrorEntityMissingPermission)
	}
}

func (s *fullStatusSuite) expectCheckIsAdmin(client *Client, read bool) {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, client.controllerTag).Return(authentication.ErrorEntityMissingPermission)
	if read {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, client.modelTag).Return(nil)
	} else {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, client.modelTag).Return(authentication.ErrorEntityMissingPermission)
	}
}

func (s *fullStatusSuite) expectEmptyModelModuloOffers(c *tc.C) {
	s.statusService.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(nil, nil)
	s.applicationService.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, nil)

	s.portService.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllDevicesByMachineNames(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)
}
