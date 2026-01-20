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
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	domainnetwork "github.com/juju/juju/domain/network"
	service "github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/domain/storage"
	internalerrors "github.com/juju/juju/internal/errors"
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
	s.modelUUID = tc.Must0(c, model.NewUUID)
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
						Space:        "alpha",
					},
					{
						AddressValue: "172.16.0.1",
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
						AddressValue: "3.16.0.1",
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
	s.statusService.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(nil, nil)
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
				IPAddresses:    []string{"172.16.0.0", "172.16.0.1"},
				MACAddress:     "",
				Gateway:        "",
				DNSNameservers: nil,
				Space:          "alpha",
				IsUp:           false,
			},
			"eth1": {
				IPAddresses:    []string{"3.16.0.1"},
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
		TotalActiveConnections: 1,
		TotalConnections:       2,
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
				CharmURL:             "ch:amd64/app-42",
				ActiveConnectedCount: 1,
				TotalConnectedCount:  2,
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

// TestFullStatusStoragePartialFailure tests that storage processing continues
// even when some storage instances fail (e.g. missing filesystem).
func (s *fullStatusSuite) TestFullStatusStoragePartialFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	client := s.client(false)
	s.expectCheckCanRead(client, true)
	// Not a model admin.
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

	// Mock Storage Instances
	s.statusService.EXPECT().GetStorageInstanceStatuses(gomock.Any()).Return([]service.StorageInstance{
		{
			ID:   "pgdata/0",
			Kind: storage.StorageKindFilesystem,
			Life: life.Alive,
		},
		{
			ID:   "pgdata/1",
			Kind: storage.StorageKindFilesystem,
			Life: life.Alive,
		},
	}, nil)

	// Mock Filesystem Statuses with one error
	s.statusService.EXPECT().GetFilesystemStatuses(gomock.Any()).Return(
		// Filesystems
		[]service.Filesystem{
			{
				ID:        "1",
				StorageID: "pgdata/1",
				Status:    status.StatusInfo{Status: status.Active},
				Life:      life.Alive,
			},
		},
		// Errors
		[]error{
			internalerrors.Errorf("filesystem for storage instance \"pgdata/0\" not found"),
		},
		nil,
	)

	// Mock Volume Statuses
	s.statusService.EXPECT().GetVolumeStatuses(gomock.Any()).Return(nil, nil, nil)

	// Act
	output, err := client.FullStatus(c.Context(), params.StatusParams{
		IncludeStorage: true,
	})

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify that we got results for both storage instances
	c.Assert(output.Storage, tc.HasLen, 2)

	// pgdata/0 (the one that failed filesystem lookup) should be present but maybe unknown status
	// The user wants it to have the ERROR message.
	// We expect the code to map the filesystem error back to the storage instance.

	var s0 params.StorageDetails
	var s1 params.StorageDetails
	for _, s := range output.Storage {
		if s.StorageTag == "storage-pgdata-0" {
			s0 = s
		} else if s.StorageTag == "storage-pgdata-1" {
			s1 = s
		}
	}

	c.Assert(s0.StorageTag, tc.Equals, "storage-pgdata-0")
	// s0 status should be Error because filesystem loop reported error for it.
	c.Assert(s0.Status.Status, tc.Equals, status.Error)
	c.Assert(s0.Status.Info, tc.Equals, "filesystem for storage instance \"pgdata/0\" not found")

	c.Assert(s1.StorageTag, tc.Equals, "storage-pgdata-1")

	c.Assert(s1.Status.Status, tc.Equals, status.Active) // linked to filesystem-1

	// Verify Filesystem results
	// We expect 1 valid filesystem. The error one is now handled on the storage instance.
	c.Assert(output.Filesystems, tc.HasLen, 1)
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
	s.statusService.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(nil, nil)
	s.applicationService.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, nil)

	s.portService.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllDevicesByMachineNames(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)
}
