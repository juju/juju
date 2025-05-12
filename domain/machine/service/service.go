// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
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
	CreateMachine(context.Context, machine.Name, string, machine.UUID) error

	// CreateMachineWithparent persists the input machine entity, associating it
	// with the parent machine.
	// It returns a MachineAlreadyExists error if a machine with the same name
	// already exists.
	// It returns a MachineNotFound error if the parent machine does not exist.

	CreateMachineWithParent(context.Context, machine.Name, machine.Name, string, machine.UUID) error

	// DeleteMachine deletes the input machine entity.
	DeleteMachine(context.Context, machine.Name) error

	// InitialWatchStatement returns the table and the initial watch statement
	// for the machines.
	InitialWatchStatement() (string, string)

	// InitialWatchModelMachinesStatement returns the table and the initial watch
	// statement for watching life changes of non-container machines.
	InitialWatchModelMachinesStatement() (string, string)

	// GetMachineLife returns the life status of the specified machine.
	// It returns a MachineNotFound if the given machine doesn't exist.
	GetMachineLife(context.Context, machine.Name) (*life.Life, error)

	// SetMachineLife sets the life status of the specified machine.
	// It returns a MachineNotFound if the provided machine doesn't exist.
	SetMachineLife(context.Context, machine.Name, life.Life) error

	// AllMachineNames retrieves the names of all machines in the model.
	// If there's no machine, it returns an empty slice.
	AllMachineNames(context.Context) ([]machine.Name, error)

	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(context.Context, machine.UUID) (string, error)

	// InstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	InstanceIDAndName(ctx context.Context, mUUID machine.UUID) (string, string, error)

	// GetInstanceStatus returns the cloud specific instance status for this
	// machine.
	// It returns MachineNotFound if the machine does not exist.
	// It returns a StatusNotSet if the instance status is not set.
	GetInstanceStatus(context.Context, machine.Name) (domainmachine.StatusInfo[domainmachine.InstanceStatusType], error)

	// SetInstanceStatus sets the cloud specific instance status for this
	// machine.
	// It returns MachineNotFound if the machine does not exist.
	SetInstanceStatus(context.Context, machine.Name, domainmachine.StatusInfo[domainmachine.InstanceStatusType]) error

	// GetMachineStatus returns the status of the specified machine.
	// It returns MachineNotFound if the machine does not exist.
	// It returns a StatusNotSet if the status is not set.
	GetMachineStatus(context.Context, machine.Name) (domainmachine.StatusInfo[domainmachine.MachineStatusType], error)

	// SetMachineStatus sets the status of the specified machine.
	// It returns MachineNotFound if the machine does not exist.
	SetMachineStatus(context.Context, machine.Name, domainmachine.StatusInfo[domainmachine.MachineStatusType]) error

	// HardwareCharacteristics returns the hardware characteristics struct with
	// data retrieved from the machine cloud instance table.
	HardwareCharacteristics(context.Context, machine.UUID) (*instance.HardwareCharacteristics, error)

	// AvailabilityZone returns the availability zone for the specified machine.
	AvailabilityZone(context.Context, machine.UUID) (string, error)

	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(context.Context, machine.UUID, instance.Id, string, *instance.HardwareCharacteristics) error

	// SetRunningAgentBinaryVersion sets the running agent version for the machine.
	// A MachineNotFound error will be returned if the machine does not exist.
	SetRunningAgentBinaryVersion(context.Context, machine.UUID, coreagentbinary.Version) error

	// DeleteMachineCloudInstance removes an entry in the machine cloud instance
	// table along with the instance tags and the link to a lxd profile if any.
	DeleteMachineCloudInstance(context.Context, machine.UUID) error

	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(context.Context, machine.Name) (bool, error)

	// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
	// the corresponding cloud instance to be stopped.
	ShouldKeepInstance(ctx context.Context, mName machine.Name) (bool, error)

	// SetKeepInstance sets whether the machine cloud instance will be retained
	// when the machine is removed from Juju. This is only relevant if an instance
	// exists.
	SetKeepInstance(ctx context.Context, mName machine.Name, keep bool) error

	// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
	RequireMachineReboot(ctx context.Context, uuid machine.UUID) error

	// ClearMachineReboot removes the reboot flag of the machine referenced by its UUID if a reboot has previously been required.
	ClearMachineReboot(ctx context.Context, uuid machine.UUID) error

	// IsMachineRebootRequired checks if the machine referenced by its UUID requires a reboot.
	IsMachineRebootRequired(ctx context.Context, uuid machine.UUID) (bool, error)

	// GetMachineParentUUID returns the parent UUID of the specified machine.
	// It returns a MachineNotFound if the machine does not exist.
	// It returns a MachineHasNoParent if the machine has no parent.
	GetMachineParentUUID(ctx context.Context, machineUUID machine.UUID) (machine.UUID, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or shutdown
	ShouldRebootOrShutdown(ctx context.Context, uuid machine.UUID) (machine.RebootAction, error)

	// MarkMachineForRemoval marks the given machine for removal.
	// It returns a MachineNotFound error if the machine does not exist.
	MarkMachineForRemoval(context.Context, machine.Name) error

	// GetAllMachineRemovals returns the UUIDs of all of the machines that need
	// to be removed but need provider-level cleanup.
	GetAllMachineRemovals(context.Context) ([]machine.UUID, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// AppliedLXDProfileNames returns the names of the LXD profiles on the machine.
	AppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID) ([]string, error)

	// SetAppliedLXDProfileNames sets the list of LXD profile names to the
	// lxd_profile table for the given machine. This method will overwrite the list
	// of profiles for the given machine without any checks.
	SetAppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID, profileNames []string) error

	// NamespaceForWatchMachineCloudInstance returns the namespace for watching
	// machine cloud instance changes.
	NamespaceForWatchMachineCloudInstance() string

	// NamespaceForWatchMachineLXDProfiles returns the namespace for watching
	// machine LXD profile changes.
	NamespaceForWatchMachineLXDProfiles() string

	// NamespaceForWatchMachineReboot returns the namespace string used for
	// tracking machine reboot events in the model.
	NamespaceForWatchMachineReboot() string

	// GetNamesForUUIDs returns a map of machine UUIDs to machine Names based
	// on the given machine UUIDs.
	// [machineerrors.MachineNotFound] will be returned if the machine does not
	// exist.
	GetNamesForUUIDs(ctx context.Context, machineUUIDs []string) (map[string]machine.Name, error)
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
func (s *Service) CreateMachine(ctx context.Context, machineName machine.Name) (_ machine.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	// Make a new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, machineUUID, err := createUUIDs()
	if err != nil {
		return "", errors.Errorf("creating machine %q: %w", machineName, err)
	}

	err = s.st.CreateMachine(ctx, machineName, nodeUUID, machineUUID)
	if err != nil {
		return machineUUID, errors.Errorf("creating machine %q: %w", machineName, err)
	}
	return machineUUID, nil
}

// CreateMachineWithParent creates the specified machine with the specified
// parent.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
// It returns a MachineNotFound error if the parent machine does not exist.
func (s *Service) CreateMachineWithParent(ctx context.Context, machineName, parentName machine.Name) (_ machine.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	// Make a new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, machineUUID, err := createUUIDs()
	if err != nil {
		return "", errors.Errorf("creating machine %q with parent %q: %w", machineName, parentName, err)
	}

	err = s.st.CreateMachineWithParent(ctx, machineName, parentName, nodeUUID, machineUUID)
	if err != nil {
		return machineUUID, errors.Errorf("creating machine %q with parent %q: %w", machineName, parentName, err)
	}
	return machineUUID, nil
}

// createUUIDs generates a new UUID for the machine and the net-node.
func createUUIDs() (string, machine.UUID, error) {
	nodeUUID, err := uuid.NewUUID()
	if err != nil {
		return "", "", errors.Errorf("generating net-node UUID: %w", err)
	}
	machineUUID, err := machine.NewUUID()
	if err != nil {
		return "", "", errors.Errorf("generating machine UUID: %w", err)
	}
	return nodeUUID.String(), machineUUID, nil
}

// DeleteMachine deletes the specified machine.
func (s *Service) DeleteMachine(ctx context.Context, machineName machine.Name) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.st.DeleteMachine(ctx, machineName); err != nil {
		return errors.Errorf("deleting machine %q: %w", machineName, err)
	}
	return nil
}

// GetMachineLife returns the GetMachineLife status of the specified machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) GetMachineLife(ctx context.Context, machineName machine.Name) (_ *life.Life, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	life, err := s.st.GetMachineLife(ctx, machineName)
	if err != nil {
		return life, errors.Errorf("getting life status for machine %q: %w", machineName, err)
	}
	return life, nil
}

// SetMachineLife sets the life status of the specified machine.
// It returns a MachineNotFound if the provided machine doesn't exist.
func (s *Service) SetMachineLife(ctx context.Context, machineName machine.Name, life life.Life) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.st.SetMachineLife(ctx, machineName, life); err != nil {
		return errors.Errorf("setting life status for machine %q: %w", machineName, err)
	}
	return nil
}

// EnsureDeadMachine sets the provided machine's life status to Dead.
// No error is returned if the provided machine doesn't exist, just nothing gets
// updated.
func (s *Service) EnsureDeadMachine(ctx context.Context, machineName machine.Name) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return s.SetMachineLife(ctx, machineName, life.Dead)
}

// AllMachineNames returns the names of all machines in the model.
func (s *Service) AllMachineNames(ctx context.Context) (_ []machine.Name, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	machines, err := s.st.AllMachineNames(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving all machines: %w", err)
	}
	return machines, nil
}

// GetInstanceStatus returns the cloud specific instance status for this
// machine.
// It returns MachineNotFound if the machine does not exist.
// It returns a StatusNotSet if the instance status is not set.
// Idempotent.
func (s *Service) GetInstanceStatus(ctx context.Context, machineName machine.Name) (_ status.StatusInfo, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	instanceStatus, err := s.st.GetInstanceStatus(ctx, machineName)
	if err != nil {
		return status.StatusInfo{}, errors.Errorf("retrieving cloud instance status for machine %q: %w", machineName, err)
	}

	return decodeInstanceStatus(instanceStatus)
}

// SetInstanceStatus sets the cloud specific instance status for this machine.
// It returns MachineNotFound if the machine does not exist. It returns
// InvalidStatus if the given status is not a known status value.
func (s *Service) SetInstanceStatus(ctx context.Context, machineName machine.Name, status status.StatusInfo) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if !status.Status.KnownInstanceStatus() {
		return machineerrors.InvalidStatus
	}

	instanceStatus, err := encodeInstanceStatus(status)
	if err != nil {
		return errors.Errorf("encoding status for machine %q: %w", machineName, err)
	}

	if err := s.st.SetInstanceStatus(ctx, machineName, instanceStatus); err != nil {
		return errors.Errorf("setting cloud instance status for machine %q: %w", machineName, err)
	}
	return nil
}

// GetMachineStatus returns the status of the specified machine. It returns
// MachineNotFound if the machine does not exist. It returns a StatusNotSet if
// the status is not set. Idempotent.
func (s *Service) GetMachineStatus(ctx context.Context, machineName machine.Name) (_ status.StatusInfo, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	machineStatus, err := s.st.GetMachineStatus(ctx, machineName)
	if err != nil {
		return status.StatusInfo{}, errors.Errorf("retrieving machine status for machine %q: %w", machineName, err)
	}

	var data map[string]any
	if len(machineStatus.Data) > 0 {
		if err := json.Unmarshal(machineStatus.Data, &data); err != nil {
			return status.StatusInfo{}, errors.Errorf("unmarshalling machine data for machine %q: %w", machineName, err)
		}
	}

	return decodeMachineStatus(machineStatus)
}

// SetMachineStatus sets the status of the specified machine. It returns
// MachineNotFound if the machine does not exist. It returns InvalidStatus if
// the given status is not a known status value.
func (s *Service) SetMachineStatus(ctx context.Context, machineName machine.Name, status status.StatusInfo) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if !status.Status.KnownMachineStatus() {
		return machineerrors.InvalidStatus
	}

	machineStatus, err := encodeMachineStatus(status)
	if err != nil {
		return errors.Errorf("encoding status for machine %q: %w", machineName, err)
	}

	if err := s.st.SetMachineStatus(ctx, machineName, machineStatus); err != nil {
		return errors.Errorf("setting machine status for machine %q: %w", machineName, err)
	}
	return nil
}

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) IsMachineController(ctx context.Context, machineName machine.Name) (_ bool, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	isController, err := s.st.IsMachineController(ctx, machineName)
	if err != nil {
		return false, errors.Errorf("checking if machine %q is a controller: %w", machineName, err)
	}
	return isController, nil
}

// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
// the corresponding cloud instance to be stopped.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) ShouldKeepInstance(ctx context.Context, machineName machine.Name) (_ bool, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()
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
func (s *Service) SetKeepInstance(ctx context.Context, machineName machine.Name, keep bool) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return errors.Capture(s.st.SetKeepInstance(ctx, machineName, keep))
}

// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
func (s *Service) RequireMachineReboot(ctx context.Context, uuid machine.UUID) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.st.RequireMachineReboot(ctx, uuid); err != nil {
		return errors.Errorf("requiring a machine reboot for machine with uuid %q: %w", uuid, err)
	}
	return nil
}

// ClearMachineReboot removes the reboot flag of the machine referenced by its UUID if a reboot has previously been required.
func (s *Service) ClearMachineReboot(ctx context.Context, uuid machine.UUID) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.st.ClearMachineReboot(ctx, uuid); err != nil {
		return errors.Errorf("clear machine reboot flag for machine with uuid %q: %w", uuid, err)
	}
	return nil
}

// IsMachineRebootRequired checks if the machine referenced by its UUID requires a reboot.
func (s *Service) IsMachineRebootRequired(ctx context.Context, uuid machine.UUID) (_ bool, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	rebootRequired, err := s.st.IsMachineRebootRequired(ctx, uuid)
	if err != nil {
		return rebootRequired, errors.Errorf("checking if machine with uuid %q is requiring a reboot: %w", uuid, err)
	}
	return rebootRequired, nil
}

// GetMachineParentUUID returns the parent UUID of the specified machine.
// It returns a MachineNotFound if the machine does not exist.
// It returns a MachineHasNoParent if the machine has no parent.
// It returns a GrandParentNotAllowed if the machine's parent has a parent.
func (s *Service) GetMachineParentUUID(ctx context.Context, machineUUID machine.UUID) (_ machine.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	parentUUID, err := s.st.GetMachineParentUUID(ctx, machineUUID)
	if err != nil {
		return "", errors.Errorf("retrieving parent UUID for machine %q: %w", machineUUID, err)
	}
	return parentUUID, nil
}

// ShouldRebootOrShutdown determines whether a machine should reboot or shutdown
func (s *Service) ShouldRebootOrShutdown(ctx context.Context, uuid machine.UUID) (_ machine.RebootAction, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	rebootRequired, err := s.st.ShouldRebootOrShutdown(ctx, uuid)
	if err != nil {
		return rebootRequired, errors.Errorf("getting if the machine with uuid %q need to reboot or shutdown: %w", uuid, err)
	}
	return rebootRequired, nil
}

// MarkMachineForRemoval marks the given machine for removal.
// It returns a MachineNotFound error if the machine does not exist.
func (s *Service) MarkMachineForRemoval(ctx context.Context, machineName machine.Name) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := s.st.MarkMachineForRemoval(ctx, machineName); err != nil {
		return errors.Errorf("marking machine %q for removal: %w", machineName, err)
	}
	return nil
}

// GetAllMachineRemovals returns the UUIDs of all of the machines that need to
// be removed but need provider-level cleanup.
func (s *Service) GetAllMachineRemovals(ctx context.Context) (_ []machine.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	removals, err := s.st.GetAllMachineRemovals(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving all machines marked to be removed: %w", err)
	}
	return removals, nil
}

// GetMachineUUID returns the UUID of a machine identified by its name.
// It returns a MachineNotFound if the machine does not exist.
func (s *Service) GetMachineUUID(ctx context.Context, name machine.Name) (_ machine.UUID, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()
	return s.st.GetMachineUUID(ctx, name)
}

// AppliedLXDProfileNames returns the names of the LXD profiles on the machine.
func (s *Service) AppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID) (_ []string, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()
	profiles, err := s.st.AppliedLXDProfileNames(ctx, mUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return profiles, nil
}

// SetAppliedLXDProfileNames sets the list of LXD profile names to the
// lxd_profile table for the given machine. This method will overwrite the list
// of profiles for the given machine without any checks.
// [machineerrors.MachineNotFound] will be returned if the machine does not
// exist.
func (s *Service) SetAppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID, profileNames []string) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()
	return errors.Capture(s.st.SetAppliedLXDProfileNames(ctx, mUUID, profileNames))
}

// GetMachineArchesForApplication returns a list of architectures which are
// included across the machines of the given application.
//
// TODO: Implement this method.
func (s *Service) GetMachineArchesForApplication(ctx context.Context, appUUID application.ID) (_ []arch.Arch, err error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return nil, errors.Errorf("GetMachineArchesForApplication not implemented")
}

// ProviderService provides the API for working with machines using the
// underlying provider.
type ProviderService struct {
	Service

	providerGetter providertracker.ProviderGetter[Provider]
}

// GetBootstrapEnviron returns the bootstrap environ.
func (s *ProviderService) GetBootstrapEnviron(ctx context.Context) (_ environs.BootstrapEnviron, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return provider, nil
}

// GetInstanceTypesFetcher returns the instance types fetcher.
func (s *ProviderService) GetInstanceTypesFetcher(ctx context.Context) (_ environs.InstanceTypesFetcher, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return provider, nil
}
