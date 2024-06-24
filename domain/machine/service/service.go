// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence methods for machines.
type State interface {
	// CreateMachine persists the input machine entity.
	CreateMachine(context.Context, string, string, string) error

	// DeleteMachine deletes the input machine entity.
	DeleteMachine(context.Context, string) error

	// InitialWatchStatement returns the table and the initial watch statement
	// for the machines.
	InitialWatchStatement() (string, string)

	// GetMachineLife returns the life status of the specified machine.
	GetMachineLife(context.Context, string) (*life.Life, error)

	// AllMachines retrieves the ids of all machines in the model.
	// If there's no machine, it returns an empty slice.
	AllMachines(context.Context) ([]string, error)

	// InstanceId returns the provider specific instance id for this machine.
	// If the machine is not provisioned, it returns a NotProvisionedError.
	InstanceId(context.Context, string) (string, error)

	// InstanceStatus returns the provider specific instance status for this
	// machine.
	// If the machine is not provisioned, it returns a NotProvisionedError.
	InstanceStatus(context.Context, string) (string, error)

	// HardwareCharacteristics returns the hardware characteristics struct with
	// data retrieved from the machine cloud instance table.
	HardwareCharacteristics(context.Context, string) (*instance.HardwareCharacteristics, error)

	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(context.Context, string, instance.Id, instance.HardwareCharacteristics) error

	// DeleteMachineCloudInstance removes an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	DeleteMachineCloudInstance(context.Context, string) error
}

// Service provides the API for working with machines.
type Service struct {
	st State
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// CreateMachine creates the specified machine.
func (s *Service) CreateMachine(ctx context.Context, machineId string) (string, error) {
	// Make a new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Annotatef(err, "creating machine %q", machineId)
	}
	machineUUID, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Annotatef(err, "creating machine %q", machineId)
	}

	err = s.st.CreateMachine(ctx, machineId, nodeUUID.String(), machineUUID.String())

	return machineUUID.String(), errors.Annotatef(err, "creating machine %q", machineId)
}

// DeleteMachine deletes the specified machine.
func (s *Service) DeleteMachine(ctx context.Context, machineId string) error {
	err := s.st.DeleteMachine(ctx, machineId)
	return errors.Annotatef(err, "deleting machine %q", machineId)
}

// GetLife returns the GetMachineLife status of the specified machine.
func (s *Service) GetMachineLife(ctx context.Context, machineId string) (*life.Life, error) {
	life, err := s.st.GetMachineLife(ctx, machineId)
	return life, errors.Annotatef(err, "getting life status for machine %q", machineId)
}

// ListAllMachines returns the ids of all machines in the model.
func (s *Service) ListAllMachines(ctx context.Context) ([]string, error) {
	machines, err := s.st.AllMachines(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving all machines")
	}
	return machines, nil
}

// InstanceId returns the provider specific instance id for this machine.
// If the machine is not provisioned, it returns a NotProvisionedError.
func (s *Service) InstanceId(ctx context.Context, machineId string) (string, error) {
	instanceId, err := s.st.InstanceId(ctx, machineId)
	if err != nil {
		return "", errors.Annotatef(err, "retrieving cloud instance id for machine %q", machineId)
	}
	return instanceId, nil
}

// InstanceStatus returns the provider specific instance status for this
// machine.
// If the machine is not provisioned, it returns a NotProvisionedError.
func (s *Service) InstanceStatus(ctx context.Context, machineId string) (string, error) {
	instanceStatus, err := s.st.InstanceStatus(ctx, machineId)
	if err != nil {
		return "", errors.Annotatef(err, "retrieving cloud instance status for machine %q", machineId)
	}
	return instanceStatus, nil
}
