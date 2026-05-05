// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	cloudimagemetadataerrors "github.com/juju/juju/domain/cloudimagemetadata/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	modelState      *MockModelState
	controllerState *MockControllerState
	metadataFetcher *MockImageMetadataFetcher
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelState = NewMockModelState(ctrl)
	s.controllerState = NewMockControllerState(ctrl)
	s.metadataFetcher = NewMockImageMetadataFetcher(ctrl)

	return ctrl
}

// expectControllerDefaults sets up standard controller state expectations
// for tests that don't specifically test image metadata or cloud endpoint
// behaviour. Returns minimal cached metadata to avoid triggering the
// external image metadata fetcher.
func (s *serviceSuite) expectControllerDefaults() {
	s.controllerState.EXPECT().GetCloudEndpoint(gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	s.controllerState.EXPECT().GetCachedImageMetadata(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]provisioner.CloudImageMetadata{{ImageID: "default-img"}}, nil,
	).AnyTimes()
}

// expectControllerDefaultsNoCache sets up standard controller state
// expectations that return empty cached metadata, forcing the external
// image metadata fetcher path.
func (s *serviceSuite) expectControllerDefaultsNoCache() {
	s.controllerState.EXPECT().GetCloudEndpoint(gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	s.controllerState.EXPECT().GetCachedImageMetadata(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
}

func (s *serviceSuite) newService(c *tc.C) *Service {
	return NewService(
		s.modelState,
		s.controllerState,
		s.metadataFetcher,
		model.UUID("model-uuid-1234"),
		loggertesting.WrapCheckLog(c),
	)
}

// TestGetProvisioningInfoInvalidMachineName verifies that an invalid machine
// name is rejected immediately.
func (s *serviceSuite) TestGetProvisioningInfoInvalidMachineName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.newService(c)
	_, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("invalid/name"), false)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err, tc.ErrorMatches, `validating machine name "invalid/name":.*`)
}

// TestGetProvisioningInfoMachineNotFound verifies that a MachineNotFound
// error from the model state is propagated.
func (s *serviceSuite) TestGetProvisioningInfoMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).
		Return(provisioner.ProvisioningInfoState{}, machineerrors.MachineNotFound)

	svc := s.newService(c)
	_, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetProvisioningInfoControllerConfigError verifies controller config
// errors are propagated.
func (s *serviceSuite) TestGetProvisioningInfoControllerConfigError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).
		Return(provisioner.ProvisioningInfoState{
			Base: corebase.MustParseBaseFromString("ubuntu@22.04"),
		}, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).
		Return(nil, errors.New("db error"))

	svc := s.newService(c)
	_, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorMatches, `getting controller config:.*db error`)
}

// TestGetProvisioningInfoMinimal verifies a minimal happy path with no
// volumes, no bindings, no image metadata cache — just basic fields.
func (s *serviceSuite) TestGetProvisioningInfoMinimal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{
		"controller-uuid": "ctrl-uuid-1",
	}, nil)
	s.expectControllerDefaultsNoCache()

	// No cached metadata, so falls through to external fetch.
	// Return 2 items to exercise the sort-by-priority path.
	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Stream:   "released",
		Region:   "us-east-1",
	}).Return([]provisioner.CloudImageMetadata{
		{ImageID: "ami-456", Priority: 20, Region: "us-east-1", Arch: "amd64"},
		{ImageID: "ami-123", Priority: 10, Region: "us-east-1", Arch: "amd64"},
	}, nil)

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.MachineUUID, tc.Equals, coremachine.UUID("machine-uuid-1"))
	c.Check(info.Base, tc.Equals, corebase.MustParseBaseFromString("ubuntu@22.04"))
	c.Check(info.Jobs, tc.DeepEquals, []model.MachineJob{model.JobHostUnits})
	c.Check(info.ControllerConfig, tc.DeepEquals, map[string]any{"controller-uuid": "ctrl-uuid-1"})
	c.Check(info.Tags[tags.JujuController], tc.Equals, "ctrl-uuid-1")
	c.Check(info.Tags[tags.JujuModel], tc.Equals, "model-uuid-1234")
	c.Check(info.Tags[tags.JujuMachine], tc.Equals, "mymodel-machine-0")
	// Results sorted by priority ascending.
	c.Assert(len(info.ImageMetadata), tc.Equals, 2)
	c.Check(info.ImageMetadata[0].ImageID, tc.Equals, "ami-123")
	c.Check(info.ImageMetadata[1].ImageID, tc.Equals, "ami-456")
}

// TestGetProvisioningInfoControllerMachine verifies that a controller
// machine in a controller model gets the JobManageModel job.
func (s *serviceSuite) TestGetProvisioningInfoControllerMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID:  "machine-uuid-ctrl",
		Base:         corebase.MustParseBaseFromString("ubuntu@22.04"),
		IsController: true,
		ModelName:    "controller",
		CloudType:    "ec2",
		CloudRegion:  "us-east-1",
		ImageStream:  "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", true).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{
		"controller-uuid": "ctrl-uuid-1",
	}, nil)
	s.expectControllerDefaultsNoCache()
	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), gomock.Any()).
		Return([]provisioner.CloudImageMetadata{{ImageID: "ami-1"}}, nil)

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Jobs, tc.DeepEquals, []model.MachineJob{model.JobHostUnits, model.JobManageModel})
}

// TestGetProvisioningInfoNonControllerMachineInControllerModel verifies
// that a non-controller machine in a controller model only gets JobHostUnits.
func (s *serviceSuite) TestGetProvisioningInfoNonControllerMachineInControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID:  "machine-uuid-2",
		Base:         corebase.MustParseBaseFromString("ubuntu@22.04"),
		IsController: false,
		ModelName:    "controller",
		CloudType:    "ec2",
		CloudRegion:  "us-east-1",
		ImageStream:  "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "1", true).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{
		"controller-uuid": "ctrl-uuid-1",
	}, nil)
	s.expectControllerDefaultsNoCache()
	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), gomock.Any()).
		Return([]provisioner.CloudImageMetadata{{ImageID: "ami-1"}}, nil)

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("1"), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Jobs, tc.DeepEquals, []model.MachineJob{model.JobHostUnits})
}

// TestGetProvisioningInfoWithPlacement verifies placement is passed through.
func (s *serviceSuite) TestGetProvisioningInfoWithPlacement(c *tc.C) {
	defer s.setupMocks(c).Finish()

	placement := "zone=us-east-1a"
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID:        "machine-uuid-1",
		Base:               corebase.MustParseBaseFromString("ubuntu@22.04"),
		PlacementDirective: &placement,
		ModelName:          "mymodel",
		CloudType:          "ec2",
		CloudRegion:        "us-east-1",
		ImageStream:        "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaultsNoCache()
	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), gomock.Any()).
		Return([]provisioner.CloudImageMetadata{{ImageID: "ami-1"}}, nil)

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.PlacementDirective, tc.Not(tc.IsNil))
	c.Check(*info.PlacementDirective, tc.Equals, "zone=us-east-1a")
}

// TestGetProvisioningInfoWithConstraints verifies constraints are passed
// through and influence image metadata lookups.
func (s *serviceSuite) TestGetProvisioningInfoWithConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	arch := "arm64"
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@24.04"),
		Constraints: constraints.Value{Arch: &arch},
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "eu-west-1",
		ImageStream: "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaultsNoCache()

	// Should pass the arch constraint through.
	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), provisioner.ImageConstraint{
		Releases: []string{"24.04"},
		Arches:   []string{"arm64"},
		Stream:   "released",
		Region:   "eu-west-1",
	}).Return([]provisioner.CloudImageMetadata{{ImageID: "ami-arm", Arch: "arm64"}}, nil)

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Constraints.Arch, tc.Not(tc.IsNil))
	c.Check(*info.Constraints.Arch, tc.Equals, "arm64")
}

// TestGetProvisioningInfoWithCachedImageMetadata verifies that cached
// metadata is used directly (sorted by priority) without calling the fetcher.
func (s *serviceSuite) TestGetProvisioningInfoWithCachedImageMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.controllerState.EXPECT().GetCloudEndpoint(gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil)
	s.controllerState.EXPECT().GetCachedImageMetadata(gomock.Any(), "22.04", "").Return([]provisioner.CloudImageMetadata{
		{ImageID: "ami-low", Priority: 50},
		{ImageID: "ami-high", Priority: 10},
	}, nil)
	// FetchImageMetadata should NOT be called.

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.ImageMetadata, tc.HasLen, 2)
	// Sorted by priority ascending.
	c.Check(info.ImageMetadata[0].ImageID, tc.Equals, "ami-high")
	c.Check(info.ImageMetadata[1].ImageID, tc.Equals, "ami-low")
}

// TestGetProvisioningInfoImageMetadataFetcherError verifies that fetch
// errors are propagated.
func (s *serviceSuite) TestGetProvisioningInfoImageMetadataFetcherError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaultsNoCache()
	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("simplestreams unavailable"))

	svc := s.newService(c)
	_, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorMatches, `resolving image metadata:.*simplestreams unavailable`)
}

// TestGetProvisioningInfoImageMetadataNotFound verifies that when the
// fetcher returns empty metadata, a NotFound error is returned.
func (s *serviceSuite) TestGetProvisioningInfoImageMetadataNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaultsNoCache()
	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), gomock.Any()).
		Return([]provisioner.CloudImageMetadata{}, nil)

	svc := s.newService(c)
	_, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIs, cloudimagemetadataerrors.NotFound)
}

// TestGetProvisioningInfoEndpointBindingsWithProviderID verifies endpoint
// bindings resolution uses provider ID when available.
func (s *serviceSuite) TestGetProvisioningInfoEndpointBindingsWithProviderID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		EndpointBindings: map[string]map[string]network.SpaceUUID{
			"myapp": {
				"web": "space-uuid-1",
				"db":  "space-uuid-2",
			},
		},
		Spaces: network.SpaceInfos{
			{ID: "space-uuid-1", Name: "public", ProviderId: "subnet-provider-1"},
			{ID: "space-uuid-2", Name: "internal", ProviderId: ""},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// "web" bound to space with provider ID → uses provider ID.
	c.Check(info.EndpointBindings["web"], tc.Equals, "subnet-provider-1")
	// "db" bound to space without provider ID → uses space name.
	c.Check(info.EndpointBindings["db"], tc.Equals, "internal")
}

// TestGetProvisioningInfoEndpointBindingsSpaceNotFound verifies that
// bindings referencing unknown spaces are silently skipped.
func (s *serviceSuite) TestGetProvisioningInfoEndpointBindingsSpaceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		EndpointBindings: map[string]map[string]network.SpaceUUID{
			"myapp": {
				"web": "space-uuid-unknown",
			},
		},
		Spaces:              network.SpaceInfos{},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	// No bindings resolved since space not found.
	c.Check(info.EndpointBindings, tc.HasLen, 0)
}

// TestGetProvisioningInfoSpaceConstraintConflict verifies that a binding
// to a space excluded by constraints produces an error.
func (s *serviceSuite) TestGetProvisioningInfoSpaceConstraintConflict(c *tc.C) {
	defer s.setupMocks(c).Finish()

	excludeSpaces := []string{"^public"}
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		Constraints: constraints.Value{Spaces: &excludeSpaces},
		EndpointBindings: map[string]map[string]network.SpaceUUID{
			"myapp": {"web": "space-uuid-1"},
		},
		Spaces: network.SpaceInfos{
			{ID: "space-uuid-1", Name: "public", ProviderId: "prov-1"},
		},
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	_, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorMatches, `.*conflicts with negative space constraint`)
}

// TestGetProvisioningInfoNetworkTopology verifies the space-subnet-AZ
// mapping is built correctly when constraints include spaces.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopology(c *tc.C) {
	defer s.setupMocks(c).Finish()

	includeSpaces := []string{"myspace"}
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		Constraints: constraints.Value{Spaces: &includeSpaces},
		Spaces: network.SpaceInfos{
			{
				ID:   "space-uuid-myspace",
				Name: "myspace",
				Subnets: network.SubnetInfos{
					{
						ProviderId:        "subnet-aaa",
						CIDR:              "10.0.0.0/24",
						AvailabilityZones: []string{"us-east-1a", "us-east-1b"},
					},
					{
						ProviderId:        "subnet-bbb",
						CIDR:              "10.0.1.0/24",
						AvailabilityZones: []string{"us-east-1c"},
					},
				},
			},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.SpaceSubnets["myspace"], tc.DeepEquals, []string{"subnet-aaa", "subnet-bbb"})
	c.Check(info.SubnetAZs["subnet-aaa"], tc.DeepEquals, []string{"us-east-1a", "us-east-1b"})
	c.Check(info.SubnetAZs["subnet-bbb"], tc.DeepEquals, []string{"us-east-1c"})
}

// TestGetProvisioningInfoNetworkTopologySkipsAlphaWhenNotConstrained verifies
// that when the only space is alpha and it wasn't explicitly constrained,
// network topology is empty.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopologySkipsAlphaWhenNotConstrained(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		EndpointBindings: map[string]map[string]network.SpaceUUID{
			"myapp": {"web": "space-uuid-alpha"},
		},
		Spaces: network.SpaceInfos{
			{
				ID:         "space-uuid-alpha",
				Name:       network.AlphaSpaceName,
				ProviderId: "prov-alpha",
				Subnets: network.SubnetInfos{
					{ProviderId: "subnet-1", CIDR: "10.0.0.0/24", AvailabilityZones: []string{"az1"}},
				},
			},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// Alpha not explicitly constrained → empty topology.
	c.Check(info.SpaceSubnets, tc.IsNil)
	c.Check(info.SubnetAZs, tc.IsNil)
}

// TestGetProvisioningInfoNetworkTopologyAlphaExplicitlyConstrained verifies
// that when alpha is explicitly constrained, topology IS populated.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopologyAlphaExplicitlyConstrained(c *tc.C) {
	defer s.setupMocks(c).Finish()

	includeSpaces := []string{"alpha"}
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		Constraints: constraints.Value{Spaces: &includeSpaces},
		Spaces: network.SpaceInfos{
			{
				ID:   "space-uuid-alpha",
				Name: network.AlphaSpaceName,
				Subnets: network.SubnetInfos{
					{ProviderId: "subnet-1", CIDR: "10.0.0.0/24", AvailabilityZones: []string{"az1"}},
				},
			},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.SpaceSubnets["alpha"], tc.DeepEquals, []string{"subnet-1"})
	c.Check(info.SubnetAZs["subnet-1"], tc.DeepEquals, []string{"az1"})
}

// TestGetProvisioningInfoNetworkTopologySkipsSubnetNoProviderID verifies
// subnets without provider IDs are skipped.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopologySkipsSubnetNoProviderID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	includeSpaces := []string{"myspace"}
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		Constraints: constraints.Value{Spaces: &includeSpaces},
		Spaces: network.SpaceInfos{
			{
				ID:   "space-uuid-1",
				Name: "myspace",
				Subnets: network.SubnetInfos{
					{ProviderId: "", CIDR: "10.0.0.0/24", AvailabilityZones: []string{"az1"}},
					{ProviderId: "subnet-valid", CIDR: "10.0.1.0/24", AvailabilityZones: []string{"az2"}},
				},
			},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// Only the subnet with a provider ID is included.
	c.Check(info.SpaceSubnets["myspace"], tc.DeepEquals, []string{"subnet-valid"})
}

// TestGetProvisioningInfoNetworkTopologyAzureNoAZsAllowed verifies that
// azure/openstack subnets without AZs are still included.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopologyAzureNoAZsAllowed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	includeSpaces := []string{"myspace"}
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "azure",
		CloudRegion: "eastus",
		ImageStream: "released",
		Constraints: constraints.Value{Spaces: &includeSpaces},
		Spaces: network.SpaceInfos{
			{
				ID:   "space-uuid-1",
				Name: "myspace",
				Subnets: network.SubnetInfos{
					{ProviderId: "subnet-az", CIDR: "10.0.0.0/24", AvailabilityZones: nil},
				},
			},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// Azure allows subnets with no AZs.
	c.Check(info.SpaceSubnets["myspace"], tc.DeepEquals, []string{"subnet-az"})
}

// TestGetProvisioningInfoNetworkTopologyEC2SkipsNoAZs verifies that non-azure
// non-openstack subnets without AZs are skipped.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopologyEC2SkipsNoAZs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	includeSpaces := []string{"myspace"}
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		Constraints: constraints.Value{Spaces: &includeSpaces},
		Spaces: network.SpaceInfos{
			{
				ID:   "space-uuid-1",
				Name: "myspace",
				Subnets: network.SubnetInfos{
					{ProviderId: "subnet-noaz", CIDR: "10.0.0.0/24", AvailabilityZones: nil},
				},
			},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// EC2 skips subnets with no AZs → empty list.
	c.Check(info.SpaceSubnets["myspace"], tc.HasLen, 0)
}

// TestGetProvisioningInfoTags verifies tag computation including unit tags
// and resource tags.
func (s *serviceSuite) TestGetProvisioningInfoTags(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "prod",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		UnitNames: []coreunit.NameWithPrincipal{
			{Name: coreunit.Name("wordpress/0")},
			{Name: coreunit.Name("wordpress/1")},
		},
		ResourceTags:        "env=production team=infra",
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{
		"controller-uuid": "ctrl-1",
	}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.Tags[tags.JujuModel], tc.Equals, "model-uuid-1234")
	c.Check(info.Tags[tags.JujuController], tc.Equals, "ctrl-1")
	c.Check(info.Tags[tags.JujuMachine], tc.Equals, "prod-machine-0")
	c.Check(info.Tags[tags.JujuUnitsDeployed], tc.Equals, "wordpress/0 wordpress/1")
	// Resource tags from model config.
	c.Check(info.Tags["env"], tc.Equals, "production")
	c.Check(info.Tags["team"], tc.Equals, "infra")
}

// TestGetProvisioningInfoTagsWithSubordinates verifies that subordinate
// units use their principal's name in the tag.
func (s *serviceSuite) TestGetProvisioningInfoTagsWithSubordinates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	principalName := coreunit.Name("wordpress/0")
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "prod",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		UnitNames: []coreunit.NameWithPrincipal{
			{Name: coreunit.Name("wordpress/0")},
			{Name: coreunit.Name("nrpe/0"), Principal: &principalName},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// nrpe/0 is subordinate → uses principal "wordpress/0", deduplicated.
	c.Check(info.Tags[tags.JujuUnitsDeployed], tc.Equals, "wordpress/0")
}

// TestGetProvisioningInfoVolumes verifies volume params are built correctly
// including attachment association.
func (s *serviceSuite) TestGetProvisioningInfoVolumes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		VolumeParams: []provisioner.VolumeProvisioningParams{
			{
				UUID:             "vol-uuid-1",
				ID:               "0",
				Provider:         "ebs",
				RequestedSizeMiB: 1024,
				Attributes:       map[string]string{"type": "gp3"},
				Tags:             map[string]string{"juju-model-uuid": "model-uuid-1234"},
			},
		},
		VolumeAttachmentParams: []provisioner.VolumeAttachmentProvisioningParams{
			{
				VolumeUUID:       "vol-uuid-1",
				VolumeID:         "0",
				Provider:         "ebs",
				ReadOnly:         false,
				VolumeProviderID: "",
			},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "5", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("5"), false)
	c.Assert(err, tc.ErrorIsNil)

	// Volume has its attachment associated (same VolumeUUID).
	c.Assert(info.Volumes, tc.HasLen, 1)
	c.Check(info.Volumes[0].VolumeID, tc.Equals, "0")
	c.Check(info.Volumes[0].Provider, tc.Equals, "ebs")
	c.Check(info.Volumes[0].SizeMiB, tc.Equals, uint64(1024))
	c.Check(info.Volumes[0].Attributes["type"], tc.Equals, "gp3")
	c.Assert(info.Volumes[0].Attachment, tc.Not(tc.IsNil))
	c.Check(info.Volumes[0].Attachment.MachineID, tc.Equals, "5")
	c.Check(info.Volumes[0].Attachment.Provider, tc.Equals, "ebs")

	// No standalone attachments since the attachment was associated.
	c.Check(info.VolumeAttachments, tc.HasLen, 0)
}

// TestGetProvisioningInfoVolumeAttachmentStandalone verifies that volume
// attachments not associated with a volume param are returned standalone.
func (s *serviceSuite) TestGetProvisioningInfoVolumeAttachmentStandalone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		VolumeAttachmentParams: []provisioner.VolumeAttachmentProvisioningParams{
			{
				VolumeUUID:       "vol-uuid-99",
				VolumeID:         "99",
				Provider:         "ebs",
				ReadOnly:         true,
				VolumeProviderID: "vol-abc123",
			},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// No volumes created (no VolumeParams), but standalone attachment exists.
	c.Check(info.Volumes, tc.HasLen, 0)
	c.Assert(info.VolumeAttachments, tc.HasLen, 1)
	c.Check(info.VolumeAttachments[0].VolumeID, tc.Equals, "99")
	c.Check(info.VolumeAttachments[0].MachineID, tc.Equals, "0")
	c.Check(info.VolumeAttachments[0].Provider, tc.Equals, "ebs")
	c.Check(info.VolumeAttachments[0].ReadOnly, tc.IsTrue)
	c.Check(info.VolumeAttachments[0].ProviderID, tc.Equals, "vol-abc123")
}

// TestGetProvisioningInfoRootDisk verifies root disk params are built
// when a storage pool is specified.
func (s *serviceSuite) TestGetProvisioningInfoRootDisk(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		RootDiskStoragePool: &provisioner.StoragePool{
			Provider: "ebs",
			Attrs:    map[string]string{"volume-type": "gp3"},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.RootDisk, tc.Not(tc.IsNil))
	c.Check(info.RootDisk.Provider, tc.Equals, "ebs")
	c.Check(info.RootDisk.Attributes["volume-type"], tc.Equals, "gp3")
}

// TestGetProvisioningInfoRootDiskNil verifies no root disk when pool is nil.
func (s *serviceSuite) TestGetProvisioningInfoRootDiskNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID:         "machine-uuid-1",
		Base:                corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:           "mymodel",
		CloudType:           "ec2",
		CloudRegion:         "us-east-1",
		ImageStream:         "released",
		RootDiskStoragePool: nil,
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.RootDisk, tc.IsNil)
}

// TestGetProvisioningInfoCloudInitUserData verifies cloud-init user data
// is passed through from state.
func (s *serviceSuite) TestGetProvisioningInfoCloudInitUserData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		CloudInitUserData: "packages:\n  - htop\n  - vim\n",
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.CloudInitUserData, tc.DeepEquals, map[string]any{
		"packages": []any{"htop", "vim"},
	})
}

// TestGetProvisioningInfoImageConstraintWithImageID verifies that the
// ImageID from constraints is passed to the image metadata fetcher.
func (s *serviceSuite) TestGetProvisioningInfoImageConstraintWithImageID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	imageID := "ami-specific"
	arch := "amd64"
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		Constraints: constraints.Value{
			Arch:    &arch,
			ImageID: &imageID,
		},
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaultsNoCache()

	// Should include the image ID in the constraint.
	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Arches:   []string{"amd64"},
		Stream:   "released",
		Region:   "us-east-1",
		ImageID:  &imageID,
	}).Return([]provisioner.CloudImageMetadata{{ImageID: imageID}}, nil)

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.ImageMetadata[0].ImageID, tc.Equals, "ami-specific")
}

// TestGetProvisioningInfoMultipleAppsEndpointBindings verifies that endpoint
// bindings from multiple applications are merged.
func (s *serviceSuite) TestGetProvisioningInfoMultipleAppsEndpointBindings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
		EndpointBindings: map[string]map[string]network.SpaceUUID{
			"wordpress": {"web": "space-uuid-1"},
			"mysql":     {"db": "space-uuid-2"},
		},
		Spaces: network.SpaceInfos{
			{ID: "space-uuid-1", Name: "public", ProviderId: "prov-pub"},
			{ID: "space-uuid-2", Name: "data", ProviderId: "prov-data"},
		},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.EndpointBindings["web"], tc.Equals, "prov-pub")
	c.Check(info.EndpointBindings["db"], tc.Equals, "prov-data")
}

// TestGetProvisioningInfoResourceTagsNotFound verifies that when resource
// tags are not configured, they are not included in instance tags.
func (s *serviceSuite) TestGetProvisioningInfoResourceTagsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID:         "machine-uuid-1",
		Base:                corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:           "mymodel",
		CloudType:           "ec2",
		CloudRegion:         "us-east-1",
		ImageStream:         "released",
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{
		"controller-uuid": "ctrl-1",
	}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// Standard tags are present.
	c.Check(info.Tags[tags.JujuModel], tc.Equals, "model-uuid-1234")
	c.Check(info.Tags[tags.JujuController], tc.Equals, "ctrl-1")
	// No custom resource tags.
	_, hasEnv := info.Tags["env"]
	c.Check(hasEnv, tc.IsFalse)
}

// TestGetProvisioningInfoNetworkTopologyOpenstackNoAZs verifies that
// openstack subnets without AZs are still included.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopologyOpenstackNoAZs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	includeSpaces := []string{"myspace"}
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "openstack",
		CloudRegion: "RegionOne",
		ImageStream: "released",
		Constraints: constraints.Value{Spaces: &includeSpaces},
		Spaces: network.SpaceInfos{
			{
				ID:   "space-uuid-1",
				Name: "myspace",
				Subnets: network.SubnetInfos{
					{ProviderId: "subnet-os", CIDR: "10.0.0.0/24", AvailabilityZones: nil},
				},
			},
		},
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.controllerState.EXPECT().GetCloudEndpoint(gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil)
	s.controllerState.EXPECT().GetCachedImageMetadata(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		[]provisioner.CloudImageMetadata{{ImageID: "img-1"}}, nil)

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.SpaceSubnets["myspace"], tc.DeepEquals, []string{"subnet-os"})
}

// TestGetProvisioningInfoNetworkTopologySpaceNotInModel verifies that
// constrained spaces not present in model are logged and skipped.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopologySpaceNotInModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	includeSpaces := []string{"missing-space"}
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID:         "machine-uuid-1",
		Base:                corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:           "mymodel",
		CloudType:           "ec2",
		CloudRegion:         "us-east-1",
		ImageStream:         "released",
		Constraints:         constraints.Value{Spaces: &includeSpaces},
		Spaces:              network.SpaceInfos{},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)

	// Space not found → empty topology.
	c.Check(info.SpaceSubnets["missing-space"], tc.HasLen, 0)
}

// TestGetProvisioningInfoNetworkTopologyEmptySpaces verifies that when
// there are no space names at all, topology is nil.
func (s *serviceSuite) TestGetProvisioningInfoNetworkTopologyEmptySpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID:         "machine-uuid-1",
		Base:                corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:           "mymodel",
		CloudType:           "ec2",
		CloudRegion:         "us-east-1",
		ImageStream:         "released",
		Spaces:              network.SpaceInfos{},
		
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaults()

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.SpaceSubnets, tc.IsNil)
	c.Check(info.SubnetAZs, tc.IsNil)
}

// TestGetProvisioningInfoImageMetadataWithCloudEndpoint verifies that
// the cloud endpoint is passed to the fetcher when available.
func (s *serviceSuite) TestGetProvisioningInfoImageMetadataWithCloudEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base:        corebase.MustParseBaseFromString("ubuntu@22.04"),
		ModelName:   "mymodel",
		CloudType:   "openstack",
		CloudRegion: "RegionOne",
		CloudName:   "mycloud",
		ImageStream: "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.controllerState.EXPECT().GetCloudEndpoint(gomock.Any(), "mycloud", "RegionOne").
		Return("https://cloud.example.com:5000/v3", nil)
	s.controllerState.EXPECT().GetCachedImageMetadata(gomock.Any(), "22.04", "").Return(nil, nil)

	s.metadataFetcher.EXPECT().FetchImageMetadata(gomock.Any(), provisioner.ImageConstraint{
		Releases: []string{"22.04"},
		Stream:   "released",
		Region:   "RegionOne",
		Endpoint: "https://cloud.example.com:5000/v3",
	}).Return([]provisioner.CloudImageMetadata{{ImageID: "img-os-1"}}, nil)

	svc := s.newService(c)
	info, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.ImageMetadata[0].ImageID, tc.Equals, "img-os-1")
}

// TestGetProvisioningInfoImageMetadataBaseParseError verifies that a
// malformed base channel causes a parse error in resolveImageMetadata.
func (s *serviceSuite) TestGetProvisioningInfoImageMetadataBaseParseError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Base with malformed channel — will fail ParseBase.
	stateInfo := provisioner.ProvisioningInfoState{
		MachineUUID: "machine-uuid-1",
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "invalid///track",
				Risk:  "stable",
			},
		},
		ModelName:   "mymodel",
		CloudType:   "ec2",
		CloudRegion: "us-east-1",
		ImageStream: "released",
	}
	s.modelState.EXPECT().GetProvisioningInfo(gomock.Any(), "0", false).Return(stateInfo, nil)
	s.controllerState.EXPECT().GetControllerConfig(gomock.Any()).Return(map[string]any{}, nil)
	s.expectControllerDefaultsNoCache()

svc := s.newService(c)
_, err := svc.GetProvisioningInfo(c.Context(), coremachine.Name("0"), false)
c.Assert(err, tc.ErrorMatches, `resolving image metadata:.*`)
}
