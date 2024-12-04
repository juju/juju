// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence methods for machines.
type State interface {
	// CreateMachine persists the input machine entity.
	// It returns a MachineAlreadyExists error if a machine with the same name
	// already exists.
	CreateMachine(context.Context, coremachine.Name, string, string) error

	// CreateMachineWithparent persists the input machine entity, associating it
	// with the parent machine.
	// It returns a MachineAlreadyExists error if a machine with the same name
	// already exists.
	// It returns a MachineNotFound error if the parent machine does not exist.

	CreateMachineWithParent(context.Context, coremachine.Name, coremachine.Name, string, string) error

	// DeleteMachine deletes the input machine entity.
	DeleteMachine(context.Context, coremachine.Name) error

	// InitialWatchStatement returns the table and the initial watch statement
	// for the machines.
	InitialWatchStatement() (string, string)

	// InitialWatchModelMachinesStatement returns the table and the initial watch
	// statement for watching life changes of non-container machines.
	InitialWatchModelMachinesStatement() (string, string)

	// GetMachineLife returns the life status of the specified machine.
	// It returns a MachineNotFound if the given machine doesn't exist.
	GetMachineLife(context.Context, coremachine.Name) (*life.Life, error)

	// SetMachineLife sets the life status of the specified machine.
	// It returns a MachineNotFound if the provided machine doesn't exist.
	SetMachineLife(context.Context, coremachine.Name, life.Life) error

	// AllMachineNames retrieves the names of all machines in the model.
	// If there's no machine, it returns an empty slice.
	AllMachineNames(context.Context) ([]coremachine.Name, error)

	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(context.Context, string) (string, error)

	// InstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	InstanceIDAndName(ctx context.Context, mUUID string) (string, string, error)

	// GetInstanceStatus returns the cloud specific instance status for this
	// machine.
	// It returns MachineNotFound if the machine does not exist.
	// It returns a StatusNotSet if the instance status is not set.
	GetInstanceStatus(context.Context, coremachine.Name) (status.StatusInfo, error)

	// SetInstanceStatus sets the cloud specific instance status for this
	// machine.
	// It returns MachineNotFound if the machine does not exist.
	SetInstanceStatus(context.Context, coremachine.Name, status.StatusInfo) error

	// GetMachineStatus returns the status of the specified machine.
	// It returns MachineNotFound if the machine does not exist.
	// It returns a StatusNotSet if the status is not set.
	GetMachineStatus(context.Context, coremachine.Name) (status.StatusInfo, error)

	// SetMachineStatus sets the status of the specified machine.
	// It returns MachineNotFound if the machine does not exist.
	SetMachineStatus(context.Context, coremachine.Name, status.StatusInfo) error

	// HardwareCharacteristics returns the hardware characteristics struct with
	// data retrieved from the machine cloud instance table.
	HardwareCharacteristics(context.Context, string) (*instance.HardwareCharacteristics, error)

	// AvailabilityZone returns the availability zone for the specified machine.
	AvailabilityZone(context.Context, string) (string, error)

	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(context.Context, string, instance.Id, string, *instance.HardwareCharacteristics) error

	// DeleteMachineCloudInstance removes an entry in the machine cloud instance
	// table along with the instance tags and the link to a lxd profile if any.
	DeleteMachineCloudInstance(context.Context, string) error

	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(context.Context, coremachine.Name) (bool, error)

	// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
	// the corresponding cloud instance to be stopped.
	ShouldKeepInstance(ctx context.Context, mName machine.Name) (bool, error)

	// SetKeepInstance sets whether the machine cloud instance will be retained
	// when the machine is removed from Juju. This is only relevant if an instance
	// exists.
	SetKeepInstance(ctx context.Context, mName machine.Name, keep bool) error

	// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
	RequireMachineReboot(ctx context.Context, uuid string) error

	// ClearMachineReboot removes the reboot flag of the machine referenced by its UUID if a reboot has previously been required.
	ClearMachineReboot(ctx context.Context, uuid string) error

	// IsMachineRebootRequired checks if the machine referenced by its UUID requires a reboot.
	IsMachineRebootRequired(ctx context.Context, uuid string) (bool, error)

	// GetMachineParentUUID returns the parent UUID of the specified machine.
	// It returns a MachineNotFound if the machine does not exist.
	// It returns a MachineHasNoParent if the machine has no parent.
	GetMachineParentUUID(ctx context.Context, machineUUID string) (string, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or shutdown
	ShouldRebootOrShutdown(ctx context.Context, uuid string) (coremachine.RebootAction, error)

	// MarkMachineForRemoval marks the given machine for removal.
	// It returns a MachineNotFound error if the machine does not exist.
	MarkMachineForRemoval(context.Context, coremachine.Name) error

	// GetAllMachineRemovals returns the UUIDs of all of the machines that need
	// to be removed but need provider-level cleanup.
	GetAllMachineRemovals(context.Context) ([]string, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, coremachine.Name) (string, error)

	// AppliedLXDProfileNames returns the names of the LXD profiles on the machine.
	AppliedLXDProfileNames(ctx context.Context, mUUID string) ([]string, error)

	// SetAppliedLXDProfileNames sets the list of LXD profile names to the
	// lxd_profile table for the given machine. This method will overwrite the list
	// of profiles for the given machine without any checks.
	SetAppliedLXDProfileNames(ctx context.Context, mUUID string, profileNames []string) error
}

// Provider represents an underlying cloud provider.
type Provider interface {
	environs.BootstrapEnviron
	environs.InstanceTypesFetcher
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
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (s *Service) CreateMachine(ctx context.Context, machineName coremachine.Name) (string, error) {
	// Make a new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, machineUUID, err := createUUIDs()
	if err != nil {
		return "", errors.Errorf("creating machine %q %w", machineName, err)
	}

	err = s.st.CreateMachine(ctx, machineName, nodeUUID, machineUUID)

	return machineUUID, errors.Errorf("creating machine %q %w", machineName, err)
}

// CreateMachineWirhParent creates the specified machine with the specified
// parent.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
// It returns a MachineNotFound error if the parent machine does not exist.
func (s *Service) CreateMachineWithParent(ctx context.Context, machineName, parentName coremachine.Name) (string, error) {
	// Make a new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, machineUUID, err := createUUIDs()
	if err != nil {
		return "", errors.Errorf("creating machine %q with parent %q %w", machineName, parentName, err)
	}

	err = s.st.CreateMachineWithParent(ctx, machineName, parentName, nodeUUID, machineUUID)

	return machineUUID, errors.Errorf("creating machine %q with parent %q %w", machineName, parentName, err)
}

// createUUIDs generates a new UUID for the machine and the net-node.
func createUUIDs() (string, string, error) {
	nodeUUID, err := uuid.NewUUID()
	if err != nil {
		return "", "", errors.Errorf("generating net-node UUID %w", err)
	}
	machineUUID, err := uuid.NewUUID()
	if err != nil {
		return "", "", errors.Errorf("generating machine UUID %w", err)
	}
	return nodeUUID.String(), machineUUID.String(), nil
}

// DeleteMachine deletes the specified machine.
func (s *Service) DeleteMachine(ctx context.Context, machineName coremachine.Name) error {
	err := s.st.DeleteMachine(ctx, machineName)
	return errors.Errorf("deleting machine %q %w", machineName, err)
}

// GetMachineLife returns the GetMachineLife status of the specified machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) GetMachineLife(ctx context.Context, machineName coremachine.Name) (*life.Life, error) {
	life, err := s.st.GetMachineLife(ctx, machineName)
	return life, errors.Errorf("getting life status for machine %q %w", machineName, err)
}

// SetMachineLife sets the life status of the specified machine.
// It returns a MachineNotFound if the provided machine doesn't exist.
func (s *Service) SetMachineLife(ctx context.Context, machineName coremachine.Name, life life.Life) error {
	err := s.st.SetMachineLife(ctx, machineName, life)
	return errors.Errorf("setting life status for machine %q %w", machineName, err)
}

// EnsureDeadMachine sets the provided machine's life status to Dead.
// No error is returned if the provided machine doesn't exist, just nothing gets
// updated.
func (s *Service) EnsureDeadMachine(ctx context.Context, machineName coremachine.Name) error {
	return s.SetMachineLife(ctx, machineName, life.Dead)
}

// AllMachineNames returns the names of all machines in the model.
func (s *Service) AllMachineNames(ctx context.Context) ([]coremachine.Name, error) {
	machines, err := s.st.AllMachineNames(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving all machines %w", err)
	}
	return machines, nil
}

// GetInstanceStatus returns the cloud specific instance status for this
// machine.
// It returns MachineNotFound if the machine does not exist.
// It returns a StatusNotSet if the instance status is not set.
// Idempotent.
func (s *Service) GetInstanceStatus(ctx context.Context, machineName coremachine.Name) (status.StatusInfo, error) {
	instanceStatus, err := s.st.GetInstanceStatus(ctx, machineName)
	if err != nil {
		return status.StatusInfo{}, errors.Errorf("retrieving cloud instance status for machine %q %w", machineName, err)
	}
	return instanceStatus, nil
}

// SetInstanceStatus sets the cloud specific instance status for this
// machine.
// It returns MachineNotFound if the machine does not exist.
// It returns InvalidStatus if the given status is not a known status value.
func (s *Service) SetInstanceStatus(ctx context.Context, machineName coremachine.Name, status status.StatusInfo) error {
	if !status.Status.KnownInstanceStatus() {
		return machineerrors.InvalidStatus
	}
	err := s.st.SetInstanceStatus(ctx, machineName, status)
	if err != nil {
		return errors.Errorf("setting cloud instance status for machine %q %w", machineName, err)
	}
	return nil
}

// GetMachineStatus returns the status of the specified machine.
// It returns MachineNotFound if the machine does not exist.
// It returns a StatusNotSet if the status is not set.
// Idempotent.
func (s *Service) GetMachineStatus(ctx context.Context, machineName coremachine.Name) (status.StatusInfo, error) {
	machineStatus, err := s.st.GetMachineStatus(ctx, machineName)
	if err != nil {
		return status.StatusInfo{}, errors.Errorf("retrieving machine status for machine %q %w", machineName, err)
	}
	return machineStatus, nil
}

// SetMachineStatus sets the status of the specified machine.
// It returns MachineNotFound if the machine does not exist.
// It returns InvalidStatus if the given status is not a known status value.
func (s *Service) SetMachineStatus(ctx context.Context, machineName coremachine.Name, status status.StatusInfo) error {
	if !status.Status.KnownMachineStatus() {
		return machineerrors.InvalidStatus
	}
	err := s.st.SetMachineStatus(ctx, machineName, status)
	if err != nil {
		return errors.Errorf("setting machine status for machine %q %w", machineName, err)
	}
	return nil
}

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) IsMachineController(ctx context.Context, machineName coremachine.Name) (bool, error) {
	isController, err := s.st.IsMachineController(ctx, machineName)
	if err != nil {
		return false, errors.Errorf("checking if machine %q is a controller %w", machineName, err)
	}
	return isController, nil
}

// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
// the corresponding cloud instance to be stopped.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) ShouldKeepInstance(ctx context.Context, machineName coremachine.Name) (bool, error) {
	keepInstance, err := s.st.ShouldKeepInstance(ctx, machineName)
	if err != nil {
		return false, errors.Capture(err)
	}
	return keepInstance, nil
}

// SetKeepInstance sets whether the machine cloud instance will be retained
// when the machine is removed from Juju. This is only relevant if an instance
// exists.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) SetKeepInstance(ctx context.Context, machineName coremachine.Name, keep bool) error {
	return errors.Capture(s.st.SetKeepInstance(ctx, machineName, keep))
}

// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
func (s *Service) RequireMachineReboot(ctx context.Context, uuid string) error {
	return errors.Errorf("requiring a machine reboot for machine with uuid %q %w", uuid, s.st.RequireMachineReboot(ctx, uuid))
}

// ClearMachineReboot removes the reboot flag of the machine referenced by its UUID if a reboot has previously been required.
func (s *Service) ClearMachineReboot(ctx context.Context, uuid string) error {
	return errors.Errorf("clear machine reboot flag for machine with uuid %q %w", uuid, s.st.ClearMachineReboot(ctx, uuid))
}

// IsMachineRebootRequired checks if the machine referenced by its UUID requires a reboot.
func (s *Service) IsMachineRebootRequired(ctx context.Context, uuid string) (bool, error) {
	rebootRequired, err := s.st.IsMachineRebootRequired(ctx, uuid)
	return rebootRequired, errors.Errorf("checking if machine with uuid %q is requiring a reboot %w", uuid, err)
}

// GetMachineParentUUID returns the parent UUID of the specified machine.
// It returns a MachineNotFound if the machine does not exist.
// It returns a MachineHasNoParent if the machine has no parent.
// It returns a GrandParentNotAllowed if the machine's parent has a parent.
func (s *Service) GetMachineParentUUID(ctx context.Context, machineUUID string) (string, error) {
	parentUUID, err := s.st.GetMachineParentUUID(ctx, machineUUID)
	if err != nil {
		return "", errors.Errorf("retrieving parent UUID for machine %q %w", machineUUID, err)
	}
	return parentUUID, nil
}

// ShouldRebootOrShutdown determines whether a machine should reboot or shutdown
func (s *Service) ShouldRebootOrShutdown(ctx context.Context, uuid string) (coremachine.RebootAction, error) {
	rebootRequired, err := s.st.ShouldRebootOrShutdown(ctx, uuid)
	return rebootRequired, errors.Errorf("getting if the machine with uuid %q need to reboot or shutdown %w", uuid, err)
}

// MarkMachineForRemoval marks the given machine for removal.
// It returns a MachineNotFound error if the machine does not exist.
func (s *Service) MarkMachineForRemoval(ctx context.Context, machineName coremachine.Name) error {
	return errors.Errorf("marking machine %q for removal %w", machineName, s.st.MarkMachineForRemoval(ctx, machineName))
}

// GetAllMachineRemovals returns the UUIDs of all of the machines that need to
// be removed but need provider-level cleanup.
func (s *Service) GetAllMachineRemovals(ctx context.Context) ([]string, error) {
	removals, err := s.st.GetAllMachineRemovals(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving all machines marked to be removed %w", err)
	}
	return removals, nil
}

// GetMachineUUID returns the UUID of a machine identified by its name.
// It returns a MachineNotFound if the machine does not exist.
func (s *Service) GetMachineUUID(ctx context.Context, name coremachine.Name) (string, error) {
	return s.st.GetMachineUUID(ctx, name)
}

// LXDProfiles returns the names of the LXD profiles on the machine.
func (s *Service) AppliedLXDProfileNames(ctx context.Context, mUUID string) ([]string, error) {
	profiles, err := s.st.AppliedLXDProfileNames(ctx, mUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return profiles, nil
}

// SetLXDProfiles sets the list of LXD profile names to the lxd_profile table
// for the given machine. This method will overwrite the list of profiles for
// the given machine without any checks.
// [machineerrors.MachineNotFound] will be returned if the machine does not
// exist.
func (s *Service) SetAppliedLXDProfileNames(ctx context.Context, mUUID string, profileNames []string) error {
	return errors.Capture(s.st.SetAppliedLXDProfileNames(ctx, mUUID, profileNames))
}

// ProviderService provides the API for working with machines using the
// underlying provider.
type ProviderService struct {
	Service

	providerGetter providertracker.ProviderGetter[Provider]
}

// GetBootstrapEnviron returns the bootstrap environ.
func (s *ProviderService) GetBootstrapEnviron(ctx context.Context) (environs.BootstrapEnviron, error) {
	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return provider, nil
}

// GetInstanceTypesFetcher returns the instance types fetcher.
func (s *ProviderService) GetInstanceTypesFetcher(ctx context.Context) (environs.InstanceTypesFetcher, error) {
	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return provider, nil
}
