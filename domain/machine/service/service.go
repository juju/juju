// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"

	"github.com/juju/clock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence methods for machines.
type State interface {
	// CreateMachine persists the input machine entity.
	// It returns a MachineAlreadyExists error if a machine with the same name
	// already exists.
	CreateMachine(context.Context, machine.Name, string, machine.UUID, *string) error

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

	// InitialWatchModelMachinesStatement returns the table and the initial
	// watch statement for watching life changes of non-container machines.
	InitialWatchModelMachinesStatement() (string, string)

	// GetMachineLife returns the life status of the specified machine.
	// It returns a MachineNotFound if the given machine doesn't exist.
	GetMachineLife(context.Context, machine.Name) (life.Life, error)

	// SetMachineLife sets the life status of the specified machine.
	// It returns a MachineNotFound if the provided machine doesn't exist.
	SetMachineLife(context.Context, machine.Name, life.Life) error

	// AllMachineNames retrieves the names of all machines in the model.
	// If there's no machine, it returns an empty slice.
	AllMachineNames(context.Context) ([]machine.Name, error)

	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(context.Context, machine.UUID) (string, error)

	// GetInstanceIDAndName returns the cloud specific instance ID and display name
	// for this machine.
	GetInstanceIDAndName(ctx context.Context, mUUID machine.UUID) (string, string, error)

	// GetInstanceStatus returns the cloud specific instance status for this
	// machine.
	// It returns MachineNotFound if the machine does not exist.
	// It returns a StatusNotSet if the instance status is not set.
	GetInstanceStatus(context.Context, machine.Name) (domainstatus.StatusInfo[domainstatus.InstanceStatusType], error)

	// SetInstanceStatus sets the cloud specific instance status for this
	// machine.
	// It returns MachineNotFound if the machine does not exist.
	SetInstanceStatus(context.Context, machine.UUID, domainstatus.StatusInfo[domainstatus.InstanceStatusType]) error

	// GetMachineStatus returns the status of the specified machine.
	// It returns MachineNotFound if the machine does not exist.
	// It returns a StatusNotSet if the status is not set.
	GetMachineStatus(context.Context, machine.Name) (domainstatus.StatusInfo[domainstatus.MachineStatusType], error)

	// SetMachineStatus sets the status of the specified machine.
	// It returns MachineNotFound if the machine does not exist.
	SetMachineStatus(context.Context, machine.Name, domainstatus.StatusInfo[domainstatus.MachineStatusType]) error

	// GetHardwareCharacteristics returns the hardware characteristics struct with
	// data retrieved from the machine cloud instance table.
	GetHardwareCharacteristics(context.Context, machine.UUID) (*instance.HardwareCharacteristics, error)

	// AvailabilityZone returns the availability zone for the specified machine.
	AvailabilityZone(context.Context, machine.UUID) (string, error)

	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(context.Context, machine.UUID, instance.Id, string, string, *instance.HardwareCharacteristics) error

	// SetRunningAgentBinaryVersion sets the running agent version for the
	// machine. A MachineNotFound error will be returned if the machine does not
	// exist.
	SetRunningAgentBinaryVersion(context.Context, machine.UUID, coreagentbinary.Version) error

	// DeleteMachineCloudInstance removes an entry in the machine cloud instance
	// table along with the instance tags and the link to a lxd profile if any.
	DeleteMachineCloudInstance(context.Context, machine.UUID) error

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
	GetMachineParentUUID(ctx context.Context, machineUUID machine.UUID) (machine.UUID, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or
	// shutdown
	ShouldRebootOrShutdown(ctx context.Context, uuid machine.UUID) (machine.RebootAction, error)

	// MarkMachineForRemoval marks the given machine for removal.
	// It returns a MachineNotFound error if the machine does not exist.
	MarkMachineForRemoval(context.Context, machine.Name) error

	// GetAllMachineRemovals returns the UUIDs of all of the machines that need
	// to be removed but need provider-level cleanup.
	GetAllMachineRemovals(context.Context) ([]machine.UUID, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// AppliedLXDProfileNames returns the names of the LXD profiles on the
	// machine.
	AppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID) ([]string, error)

	// SetAppliedLXDProfileNames sets the list of LXD profile names to the
	// lxd_profile table for the given machine. This method will overwrite the
	// list of profiles for the given machine without any checks.
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

	// GetMachineArchesForApplication returns a map of machine names to their
	// instance IDs. This will ignore non-provisioned machines or container
	// machines.
	GetAllProvisionedMachineInstanceID(ctx context.Context) (map[string]string, error)

	// SetMachineHostname sets the hostname for the given machine.
	// Also updates the agent_started_at timestamp.
	SetMachineHostname(ctx context.Context, mUUID machine.UUID, hostname string) error

	// GetSupportedContainersTypes returns the supported container types for the
	// given machine.
	GetSupportedContainersTypes(ctx context.Context, mUUID machine.UUID) ([]string, error)
}

// StatusHistory records status information into a generalized way.
type StatusHistory interface {
	// RecordStatus records the given status information.
	// If the status data cannot be marshalled, it will not be recorded, instead
	// the error will be logged under the data_error key.
	RecordStatus(context.Context, statushistory.Namespace, status.StatusInfo) error
}

// Provider represents an underlying cloud provider.
type Provider interface {
	environs.BootstrapEnviron
	environs.InstanceTypesFetcher
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

// CreateMachine creates the specified machine.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (s *Service) CreateMachine(ctx context.Context, machineName machine.Name, nonce *string) (machine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Make a new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, machineUUID, err := createUUIDs()
	if err != nil {
		return "", errors.Errorf("creating machine %q: %w", machineName, err)
	}

	err = s.st.CreateMachine(ctx, machineName, nodeUUID, machineUUID, nonce)
	if err != nil {
		return machineUUID, errors.Errorf("creating machine %q: %w", machineName, err)
	}

	if err := s.recordCreateMachineStatusHistory(ctx, machineName); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return machineUUID, nil
}

// CreateMachineWithParent creates the specified machine with the specified
// parent.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
// It returns a MachineNotFound error if the parent machine does not exist.
func (s *Service) CreateMachineWithParent(ctx context.Context, machineName, parentName machine.Name) (machine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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

	if err := s.recordCreateMachineStatusHistory(ctx, machineName); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return machineUUID, nil
}

func (s *Service) recordCreateMachineStatusHistory(ctx context.Context, machineName machine.Name) error {
	info := status.StatusInfo{
		Status: status.Pending,
		Since:  ptr(s.clock.Now()),
	}

	if err := s.statusHistory.RecordStatus(ctx, domainstatus.MachineNamespace.WithID(machineName.String()), info); err != nil {
		return errors.Errorf("recording machine status history: %w", err)
	}
	if err := s.statusHistory.RecordStatus(ctx, domainstatus.MachineInstanceNamespace.WithID(machineName.String()), info); err != nil {
		return errors.Errorf("recording instance status history: %w", err)
	}
	return nil
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

// SetMachineLife sets the life status of the specified machine.
// It returns a MachineNotFound if the provided machine doesn't exist.
func (s *Service) SetMachineLife(ctx context.Context, machineName machine.Name, life life.Life) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.SetMachineLife(ctx, machineName, life); err != nil {
		return errors.Errorf("setting life status for machine %q: %w", machineName, err)
	}
	return nil
}

// EnsureDeadMachine sets the provided machine's life status to Dead.
// No error is returned if the provided machine doesn't exist, just nothing gets
// updated.
func (s *Service) EnsureDeadMachine(ctx context.Context, machineName machine.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.SetMachineLife(ctx, machineName, life.Dead)
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

// GetInstanceStatus returns the cloud specific instance status for this
// machine.
// It returns MachineNotFound if the machine does not exist.
// It returns a StatusNotSet if the instance status is not set.
// Idempotent.
func (s *Service) GetInstanceStatus(ctx context.Context, machineName machine.Name) (status.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	instanceStatus, err := s.st.GetInstanceStatus(ctx, machineName)
	if err != nil {
		return status.StatusInfo{}, errors.Errorf("retrieving instance status for machine %q: %w", machineName, err)
	}

	return decodeInstanceStatus(instanceStatus)
}

// SetInstanceStatus sets the cloud specific instance status for this machine.
// It returns MachineNotFound if the machine does not exist. It returns
// InvalidStatus if the given status is not a known status value.
func (s *Service) SetInstanceStatus(ctx context.Context, machineName machine.Name, statusInfo status.StatusInfo) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return errors.Errorf("validating machine name %q: %w", machineName, err)
	}

	if !statusInfo.Status.KnownInstanceStatus() {
		return machineerrors.InvalidStatus
	}

	machineUUID, err := s.st.GetMachineUUID(ctx, machineName)
	if err != nil {
		return errors.Errorf("getting machine uuid for %q: %w", machineName, err)
	}

	instanceStatus, err := encodeInstanceStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding status for machine %q: %w", machineName, err)
	}

	if err := s.st.SetInstanceStatus(ctx, machineUUID, instanceStatus); err != nil {
		return errors.Errorf("setting instance status for machine %q: %w", machineName, err)
	}

	if err := s.statusHistory.RecordStatus(ctx, domainstatus.MachineInstanceNamespace.WithID(machineName.String()), statusInfo); err != nil {
		s.logger.Infof(ctx, "failed recording instance status history: %w", err)
	}

	return nil
}

// GetMachineStatus returns the status of the specified machine. It returns
// MachineNotFound if the machine does not exist. It returns a StatusNotSet if
// the status is not set. Idempotent.
func (s *Service) GetMachineStatus(ctx context.Context, machineName machine.Name) (status.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
func (s *Service) SetMachineStatus(ctx context.Context, machineName machine.Name, statusInfo status.StatusInfo) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return errors.Errorf("validating machine name %q: %w", machineName, err)
	}

	if !statusInfo.Status.KnownMachineStatus() {
		return machineerrors.InvalidStatus
	}

	machineStatus, err := encodeMachineStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding status for machine %q: %w", machineName, err)
	}

	if err := s.st.SetMachineStatus(ctx, machineName, machineStatus); err != nil {
		return errors.Errorf("setting machine status for machine %q: %w", machineName, err)
	}

	if err := s.statusHistory.RecordStatus(ctx, domainstatus.MachineNamespace.WithID(machineName.String()), statusInfo); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return nil
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

	parentUUID, err := s.st.GetMachineParentUUID(ctx, machineUUID)
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

// MarkMachineForRemoval marks the given machine for removal.
// It returns a MachineNotFound error if the machine does not exist.
func (s *Service) MarkMachineForRemoval(ctx context.Context, machineName machine.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.st.MarkMachineForRemoval(ctx, machineName); err != nil {
		return errors.Errorf("marking machine %q for removal: %w", machineName, err)
	}
	return nil
}

// GetAllMachineRemovals returns the UUIDs of all of the machines that need to
// be removed but need provider-level cleanup.
func (s *Service) GetAllMachineRemovals(ctx context.Context) ([]machine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	removals, err := s.st.GetAllMachineRemovals(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving all machines marked to be removed: %w", err)
	}
	return removals, nil
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
func (s *Service) SetAppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID, profileNames []string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return errors.Capture(s.st.SetAppliedLXDProfileNames(ctx, mUUID, profileNames))
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
		result[machine.Name(name)] = instance.Id(id)
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

	if err := s.st.SetMachineHostname(ctx, mUUID, hostname); err != nil {
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
	containerTypes, err := s.st.GetSupportedContainersTypes(ctx, mUUID)
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

// ProviderService provides the API for working with machines using the
// underlying provider.
type ProviderService struct {
	Service

	providerGetter providertracker.ProviderGetter[Provider]
}

// GetBootstrapEnviron returns the bootstrap environ.
func (s *ProviderService) GetBootstrapEnviron(ctx context.Context) (environs.BootstrapEnviron, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return provider, nil
}

// GetInstanceTypesFetcher returns the instance types fetcher.
func (s *ProviderService) GetInstanceTypesFetcher(ctx context.Context) (environs.InstanceTypesFetcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return provider, nil
}

func ptr[T any](v T) *T {
	return &v
}
