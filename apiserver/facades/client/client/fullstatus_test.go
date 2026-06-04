// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/canonical/gomock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/controller"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	application "github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/deployment"
	domainnetwork "github.com/juju/juju/domain/network"
	service "github.com/juju/juju/domain/status/service"
	domainstorage "github.com/juju/juju/domain/storage"
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
	controllerConfigService   *MockControllerConfigService
}

type stubLeadershipReader struct {
	leaders map[string]string
	err     error
}

func (s stubLeadershipReader) Leaders() (map[string]string, error) {
	return s.leaders, s.err
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

func (s *fullStatusSuite) TestFullStatusUsesControllerFlagOnMachineStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := s.client(true)
	s.expectCheckCanRead(client, true)
	s.expectCheckIsAdmin(client, false)

	s.modelInfoService.EXPECT().GetModelInfo(c.Context()).Return(model.ModelInfo{
		Cloud:     "dummy",
		CloudType: "dummy",
		Type:      model.IAAS,
	}, nil)
	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status: status.Available,
	}, nil)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.APIPort:       17070,
		controller.SSHServerPort: 22,
	}, nil)
	s.statusService.EXPECT().GetMachineFullStatuses(gomock.Any()).Return(map[machine.Name]service.Machine{
		"0": {
			Name:         "0",
			IsController: true,
		},
	}, nil)
	s.applicationService.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(nil, nil)
	s.portService.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllDevicesByMachineNames(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)

	output, err := client.FullStatus(c.Context(), params.StatusParams{})
	c.Assert(err, tc.IsNil)
	c.Check(output.Machines["0"].Jobs, tc.DeepEquals, []model.MachineJob{
		model.JobHostUnits,
		model.JobManageModel,
	})
}

func (s *fullStatusSuite) TestFullStatusControllerAppPortsAugmented(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := s.client(true)
	s.expectCheckCanRead(client, true)
	s.expectCheckIsAdmin(client, false)

	s.modelInfoService.EXPECT().GetModelInfo(c.Context()).Return(model.ModelInfo{
		Name:      "controller",
		Cloud:     "dummy",
		CloudType: "dummy",
		Type:      model.IAAS,
	}, nil)
	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status: status.Available,
	}, nil)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.APIPort:       17777,
		controller.SSHServerPort: 2222,
	}, nil)
	s.statusService.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(map[string]service.Application{
		"controller": {
			CharmLocator: charm.CharmLocator{
				Name:         "juju-controller",
				Revision:     1,
				Source:       charm.LocalSource,
				Architecture: architecture.AMD64,
			},
			Platform: deployment.Platform{
				OSType:  deployment.Ubuntu,
				Channel: "22.04/stable",
			},
			Status: status.StatusInfo{
				Status: status.Active,
			},
			Units: map[coreunit.Name]service.Unit{
				"controller/0": {
					ApplicationName: "controller",
					AgentStatus: status.StatusInfo{
						Status: status.Idle,
					},
					WorkloadStatus: status.StatusInfo{
						Status: status.Active,
					},
				},
			},
		},
	}, nil)
	s.applicationService.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetMachineFullStatuses(gomock.Any()).Return(nil, nil)
	s.portService.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllDevicesByMachineNames(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)

	output, err := client.FullStatus(c.Context(), params.StatusParams{})
	c.Assert(err, tc.ErrorIsNil)

	unit := output.Applications["controller"].Units["controller/0"]
	c.Check(slices.Contains(unit.OpenedPorts, fmt.Sprintf("%d/tcp", 17777)), tc.IsTrue)
	c.Check(slices.Contains(unit.OpenedPorts, fmt.Sprintf("%d/tcp", 2222)), tc.IsTrue)
}

func (s *fullStatusSuite) TestFullStatusExposedEndpointsFetchedInBulk(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := s.client(false)
	s.expectCheckCanRead(client, true)
	s.expectCheckIsAdmin(client, false)

	s.modelInfoService.EXPECT().GetModelInfo(c.Context()).Return(model.ModelInfo{
		Cloud:     "dummy",
		CloudType: "dummy",
		Type:      model.IAAS,
	}, nil)
	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status: status.Available,
	}, nil)
	s.applicationService.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(map[string]service.Application{
		"mysql": {
			CharmLocator: charm.CharmLocator{
				Name:         "mysql",
				Revision:     1,
				Source:       charm.LocalSource,
				Architecture: architecture.AMD64,
			},
			Platform: deployment.Platform{
				OSType:  deployment.Ubuntu,
				Channel: "22.04/stable",
			},
			Status: status.StatusInfo{
				Status: status.Active,
			},
			Exposed: true,
		},
	}, nil)
	s.statusService.EXPECT().GetMachineFullStatuses(gomock.Any()).Return(nil, nil)
	s.applicationService.EXPECT().GetAllExposedEndpoints(gomock.Any()).Return(map[string]map[string]application.ExposedEndpoint{
		"mysql": {
			"": {
				ExposeToCIDRs: set.NewStrings("0.0.0.0/0"),
			},
		},
	}, nil)
	s.statusService.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(nil, nil)
	s.portService.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllDevicesByMachineNames(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)

	output, err := client.FullStatus(c.Context(), params.StatusParams{})
	c.Assert(err, tc.IsNil)
	c.Check(output.Applications["mysql"].ExposedEndpoints, tc.DeepEquals, map[string]params.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{"0.0.0.0/0"},
		},
	})
}

func (s *fullStatusSuite) TestFullStatusCAASApplicationAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := s.client(false)
	s.expectCheckCanRead(client, true)
	s.expectCheckIsAdmin(client, false)

	s.applicationService.EXPECT().GetUnitsK8sPodInfo(gomock.Any()).Return(nil, nil)
	s.modelInfoService.EXPECT().GetModelInfo(c.Context()).Return(model.ModelInfo{
		Cloud:     "k8s",
		CloudType: "k8s",
		Type:      model.CAAS,
	}, nil)
	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status: status.Available,
	}, nil)
	s.applicationService.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(map[string]service.Application{
		"postgresql-k8s": {
			CharmLocator: charm.CharmLocator{
				Name:         "postgresql-k8s",
				Revision:     774,
				Source:       charm.CharmHubSource,
				Architecture: architecture.AMD64,
			},
			Platform: deployment.Platform{
				OSType:  deployment.Ubuntu,
				Channel: "22.04/stable",
			},
			Status: status.StatusInfo{
				Status: status.Active,
			},
			Scale:            new(3),
			K8sProviderID:    new("provider-id"),
			K8sPublicAddress: new("10.102.137.7"),
		},
	}, nil)
	s.statusService.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(nil, nil)
	s.portService.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllDevicesByMachineNames(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)

	output, err := client.FullStatus(c.Context(), params.StatusParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(output.Applications["postgresql-k8s"].PublicAddress, tc.Equals, "10.102.137.7")
	c.Check(output.Applications["postgresql-k8s"].ProviderId, tc.Equals, "provider-id")
}

func (s *fullStatusSuite) TestFullStatusFilteredByApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := s.client(false)
	s.expectCheckCanRead(client, true)
	s.expectCheckIsAdmin(client, false)

	s.modelInfoService.EXPECT().GetModelInfo(c.Context()).Return(model.ModelInfo{
		Name:      "controller",
		Cloud:     "dummy",
		CloudType: "dummy",
		Type:      model.IAAS,
	}, nil)
	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status: status.Available,
	}, nil)
	s.applicationService.EXPECT().GetAllEndpointBindings(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(map[string]service.Application{
		"mysql": {
			CharmLocator: charm.CharmLocator{
				Name:         "mysql",
				Revision:     1,
				Source:       charm.LocalSource,
				Architecture: architecture.AMD64,
			},
			Platform: deployment.Platform{
				OSType:  deployment.Ubuntu,
				Channel: "22.04/stable",
			},
			Status: status.StatusInfo{
				Status: status.Active,
			},
			Units: map[coreunit.Name]service.Unit{
				"mysql/0": {
					ApplicationName: "mysql",
					MachineName:     new(machine.Name("1")),
					AgentStatus: status.StatusInfo{
						Status: status.Idle,
					},
					WorkloadStatus: status.StatusInfo{
						Status: status.Active,
					},
				},
			},
		},
		"wordpress": {
			CharmLocator: charm.CharmLocator{
				Name:         "wordpress",
				Revision:     1,
				Source:       charm.LocalSource,
				Architecture: architecture.AMD64,
			},
			Platform: deployment.Platform{
				OSType:  deployment.Ubuntu,
				Channel: "22.04/stable",
			},
			Status: status.StatusInfo{
				Status: status.Active,
			},
			Units: map[coreunit.Name]service.Unit{
				"wordpress/0": {
					ApplicationName: "wordpress",
					MachineName:     new(machine.Name("2")),
					AgentStatus: status.StatusInfo{
						Status: status.Idle,
					},
					WorkloadStatus: status.StatusInfo{
						Status: status.Active,
					},
				},
			},
		},
	}, nil)
	s.statusService.EXPECT().GetRemoteApplicationOffererStatuses(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetMachineFullStatuses(gomock.Any()).Return(map[machine.Name]service.Machine{
		"1": {
			Name:        "1",
			IPAddresses: []string{"10.0.0.1"},
			Platform: deployment.Platform{
				OSType:  deployment.Ubuntu,
				Channel: "22.04/stable",
			},
		},
		"2": {
			Name:        "2",
			IPAddresses: []string{"10.0.0.2"},
			Platform: deployment.Platform{
				OSType:  deployment.Ubuntu,
				Channel: "22.04/stable",
			},
		},
	}, nil)
	s.portService.EXPECT().GetAllOpenedPorts(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().GetAllDevicesByMachineNames(gomock.Any()).Return(nil, nil)
	s.relationService.EXPECT().GetAllRelationDetails(gomock.Any()).Return(nil, nil)

	output, err := client.FullStatus(c.Context(), params.StatusParams{
		Patterns: []string{"mysql"},
	})
	c.Assert(err, tc.IsNil)

	c.Check(output.IsEmpty(), tc.IsFalse)
	c.Check(output.Applications, tc.HasLen, 1)
	c.Check(output.Machines, tc.HasLen, 1)
	_, ok := output.Applications["mysql"]
	c.Check(ok, tc.IsTrue)
	_, ok = output.Applications["wordpress"]
	c.Check(ok, tc.IsFalse)
	_, ok = output.Machines["1"]
	c.Check(ok, tc.IsTrue)
	_, ok = output.Machines["2"]
	c.Check(ok, tc.IsFalse)
	c.Check(output.Applications["mysql"].Units, tc.HasLen, 1)
}

func (s *fullStatusSuite) TestProcessStorageIncludesPoolNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.statusService.EXPECT().GetAllStorageInstanceStatuses(gomock.Any()).Return(
		[]service.StorageInstance{{
			ID:   "data/0",
			Kind: domainstorage.StorageKindFilesystem,
			Life: corelife.Alive,
		}}, nil)
	s.statusService.EXPECT().GetAllFilesystemStatuses(gomock.Any()).Return(
		[]service.Filesystem{{
			ID:        "0",
			Life:      corelife.Alive,
			PoolName:  "fspool",
			SizeMiB:   2048,
			StorageID: "data/0",
		}}, nil)
	s.statusService.EXPECT().GetAllVolumeStatuses(gomock.Any()).Return(
		[]service.Volume{{
			ID:       "0",
			Life:     corelife.Alive,
			PoolName: "blkpool",
			SizeMiB:  2048,
		}}, nil)

	_, filesystems, volumes, err := processStorage(
		c.Context(), s.statusService, nil,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(filesystems, tc.HasLen, 1)
	c.Check(filesystems[0].Info.Pool, tc.Equals, "fspool")
	c.Assert(volumes, tc.HasLen, 1)
	c.Check(volumes[0].Info.Pool, tc.Equals, "blkpool")
}

func (s *fullStatusSuite) TestProcessStorageLinksFilesystemStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitName := coreunit.Name("dummy-storage/0")
	machineName := machine.Name("0")
	storageSince := time.Date(2026, 4, 28, 10, 38, 0, 0, time.UTC)
	filesystemSince := storageSince.Add(time.Minute)

	s.statusService.EXPECT().GetAllStorageInstanceStatuses(gomock.Any()).Return(
		[]service.StorageInstance{{
			UUID:  storageUUID,
			ID:    "multi-fs/0",
			Owner: &unitName,
			Kind:  domainstorage.StorageKindFilesystem,
			Life:  corelife.Alive,
			Status: status.StatusInfo{
				Status: status.Pending,
				Since:  &storageSince,
			},
			Attachments: map[coreunit.Name]service.StorageAttachment{
				unitName: {
					Life:     corelife.Alive,
					Unit:     unitName,
					Machine:  &machineName,
					Location: "/srv/multi-fs/storage-instance",
				},
			},
		}}, nil)
	s.statusService.EXPECT().GetAllFilesystemStatuses(gomock.Any()).Return(
		[]service.Filesystem{{
			ID:        "0",
			StorageID: "multi-fs/0",
			Life:      corelife.Alive,
			Status: status.StatusInfo{
				Status: status.Attached,
				Since:  &filesystemSince,
			},
			MachineAttachments: map[machine.Name]service.FilesystemAttachment{
				machineName: {
					Life:       corelife.Alive,
					MountPoint: "/srv/multi-fs/filesystem",
				},
			},
			UnitAttachments: map[coreunit.Name]service.FilesystemAttachment{
				unitName: {
					Life:       corelife.Alive,
					MountPoint: "/srv/multi-fs/filesystem",
				},
			},
		}}, nil)
	s.statusService.EXPECT().GetAllVolumeStatuses(gomock.Any()).Return(nil, nil)

	storage, filesystems, volumes, err := processStorage(
		c.Context(), s.statusService, nil,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(volumes, tc.HasLen, 0)
	c.Assert(storage, tc.HasLen, 1)
	c.Assert(filesystems, tc.HasLen, 1)

	c.Assert(filesystems[0].Storage, tc.NotNil)
	c.Check(filesystems[0].Storage.StorageTag, tc.Equals, "storage-multi-fs-0")
	c.Check(filesystems[0].Storage.Status.Status, tc.Equals, status.Pending)
	c.Check(storage[0].Status.Status, tc.Equals, status.Pending)

	attachment := filesystems[0].Storage.Attachments["unit-dummy-storage-0"]
	c.Check(attachment.MachineTag, tc.Equals, "machine-0")
	c.Check(attachment.Location, tc.Equals, "/srv/multi-fs/storage-instance")
	c.Check(attachment.Life, tc.Equals, corelife.Alive)
}

func (s *fullStatusSuite) TestProcessStorageLinksVolumeStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitName := coreunit.Name("postgresql/0")
	machineName := machine.Name("0")
	storageSince := time.Date(2026, 4, 28, 10, 38, 0, 0, time.UTC)
	volumeSince := storageSince.Add(time.Minute)

	s.statusService.EXPECT().GetAllStorageInstanceStatuses(gomock.Any()).Return(
		[]service.StorageInstance{{
			UUID:  storageUUID,
			ID:    "pgdata/0",
			Owner: &unitName,
			Kind:  domainstorage.StorageKindBlock,
			Life:  corelife.Alive,
			Status: status.StatusInfo{
				Status: status.Pending,
				Since:  &storageSince,
			},
			Attachments: map[coreunit.Name]service.StorageAttachment{
				unitName: {
					Life:     corelife.Alive,
					Unit:     unitName,
					Machine:  &machineName,
					Location: "/dev/disk/by-id/storage-link",
				},
			},
		}}, nil)
	s.statusService.EXPECT().GetAllFilesystemStatuses(gomock.Any()).Return(nil, nil)
	s.statusService.EXPECT().GetAllVolumeStatuses(gomock.Any()).Return(
		[]service.Volume{{
			ID:         "0",
			StorageID:  "pgdata/0",
			Life:       corelife.Alive,
			Persistent: true,
			Status: status.StatusInfo{
				Status: status.Attached,
				Since:  &volumeSince,
			},
			UnitAttachments: map[coreunit.Name]service.VolumeAttachment{
				unitName: {
					Life:       corelife.Alive,
					DeviceLink: "/dev/disk/by-id/volume-0",
				},
			},
			MachineAttachments: map[machine.Name]service.VolumeAttachment{
				machineName: {
					Life:       corelife.Alive,
					DeviceLink: "/dev/disk/by-id/volume-0",
				},
			},
		}}, nil)

	storage, filesystems, volumes, err := processStorage(
		c.Context(), s.statusService, nil,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(filesystems, tc.HasLen, 0)
	c.Assert(storage, tc.HasLen, 1)
	c.Assert(volumes, tc.HasLen, 1)

	c.Assert(volumes[0].Storage, tc.NotNil)
	c.Check(volumes[0].Storage.StorageTag, tc.Equals, "storage-pgdata-0")
	c.Check(volumes[0].Storage.Status.Status, tc.Equals, status.Pending)
	c.Check(volumes[0].Storage.Persistent, tc.IsFalse)
	c.Check(storage[0].Status.Status, tc.Equals, status.Pending)
	c.Check(storage[0].Persistent, tc.IsFalse)

	attachment := volumes[0].Storage.Attachments["unit-postgresql-0"]
	c.Check(attachment.MachineTag, tc.Equals, "machine-0")
	c.Check(attachment.Location, tc.Equals, "/dev/disk/by-id/storage-link")
	c.Check(attachment.Life, tc.Equals, corelife.Alive)
}

func (s *fullStatusSuite) client(isControllerModel bool) *Client {
	return &Client{
		controllerTag:     names.NewControllerTag(internaluuid.MustNewUUID().String()),
		isControllerModel: isControllerModel,
		modelTag:          names.NewModelTag(s.modelUUID.String()),
		clock:             s.clock,
		auth:              s.authorizer,
		leadershipReader:  stubLeadershipReader{leaders: map[string]string{}},

		applicationService:        s.applicationService,
		blockDeviceService:        s.blockDeviceService,
		crossModelRelationService: s.crossModelRelationService,
		machineService:            s.machineService,
		modelInfoService:          s.modelInfoService,
		networkService:            s.networkService,
		portService:               s.portService,
		relationService:           s.relationService,
		statusService:             s.statusService,
		controllerConfigService:   s.controllerConfigService,
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
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

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
		s.controllerConfigService = nil
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
