// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	"github.com/juju/juju/domain/machine/internal"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

// State describes retrieval and persistence methods for machines.
type State interface {
	// PlaceMachine places the net node and machines if required, depending
	// on the placement.
	// It returns the net node UUID for the machine and a list of child
	// machine names that were created as part of the placement.
	AddMachine(ctx context.Context, args domainmachine.AddMachineArgs) (string, []machine.Name, error)

	// DeleteMachine deletes the input machine entity.
	DeleteMachine(context.Context, machine.Name) error

	// InitialWatchStatement returns the table and the initial watch statement
	// for the machines.
	InitialWatchStatement() (string, string)

	// InitialWatchModelMachinesStatement returns the table and the initial
	// watch statement for watching life changes of non-container machines.
	InitialWatchModelMachinesStatement() (string, string)

	// InitialWatchModelMachineLifeAndStartTimesStatement returns the namespace and the initial watch
	// statement for watching life and agent start time changes machines.
	InitialWatchModelMachineLifeAndStartTimesStatement() (string, string)

	// GetMachineLife returns the life status of the specified machine.
	// It returns a MachineNotFound if the given machine doesn't exist.
	GetMachineLife(context.Context, machine.Name) (life.Life, error)

	// AllMachineNames retrieves the names of all machines in the model.
	// If there's no machine, it returns an empty slice.
	AllMachineNames(context.Context) ([]machine.Name, error)

	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(context.Context, string) (string, error)

	// GetInstanceIDAndName returns the cloud specific instance ID and display name
	// for this machine.
	GetInstanceIDAndName(ctx context.Context, mUUID string) (string, string, error)

	// GetHardwareCharacteristics returns the hardware characteristics struct with
	// data retrieved from the machine cloud instance table.
	GetHardwareCharacteristics(context.Context, string) (*instance.HardwareCharacteristics, error)

	// AvailabilityZone returns the availability zone for the specified machine.
	AvailabilityZone(context.Context, string) (string, error)

	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(context.Context, string, instance.Id, string, string, *instance.HardwareCharacteristics) error

	// SetRunningAgentBinaryVersion sets the running agent version for the
	// machine. A MachineNotFound error will be returned if the machine does not
	// exist.
	SetRunningAgentBinaryVersion(context.Context, string, coreagentbinary.Version) error

	// DeleteMachineCloudInstance removes an entry in the machine cloud instance
	// table along with the instance tags and the link to a lxd profile if any.
	DeleteMachineCloudInstance(context.Context, string) error

	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(context.Context, machine.Name) (bool, error)

	// IsMachineManuallyProvisioned returns whether the machine is a manual
	// machine.
	IsMachineManuallyProvisioned(context.Context, machine.Name) (bool, error)

	// ShouldKeepInstance reports whether a machine, when removed from Juju,
	// should cause the corresponding cloud instance to be stopped.
	ShouldKeepInstance(ctx context.Context, mName machine.Name) (bool, error)

	// SetKeepInstance sets whether the machine cloud instance will be retained
	// when the machine is removed from Juju. This is only relevant if an
	// instance exists.
	SetKeepInstance(ctx context.Context, mName machine.Name, keep bool) error

	// RequireMachineReboot sets the machine referenced by its UUID as requiring
	// a reboot.
	RequireMachineReboot(ctx context.Context, uuid machine.UUID) error

	// ClearMachineReboot removes the reboot flag of the machine referenced by
	// its UUID if a reboot has previously been required.
	ClearMachineReboot(ctx context.Context, uuid machine.UUID) error

	// IsMachineRebootRequired checks if the machine referenced by its UUID
	// requires a reboot.
	IsMachineRebootRequired(ctx context.Context, uuid machine.UUID) (bool, error)

	// GetMachineParentUUID returns the parent UUID of the specified machine.
	// It returns a MachineNotFound if the machine does not exist.
	// It returns a MachineHasNoParent if the machine has no parent.
	GetMachineParentUUID(ctx context.Context, machineUUID string) (machine.UUID, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or
	// shutdown
	ShouldRebootOrShutdown(ctx context.Context, uuid machine.UUID) (machine.RebootAction, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// AppliedLXDProfileNames returns the names of the LXD profiles on the
	// machine.
	AppliedLXDProfileNames(ctx context.Context, mUUID string) ([]string, error)

	// SetAppliedLXDProfileNames sets the list of LXD profile names to the
	// lxd_profile table for the given machine. This method will overwrite the
	// list of profiles for the given machine without any checks.
	SetAppliedLXDProfileNames(ctx context.Context, mUUID string, profileNames []string) error

	// NamespaceForWatchMachineCloudInstance returns the namespace for watching
	// machine cloud instance changes.
	NamespaceForWatchMachineCloudInstance() string

	// NamespaceForWatchMachineLXDProfiles returns the namespace for watching
	// machine LXD profile changes.
	NamespaceForWatchMachineLXDProfiles() string

	// NamespaceForWatchMachineReboot returns the namespace string used for
	// tracking machine reboot events in the model.
	NamespaceForWatchMachineReboot() string

	// NamespaceForMachineLife returns the namespace string used for
	// tracking machine lifecycle events in the model.
	NamespaceForMachineLife() string

	// NamespaceForMachineAndMachineUnitLife returns the namespace string used
	// for tracking machine and machine unit lifecycle events in the model.
	NamespaceForMachineAndMachineUnitLife() (string, string)

	// InitialMachineContainerLifeStatement returns the table and the initial
	// watch statement for watching life changes of container machines.
	InitialMachineContainerLifeStatement() (string, string, func(string) string)

	// GetNamesForUUIDs returns a map of machine UUIDs to machine Names based
	// on the given machine UUIDs.
	// [machineerrors.MachineNotFound] will be returned if the machine does not
	// exist.
	GetNamesForUUIDs(ctx context.Context, machineUUIDs []string) (map[machine.UUID]machine.Name, error)

	// GetMachineArchesForApplication returns a map of machine names to their
	// instance IDs. This will ignore non-provisioned machines or container
	// machines.
	GetAllProvisionedMachineInstanceID(ctx context.Context) (map[machine.Name]string, error)

	// SetMachineHostname sets the hostname for the given machine.
	// Also updates the agent_started_at timestamp.
	SetMachineHostname(ctx context.Context, mUUID string, hostname string) error

	// GetSupportedContainersTypes returns the supported container types for the
	// given machine.
	GetSupportedContainersTypes(ctx context.Context, mUUID string) ([]string, error)

	// GetMachineContainers returns the names of the machines which have as
	// parent the specified machine.
	GetMachineContainers(ctx context.Context, mUUID string) ([]string, error)

	// GetMachinePrincipalApplications returns the names of the principal
	// (non-subordinate) applications for the specified machine.
	GetMachinePrincipalApplications(ctx context.Context, mName machine.Name) ([]string, error)

	// GetMachinePlacement returns the placement structure as it was recorded
	// for the given machine.
	GetMachinePlacementDirective(ctx context.Context, mName string) (*string, error)

	// GetMachineConstraints returns the constraints for the given machine.
	// Empty constraints are returned if no constraints exist for the given
	// machine.
	GetMachineConstraints(ctx context.Context, mName string) (constraints.Constraints, error)

	// GetMachineBase returns the base for the given machine. Since the
	// machine_platform table is populated when creating a machine, there should
	// always be a base for a machine.
	GetMachineBase(ctx context.Context, mName string) (base.Base, error)

	// GetModelConstraints returns the currently set constraints for the model.
	// Note: This method should mirror the model domain method of the same name.
	GetModelConstraints(ctx context.Context) (constraints.Constraints, error)

	// CountMachinesInSpace counts the number of machines with address in a
	// given space. This method counts the distinct occurrences of net nodes of
	// the addresses, meaning that if a machine has multiple addresses in the
	// same subnet it will be counted only once.
	CountMachinesInSpace(ctx context.Context, spUUID string) (int64, error)

	// GetSSHHostKeys returns the SSH host keys for the given machine.
	GetSSHHostKeys(ctx context.Context, mUUID string) ([]string, error)

	// SetSSHHostKeys sets the SSH host keys for the given machine.
	// This will overwrite the existing SSH host keys for the machine.
	SetSSHHostKeys(ctx context.Context, mUUID string, sshHostKeys []string) error

	// GetLXDProfilesForMachine returns a slice of structs containing data to
	// put LXD Profiles in LXC.
	GetLXDProfilesForMachine(ctx context.Context, mName string) ([]internal.CreateLXDProfileDetails, error)
}

// StatusHistory records status information into a generalized way.
type StatusHistory interface {
	// RecordStatus records the given status information.
	// If the status data cannot be marshalled, it will not be recorded, instead
	// the error will be logged under the data_error key.
	RecordStatus(context.Context, statushistory.Namespace, status.StatusInfo) error
}

// Service provides the API for working with machines.
type Service struct {
	st State

	statusHistory StatusHistory
	clock         clock.Clock
	logger        logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, statusHistory StatusHistory, clock clock.Clock, logger logger.Logger) *Service {
	return &Service{
		st:            st,
		statusHistory: statusHistory,
		clock:         clock,
		logger:        logger,
	}
}

// DeleteMachine deletes the specified machine.
func (s *Service) DeleteMachine(ctx context.Context, machineName machine.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.DeleteMachine(ctx, machineName); err != nil {
		return errors.Errorf("deleting machine %q: %w", machineName, err)
	}
	return nil
}

// GetMachineLife returns the GetMachineLife status of the specified machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) GetMachineLife(ctx context.Context, machineName machine.Name) (corelife.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	life, err := s.st.GetMachineLife(ctx, machineName)
	if err != nil {
		return corelife.Dead, errors.Errorf("getting life status for machine %q: %w", machineName, err)
	}

	return life.Value()
}

// AllMachineNames returns the names of all machines in the model.
func (s *Service) AllMachineNames(ctx context.Context) ([]machine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	machines, err := s.st.AllMachineNames(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving all machines: %w", err)
	}
	return machines, nil
}

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) IsMachineController(ctx context.Context, machineName machine.Name) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	isController, err := s.st.IsMachineController(ctx, machineName)
	if err != nil {
		return false, errors.Errorf("checking if machine %q is a controller: %w", machineName, err)
	}
	return isController, nil
}

// IsMachineManuallyProvisioned returns whether the machine is a manual machine.
func (s *Service) IsMachineManuallyProvisioned(ctx context.Context, machineName machine.Name) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	isManual, err := s.st.IsMachineManuallyProvisioned(ctx, machineName)
	if err != nil {
		return false, errors.Errorf("checking if machine %q is a manual machine: %w", machineName, err)
	}
	return isManual, nil
}

// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
// the corresponding cloud instance to be stopped.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) ShouldKeepInstance(ctx context.Context, machineName machine.Name) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
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
func (s *Service) SetKeepInstance(ctx context.Context, machineName machine.Name, keep bool) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return errors.Capture(s.st.SetKeepInstance(ctx, machineName, keep))
}

// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
func (s *Service) RequireMachineReboot(ctx context.Context, uuid machine.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.RequireMachineReboot(ctx, uuid); err != nil {
		return errors.Errorf("requiring a machine reboot for machine with uuid %q: %w", uuid, err)
	}
	return nil
}

// ClearMachineReboot removes the reboot flag of the machine referenced by its UUID if a reboot has previously been required.
func (s *Service) ClearMachineReboot(ctx context.Context, uuid machine.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.ClearMachineReboot(ctx, uuid); err != nil {
		return errors.Errorf("clear machine reboot flag for machine with uuid %q: %w", uuid, err)
	}
	return nil
}

// IsMachineRebootRequired checks if the machine referenced by its UUID requires a reboot.
func (s *Service) IsMachineRebootRequired(ctx context.Context, uuid machine.UUID) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
func (s *Service) GetMachineParentUUID(ctx context.Context, machineUUID machine.UUID) (machine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	parentUUID, err := s.st.GetMachineParentUUID(ctx, machineUUID.String())
	if err != nil {
		return "", errors.Errorf("retrieving parent UUID for machine %q: %w", machineUUID, err)
	}
	return parentUUID, nil
}

// ShouldRebootOrShutdown determines whether a machine should reboot or shutdown
func (s *Service) ShouldRebootOrShutdown(ctx context.Context, uuid machine.UUID) (machine.RebootAction, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	rebootRequired, err := s.st.ShouldRebootOrShutdown(ctx, uuid)
	if err != nil {
		return rebootRequired, errors.Errorf("getting if the machine with uuid %q need to reboot or shutdown: %w", uuid, err)
	}
	return rebootRequired, nil
}

// GetMachineUUID returns the UUID of a machine identified by its name.
// It returns a MachineNotFound if the machine does not exist.
func (s *Service) GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetMachineUUID(ctx, name)
}

// AppliedLXDProfileNames returns the names of the LXD profiles on the machine.
func (s *Service) AppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	profiles, err := s.st.AppliedLXDProfileNames(ctx, mUUID.String())
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
func (s *Service) SetAppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID, profileNames []string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return errors.Capture(s.st.SetAppliedLXDProfileNames(ctx, mUUID.String(), profileNames))
}

// GetMachineArchesForApplication returns a list of architectures which are
// included across the machines of the given application.
//
// TODO: Implement this method.
func (s *Service) GetMachineArchesForApplication(ctx context.Context, appUUID application.ID) ([]arch.Arch, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return nil, errors.Errorf("GetMachineArchesForApplication not implemented")
}

// GetAllProvisionedMachineInstanceID returns all provisioned machine
// instance IDs in the model.
func (s *Service) GetAllProvisionedMachineInstanceID(ctx context.Context) (map[machine.Name]instance.Id, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	m, err := s.st.GetAllProvisionedMachineInstanceID(ctx)
	if err != nil {
		return nil, errors.Errorf("getting all provisioned machine instance IDs: %w", err)
	}

	result := make(map[machine.Name]instance.Id, len(m))
	for name, id := range m {
		result[name] = instance.Id(id)
	}
	return result, nil
}

// SetMachineHostname sets the hostname for the given machine.
// Also updates the agent_started_at timestamp for a machine.
func (s *Service) SetMachineHostname(ctx context.Context, mUUID machine.UUID, hostname string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := mUUID.Validate(); err != nil {
		return errors.Errorf("validating machine UUID %q: %w", mUUID, err)
	}

	if err := s.st.SetMachineHostname(ctx, mUUID.String(), hostname); err != nil {
		return errors.Errorf("setting hostname for machine with UUID %q: %w", mUUID, err)
	}
	return nil
}

// GetSupportedContainersTypes returns the supported container types for the
// provider.
// This will always return LXD as the only supported container type. This
// is hardcoded for now.
func (s *Service) GetSupportedContainersTypes(ctx context.Context, mUUID machine.UUID) ([]instance.ContainerType, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := mUUID.Validate(); err != nil {
		return nil, errors.Errorf("validating machine UUID %q: %w", mUUID, err)
	}

	var results []instance.ContainerType
	containerTypes, err := s.st.GetSupportedContainersTypes(ctx, mUUID.String())
	if err != nil {
		return nil, errors.Errorf("getting supported container types for machine with UUID %q: %w", mUUID, err)
	}
	for _, containerType := range containerTypes {
		if containerType == "lxd" {
			results = append(results, instance.LXD)
		} else {
			return nil, errors.Errorf("unknown container type %q for machine with UUID %q", containerType, mUUID)
		}
	}
	return results, nil
}

// GetMachineContainers returns the names of the machines which have as parent
// the specified machine.
func (s *Service) GetMachineContainers(ctx context.Context, machineName machine.Name) ([]machine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return nil, errors.Errorf("validating machine name %q: %w", machineName, err)
	}

	containers, err := s.st.GetMachineContainers(ctx, machineName.String())
	if err != nil {
		return nil, errors.Errorf("getting machine containers for machine %q: %w", machineName, err)
	}

	return transform.Slice(containers, func(v string) machine.Name {
		return machine.Name(v)
	}), nil
}

// GetMachinePrincipalApplications returns the names of the principal (non-subordinate)
// units for the specified machine.
func (s *Service) GetMachinePrincipalApplications(ctx context.Context, mName machine.Name) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := mName.Validate(); err != nil {
		return nil, errors.Errorf("validating machine name %q: %w", mName, err)
	}

	return s.st.GetMachinePrincipalApplications(ctx, mName)
}

// GetMachinePlacement returns the placement structure as it was recorded for
// the given machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
func (s *Service) GetMachinePlacementDirective(ctx context.Context, mName machine.Name) (*string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := mName.Validate(); err != nil {
		return nil, errors.Errorf("validating machine name %q: %w", mName, err)
	}

	return s.st.GetMachinePlacementDirective(ctx, mName.String())
}

// GetMachineConstraints returns the constraints for the given machine.
// Empty constraints are returned if no constraints exist for the given
// machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
func (s *Service) GetMachineConstraints(ctx context.Context, mName machine.Name) (coreconstraints.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := mName.Validate(); err != nil {
		return coreconstraints.Value{}, errors.Errorf("validating machine name %q: %w", mName, err)
	}

	cons, err := s.st.GetMachineConstraints(ctx, mName.String())
	return constraints.EncodeConstraints(cons), errors.Capture(err)
}

// GetMachineBase returns the base for the given machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
func (s *Service) GetMachineBase(ctx context.Context, mName machine.Name) (base.Base, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := mName.Validate(); err != nil {
		return base.Base{}, errors.Errorf("validating machine name %q: %w", mName, err)
	}

	return s.st.GetMachineBase(ctx, mName.String())
}

// CountMachinesInSpace counts the number of machines with address in a given
// space. This method counts the distinct occurrences of net nodes of the
// addresses, meaning that if a machine has multiple addresses in the same
// subnet it will be counted only once.
func (s *Service) CountMachinesInSpace(ctx context.Context, spaceID network.SpaceUUID) (int64, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := spaceID.Validate(); err != nil {
		return 0, errors.Errorf("validating space UUID %q: %w", spaceID, err)
	}

	return s.st.CountMachinesInSpace(ctx, spaceID.String())
}

// GetSSHHostKeys returns the SSH host keys for the given machine UUID.
func (s *Service) GetSSHHostKeys(ctx context.Context, mUUID machine.UUID) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := mUUID.Validate(); err != nil {
		return nil, errors.Errorf("validating machine UUID %q: %w", mUUID, err)
	}

	keys, err := s.st.GetSSHHostKeys(ctx, mUUID.String())
	if err != nil {
		return nil, errors.Errorf("getting SSH host keys for machine with UUID %q: %w", mUUID, err)
	}
	return keys, nil
}

// SetSSHHostKeys sets the SSH host keys for the given machine UUID.
func (s *Service) SetSSHHostKeys(ctx context.Context, mUUID machine.UUID, keys []string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := mUUID.Validate(); err != nil {
		return errors.Errorf("validating machine UUID %q: %w", mUUID, err)
	}

	if err := s.st.SetSSHHostKeys(ctx, mUUID.String(), keys); err != nil {
		return errors.Errorf("setting SSH host keys for machine with UUID %q: %w", mUUID, err)
	}
	return nil
}

func recordCreateMachineStatusHistory(ctx context.Context, statusHistory StatusHistory, machineName machine.Name, clock clock.Clock) error {
	info := status.StatusInfo{
		Status: status.Pending,
		Since:  ptr(clock.Now()),
	}

	if err := statusHistory.RecordStatus(ctx, domainstatus.MachineNamespace.WithID(machineName.String()), info); err != nil {
		return errors.Errorf("recording machine status history: %w", err)
	}
	if err := statusHistory.RecordStatus(ctx, domainstatus.MachineInstanceNamespace.WithID(machineName.String()), info); err != nil {
		return errors.Errorf("recording instance status history: %w", err)
	}
	return nil
}

// createUUIDs generates a new UUID for the machine and the net-node.
func createUUIDs() (machine.UUID, error) {
	machineUUID, err := machine.NewUUID()
	if err != nil {
		return "", errors.Errorf("generating machine UUID: %w", err)
	}
	return machineUUID, nil
}

func ptr[T any](v T) *T {
	return &v
}
