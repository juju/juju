// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	provisioning "github.com/juju/juju/domain/provisioner"
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
