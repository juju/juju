// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryservice "github.com/juju/juju/domain/agentbinary/service"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	provisioning "github.com/juju/juju/domain/provisioner"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

type ProvisionerAdaptersSuite struct{}

func TestProvisionerAdaptersSuite(t *testing.T) {
	tc.Run(t, &ProvisionerAdaptersSuite{})
}

func (s *ProvisionerAdaptersSuite) TestMachinesPreserveNotFoundCode(c *tc.C) {
	adapter := &machinesAPIAdapter{
		machineService: fakeMachineService{
			getMachineUUIDErr: machineerrors.MachineNotFound,
		},
	}

	results, err := adapter.Machines(c.Context(), names.NewMachineTag("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Assert(results[0].Err, tc.NotNil)
	c.Check(results[0].Err.Code, tc.Equals, params.CodeNotFound)
}

func (s *ProvisionerAdaptersSuite) TestProvisioningInfoUsesControllerModelAndPreservesNotFoundCode(c *tc.C) {
	provSvc := &fakeProvisionerService{
		getProvisioningInfoErr: machineerrors.MachineNotFound,
	}
	adapter := &machinesAPIAdapter{
		provisionerSvc: provSvc,
		ctrlConfigSvc:  fakeControllerConfigService{},
		modelInfoSvc: fakeModelInfoService{
			isControllerModel: true,
		},
	}

	results, err := adapter.ProvisioningInfo(c.Context(), []names.MachineTag{names.NewMachineTag("0")})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(provSvc.isControllerModel, tc.IsTrue)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.NotNil)
	c.Check(results.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *ProvisionerAdaptersSuite) TestSetInstanceStatusUpdatesMachineStatusForProvisioningErrors(c *tc.C) {
	statusSvc := &fakeStatusService{}
	machine := &machineAdapter{
		machineName: "0",
		statusSvc:   statusSvc,
	}

	err := machine.SetInstanceStatus(c.Context(), corestatus.ProvisioningError, "boom", map[string]any{"transient": true})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(statusSvc.instanceStatuses, tc.HasLen, 1)
	c.Assert(statusSvc.machineStatuses, tc.HasLen, 1)
	c.Check(statusSvc.instanceStatuses[0].Status, tc.Equals, corestatus.ProvisioningError)
	c.Check(statusSvc.machineStatuses[0].Status, tc.Equals, corestatus.Error)
	c.Check(statusSvc.machineStatuses[0].Message, tc.Equals, "boom")
}

type fakeMachineService struct {
	getMachineUUIDErr error
}

func (f fakeMachineService) SetMachineCloudInstance(context.Context, coremachine.UUID, instance.Id, string, string, *instance.HardwareCharacteristics) error {
	return nil
}

func (f fakeMachineService) GetMachineUUID(context.Context, coremachine.Name) (coremachine.UUID, error) {
	return "", f.getMachineUUIDErr
}

type fakeProvisionerService struct {
	isControllerModel      bool
	getProvisioningInfoErr error
}

func (f *fakeProvisionerService) GetPreludeProvisioningInfo(context.Context) (provisioning.SharedProvisioningInfo, error) {
	return provisioning.SharedProvisioningInfo{}, nil
}

func (f *fakeProvisionerService) GetProvisioningInfo(_ context.Context, _ coremachine.Name, isControllerModel bool, _ provisioning.SharedProvisioningInfo) (provisioning.ProvisioningInfo, error) {
	f.isControllerModel = isControllerModel
	return provisioning.ProvisioningInfo{}, f.getProvisioningInfoErr
}

type fakeControllerConfigService struct{}

func (fakeControllerConfigService) ControllerConfig(context.Context) (controller.Config, error) {
	return controller.Config{}, nil
}

type fakeModelInfoService struct {
	isControllerModel bool
}

func (f fakeModelInfoService) IsControllerModel(context.Context) (bool, error) {
	return f.isControllerModel, nil
}

type fakeStatusService struct {
	instanceStatuses []corestatus.StatusInfo
	machineStatuses  []corestatus.StatusInfo
}

func (f *fakeStatusService) GetInstanceStatus(context.Context, coremachine.Name) (corestatus.StatusInfo, error) {
	return corestatus.StatusInfo{}, errors.NotFoundf("instance status")
}

func (f *fakeStatusService) SetInstanceStatus(_ context.Context, _ coremachine.Name, statusInfo corestatus.StatusInfo) error {
	f.instanceStatuses = append(f.instanceStatuses, statusInfo)
	return nil
}

func (f *fakeStatusService) GetMachineStatus(context.Context, coremachine.Name) (corestatus.StatusInfo, error) {
	return corestatus.StatusInfo{}, errors.NotFoundf("machine status")
}

func (f *fakeStatusService) SetMachineStatus(_ context.Context, _ coremachine.Name, statusInfo corestatus.StatusInfo) error {
	f.machineStatuses = append(f.machineStatuses, statusInfo)
	return nil
}

var _ MachineService = fakeMachineService{}
var _ ProvisionerDomainService = (*fakeProvisionerService)(nil)
var _ ControllerConfigService = fakeControllerConfigService{}
var _ ModelInfoService = fakeModelInfoService{}
var _ StatusDomainService = (*fakeStatusService)(nil)

// --- FindTools fakes ---

type fakeAgentBinaryService struct {
	metadata   []agentbinary.Metadata
	listErr    error
	finderFunc agentbinaryservice.EnvironAgentBinariesFinderFunc
}

func (f *fakeAgentBinaryService) ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error) {
	return f.metadata, f.listErr
}

func (f *fakeAgentBinaryService) GetEnvironAgentBinariesFinder() agentbinaryservice.EnvironAgentBinariesFinderFunc {
	return f.finderFunc
}

type fakeControllerNodeService struct {
	addrs    []string
	addrsErr error
}

func (f *fakeControllerNodeService) GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error) {
	return f.addrs, f.addrsErr
}

var _ AgentBinaryDomainService = (*fakeAgentBinaryService)(nil)
var _ ControllerNodeService = (*fakeControllerNodeService)(nil)

// --- FindTools tests ---

func (s *ProvisionerAdaptersSuite) TestFindToolsEmitsOneEntryPerAPIAddress(c *tc.C) {
	addrs := []string{"10.0.0.1:17070", "52.1.2.3:17070", "localhost:17070"}
	adapter := &toolsFinderAdapter{
		agentBinarySvc: &fakeAgentBinaryService{
			metadata: []agentbinary.Metadata{{
				Version: "4.0.0",
				Arch:    "amd64",
				Size:    1024,
				SHA256:  "abc123",
			}},
		},
		ctrlNodeSvc: &fakeControllerNodeService{addrs: addrs},
		modelUUID:   "test-uuid",
	}

	result, err := adapter.FindTools(c.Context(), semversion.MustParse("4.0.0"), "ubuntu", "amd64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 3)

	for i, addr := range addrs {
		want := fmt.Sprintf("https://%s/model/test-uuid/tools/4.0.0-ubuntu-amd64", addr)
		c.Check(result[i].URL, tc.Equals, want)
	}
}

func (s *ProvisionerAdaptersSuite) TestFindToolsSimplestreamsFallbackRewritesURLs(c *tc.C) {
	addrs := []string{"10.0.0.1:17070", "public.example.com:17070"}
	v := semversion.MustParse("4.0.0")

	adapter := &toolsFinderAdapter{
		agentBinarySvc: &fakeAgentBinaryService{
			// Storage empty — triggers simplestreams fallback.
			metadata: nil,
			finderFunc: func(
				ctx context.Context,
				major, minor int,
				version semversion.Number,
				requestedStream string,
				filter coretools.Filter,
			) (coretools.List, error) {
				binVer := semversion.Binary{Number: v, Arch: "amd64", Release: "ubuntu"}
				return coretools.List{&coretools.Tools{
					Version: binVer,
					URL:     "https://streams.internal/juju-tools-4.0.0.tgz",
				}}, nil
			},
		},
		ctrlNodeSvc: &fakeControllerNodeService{addrs: addrs},
		modelUUID:   "model-uuid",
	}

	result, err := adapter.FindTools(c.Context(), v, "ubuntu", "amd64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)

	for i, addr := range addrs {
		want := fmt.Sprintf("https://%s/model/model-uuid/tools/4.0.0-ubuntu-amd64", addr)
		c.Check(result[i].URL, tc.Equals, want)
	}
}

func (s *ProvisionerAdaptersSuite) TestFindToolsSimplestreamsFinderErrorPropagates(c *tc.C) {
	v := semversion.MustParse("4.0.0")
	adapter := &toolsFinderAdapter{
		agentBinarySvc: &fakeAgentBinaryService{
			finderFunc: func(
				ctx context.Context,
				major, minor int,
				version semversion.Number,
				requestedStream string,
				filter coretools.Filter,
			) (coretools.List, error) {
				return nil, errors.New("streams down")
			},
		},
		ctrlNodeSvc: &fakeControllerNodeService{
			addrs: []string{"10.0.0.1:17070"},
		},
		modelUUID: "test-uuid",
	}

	_, err := adapter.FindTools(c.Context(), v, "ubuntu", "amd64")
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Contains, "streams down")
}

func (s *ProvisionerAdaptersSuite) TestFindToolsListAgentBinariesErrorFallsBackToSimplestreams(c *tc.C) {
	addrs := []string{"10.0.0.1:17070", "public.example.com:17070"}
	v := semversion.MustParse("4.0.0")

	adapter := &toolsFinderAdapter{
		agentBinarySvc: &fakeAgentBinaryService{
			metadata: nil,
			listErr:  errors.New("storage down"),
			finderFunc: func(
				ctx context.Context,
				major, minor int,
				version semversion.Number,
				requestedStream string,
				filter coretools.Filter,
			) (coretools.List, error) {
				binVer := semversion.Binary{Number: v, Arch: "amd64", Release: "ubuntu"}
				return coretools.List{&coretools.Tools{
					Version: binVer,
					URL:     "https://streams.internal/juju-tools-4.0.0.tgz",
				}}, nil
			},
		},
		ctrlNodeSvc: &fakeControllerNodeService{addrs: addrs},
		modelUUID:   "model-uuid",
	}

	result, err := adapter.FindTools(c.Context(), v, "ubuntu", "amd64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)

	for i, addr := range addrs {
		want := fmt.Sprintf("https://%s/model/model-uuid/tools/4.0.0-ubuntu-amd64", addr)
		c.Check(result[i].URL, tc.Equals, want)
	}
}

func (s *ProvisionerAdaptersSuite) TestFindToolsNoAddressesReturnsError(c *tc.C) {
	adapter := &toolsFinderAdapter{
		agentBinarySvc: &fakeAgentBinaryService{
			metadata: []agentbinary.Metadata{{
				Version: "4.0.0",
				Arch:    "amd64",
			}},
		},
		ctrlNodeSvc: &fakeControllerNodeService{addrs: nil},
		modelUUID:   "test-uuid",
	}

	_, err := adapter.FindTools(c.Context(), semversion.MustParse("4.0.0"), "ubuntu", "amd64")
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Contains, "no suitable API server address")
}

func (s *ProvisionerAdaptersSuite) TestFindToolsGetAddressesErrorReturnsError(c *tc.C) {
	adapter := &toolsFinderAdapter{
		agentBinarySvc: &fakeAgentBinaryService{
			metadata: []agentbinary.Metadata{{
				Version: "4.0.0",
				Arch:    "amd64",
			}},
		},
		ctrlNodeSvc: &fakeControllerNodeService{
			addrsErr: errors.New("controller db down"),
		},
		modelUUID: "test-uuid",
	}

	_, err := adapter.FindTools(c.Context(), semversion.MustParse("4.0.0"), "ubuntu", "amd64")
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Contains, "controller db down")
	c.Check(err.Error(), tc.Contains, "getting API addresses")
}

func (s *ProvisionerAdaptersSuite) TestFindToolsDoesNotMutateSharedPointers(c *tc.C) {
	addrs := []string{"addr1:17070", "addr2:17070"}
	adapter := &toolsFinderAdapter{
		agentBinarySvc: &fakeAgentBinaryService{
			metadata: []agentbinary.Metadata{{
				Version: "4.0.0",
				Arch:    "amd64",
				Size:    2048,
				SHA256:  "deadbeef",
			}},
		},
		ctrlNodeSvc: &fakeControllerNodeService{addrs: addrs},
		modelUUID:   "test-uuid",
	}

	result, err := adapter.FindTools(c.Context(), semversion.MustParse("4.0.0"), "ubuntu", "amd64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)

	// Mutating one result should not affect the other.
	result[0].URL = "tampered"
	c.Check(result[1].URL, tc.Not(tc.Equals), "tampered")
}
