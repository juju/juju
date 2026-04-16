// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net"

	"github.com/juju/proxy"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationinternal "github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/application/service/storage"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

// UnitState describes retrieval and persistence methods for
// units.
type UnitState interface {
	// AddIAASUnits adds the specified units to the application, returning their
	// names.
	AddIAASUnits(context.Context, coreapplication.UUID, ...application.AddIAASUnitArg) ([]coreunit.Name, []coremachine.Name, error)

	// AddCAASUnits adds the specified units to the application, returning their
	// names.
	AddCAASUnits(context.Context, coreapplication.UUID, ...application.AddCAASUnitArg) ([]coreunit.Name, error)

	// GetCAASUnitRegistered checks if a caas unit by the provided name is
	// already registered in the model. False is returned when no unit exists,
	// otherwise the units existing uuid and netnode uuid is returned.
	GetCAASUnitRegistered(
		context.Context, coreunit.Name,
	) (bool, coreunit.UUID, domainnetwork.NetNodeUUID, error)

	// InsertMigratingIAASUnits inserts the fully formed units for the specified
	// IAAS application. This is only used when inserting units during model
	// migration. If the application is not found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If any of the units
	// already exists, an error satisfying [applicationerrors.UnitAlreadyExists]
	// is returned.
	InsertMigratingIAASUnits(context.Context, coreapplication.UUID, ...application.ImportIAASUnitArg) error

	// InsertMigratingCAASUnits inserts the fully formed units for the specified
	// CAAS application. This is only used when inserting units during model
	// migration. If the application is not found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If any of the units
	// already exists, an error satisfying [applicationerrors.UnitAlreadyExists]
	// is returned.
	InsertMigratingCAASUnits(context.Context, coreapplication.UUID, ...application.ImportCAASUnitArg) error

	// RegisterCAASUnit registers the specified CAAS application unit. The
	// following errors can be expected:
	// [applicationerrors.ApplicationNotAlive] when the application is not alive
	// [applicationerrors.UnitAlreadyExists] when the unit exists
	// [applicationerrors.UnitNotAssigned] when the unit was not assigned
	RegisterCAASUnit(context.Context, string, application.RegisterCAASUnitArg) error

	// UpdateCAASUnit updates the cloud container for specified unit,
	// returning an error satisfying [applicationerrors.UnitNotFoundError]
	// if the unit doesn't exist.
	UpdateCAASUnit(context.Context, coreunit.Name, application.UpdateCAASUnitParams) error

	// UpdateUnitCharm sets the currently running charm marker for the given
	// unit, adding the specified new storage requirements and removing the old
	// unit storage directives.
	UpdateUnitCharm(
		ctx context.Context,
		arg applicationinternal.UpdateUnitCharmArg,
	) error

	// GetUnitStorageDirectivesCurrentNext returns the current and the next storage
	// directives for this unit, if the unit was to switch to the given charm.
	GetUnitStorageRefreshArgs(
		ctx context.Context, unit coreunit.UUID, next corecharm.ID,
	) (applicationinternal.UnitStorageRefreshArgs, error)

	// GetUnitOwnedStorageInstances returns the storage compositions for all
	// storage instances owned by the unit in the model. If the unit does not
	// currently own any storage instances then an empty result is returned.
	GetUnitOwnedStorageInstances(
		ctx context.Context,
		unitUUID coreunit.UUID,
	) (
		[]applicationinternal.StorageInstanceInfoForAttach,
		[]applicationinternal.StorageAttachmentComposition,
		error,
	)

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// GetUnitNetNodeUUID returns the net node UUID for the specified unit.
	// The following error types can be expected:
	// - [applicationerrors.UnitNotFound]: when the unit is not found.
	GetUnitNetNodeUUID(ctx context.Context, uuid coreunit.UUID) (string, error)

	// GetUnitUUIDAndNetNodeForName returns the unit uuid and net node uuid for a
	// unit matching the supplied name.
	//
	// The following errors may be expected:
	// - [applicationerrors.UnitNotFound] if no unit exists for the supplied
	// name.
	GetUnitUUIDAndNetNodeForName(
		context.Context, coreunit.Name,
	) (coreunit.UUID, domainnetwork.NetNodeUUID, error)

	// GetUnitLife looks up the life of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
	GetUnitLife(context.Context, coreunit.Name) (life.Life, error)

	// GetUnitPrincipal gets the subordinates principal unit. If no principal unit
	// is found, for example, when the unit is not a subordinate, then false is
	// returned.
	GetUnitPrincipal(ctx context.Context, unitName coreunit.Name) (coreunit.Name, bool, error)

	// GetUnitMachineUUID gets the unit's machine uuid. If the unit does not
	// have a machine assigned, [applicationerrors.UnitMachineNotAssigned] is
	// returned.
	GetUnitMachineUUID(ctx context.Context, unitUUID string) (string, error)

	// GetUnitMachineName gets the unit's machine uuid. If the unit does not
	// have a machine assigned, [applicationerrors.UnitMachineNotAssigned] is
	// returned.
	GetUnitMachineName(ctx context.Context, unitUUID string) (string, error)

	// GetAllUnitLifeForApplication returns a map of the unit names and their lives
	// for the given application.
	//   - If the application is not found, [applicationerrors.ApplicationNotFound]
	//     is returned.
	GetAllUnitLifeForApplication(context.Context, coreapplication.UUID) (map[string]int, error)

	// GetMachineUUIDAndNetNodeForName is responsible for identifying the uuid
	// and net node for a machine by it's name.
	//
	// The following errors may be expected:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the supplied machine name.
	GetMachineUUIDAndNetNodeForName(
		context.Context, string,
	) (coremachine.UUID, domainnetwork.NetNodeUUID, error)

	// GetModelConstraints returns the currently set constraints for the model.
	// The following error types can be expected:
	// - [modelerrors.NotFound]: when no model exists to set constraints for.
	// - [modelerrors.ConstraintsNotFound]: when no model constraints have been
	// set for the model.
	// Note: This method should mirror the model domain method of the same name.
	GetModelConstraints(context.Context) (constraints.Constraints, error)

	// GetUnitRefreshAttributes returns the unit refresh attributes for the
	// specified unit. If the unit is not found, an error satisfying
	// [applicationerrors.UnitNotFound] is returned.
	// This doesn't take into account life, so it can return the life of a unit
	// even if it's dead.
	GetUnitRefreshAttributes(context.Context, coreunit.Name) (application.UnitAttributes, error)

	// GetUnitK8sPodInfo returns information about the k8s pod for the given unit.
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist
	// - [applicationerrors.UnitIsDead] if the unit is dead
	GetUnitK8sPodInfo(context.Context, coreunit.Name) (application.K8sPodInfo, error)

	// GetUnitsK8sPodInfo returns information about the k8s pods for all alive units.
	GetUnitsK8sPodInfo(ctx context.Context) (map[coreunit.Name]application.K8sPodInfo, error)

	// GetAllUnitNames returns a slice of all unit names in the model.
	GetAllUnitNames(context.Context) ([]coreunit.Name, error)

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	// The following errors may be returned:
	// - [applicationerrors.ApplicationIsDead] if the application is dead
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	GetUnitNamesForApplication(context.Context, coreapplication.UUID) ([]coreunit.Name, error)

	// GetUnitNamesForNetNode returns a slice of the unit names for the given net node
	GetUnitNamesForNetNode(context.Context, string) ([]coreunit.Name, error)

	// GetMachineNetNodeUUIDFromName returns the net node UUID for the named
	// machine. The following errors may be returned: -
	// [applicationerrors.MachineNotFound] if the machine does not exist
	GetMachineNetNodeUUIDFromName(context.Context, coremachine.Name) (string, error)

	// SetUnitWorkloadVersion sets the workload version for the given unit.
	SetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name, version string) error

	// GetUnitWorkloadVersion returns the workload version for the given unit.
	GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error)

	// GetUnitSubordinates returns the names of all the subordinate units of the
	// given principal unit.
	GetUnitSubordinates(ctx context.Context, unitName coreunit.Name) ([]coreunit.Name, error)

	// GetUnitNetNodesByName returns the net node UUIDs associated with the
	// specified unit. The net nodes are selected in the same way as in
	// GetUnitAddresses, i.e. the union of the net nodes of the cloud service (if
	// any) and the net node of the unit.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	GetUnitNetNodesByName(ctx context.Context, name coreunit.Name) ([]string, error)

	// GetAllUnitCloudContainerIDsForApplication returns a map of the unit names
	// and their cloud container provider IDs for the given application.
	//   - If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
	//   - If the application is not found, [applicationerrors.ApplicationNotFound]
	//     is returned.
	GetAllUnitCloudContainerIDsForApplication(context.Context, coreapplication.UUID) (map[coreunit.Name]string, error)

	// GetStorageAddInfoByUnitUUID returns the deploy metadata and how many
	// storage instances exist for the named storage on the specified unit.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.StorageNameNotSupported]: when storage name is not defined in charm metadata.
	GetStorageAddInfoByUnitUUID(
		context.Context,
		coreunit.UUID,
		corestorage.Name,
	) (applicationinternal.StorageInfoForAdd, error)

	// GetStorageAttachInfoByUnitUUIDAndStorageUUID returns the metadata
	// and select details for the storage instance on the specified unit.
	// The details include how many existing instances of the same named storage
	// already exist, the requested size, and the instance's storage pool.
	//
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit does not exist.
	// - [storageerrors.StorageInstanceNotFound] when the storage instance does not
	// exist.
	// - [applicationerrors.StorageNameNotSupported] when the unit's charm does not
	// define the storage name in use by the storage instance.
	GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		ctx context.Context,
		unitUUID coreunit.UUID,
		storageUUID domainstorage.StorageInstanceUUID,
	) (applicationinternal.StorageInstanceInfoForUnitAttach, error)

	// GetStorageAttachInfoForStorageInstances returns attachment metadata for
	// the specified storage instances.
	//
	// The following errors can be expected:
	// - [storageerrors.StorageInstanceNotFound] when a storage instance does not
	// exist.
	GetStorageAttachInfoForStorageInstances(
		ctx context.Context,
		storageInstanceUUIDs []domainstorage.StorageInstanceUUID,
	) ([]applicationinternal.StorageInstanceInfoForAttach, error)

	// AddStorageForCAASUnit adds storage instances to given unit as specified.
	// The specified storage name is used to retrieve existing storage instances.
	// Combination of existing storage instances and anticipated additional storage
	// instances is validated as specified in the unit's charm.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the
	// unit does not exist.
	// - [github.com/juju/juju/domain/application/errors.UnitNotAlive]: when the
	// unit is not alive.
	// - [github.com/juju/juju/domain/application/errors.StorageNameNotSupported]:
	// when storage name is not defined in charm metadata.
	// - [github.com/juju/juju/domain/application/errors.StorageCountLimitExceeded]
	// when the requested storage falls outside of the bounds defined by the charm.
	AddStorageForCAASUnit(
		ctx context.Context, unitUUID coreunit.UUID, storageName corestorage.Name,
		storageArg applicationinternal.AddStorageToUnitArg,
	) ([]corestorage.ID, error)

	// AddStorageForIAASUnit adds storage instances to given IAAS unit as specified.
	// The specified storage name is used to retrieve existing storage instances.
	// Combination of existing storage instances and anticipated additional storage
	// instances is validated as specified in the unit's charm.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound]: when the
	// unit does not exist.
	// - [github.com/juju/juju/domain/application/errors.UnitNotAlive]: when the
	// unit is not alive.
	// - [github.com/juju/juju/domain/application/errors.StorageNameNotSupported]:
	// when storage name is not defined in charm metadata.
	// - [github.com/juju/juju/domain/application/errors.StorageCountLimitExceeded]
	// when the requested storage falls outside of the bounds defined by the charm.
	AddStorageForIAASUnit(
		ctx context.Context, unitUUID coreunit.UUID, storageName corestorage.Name,
		storageArg applicationinternal.AddStorageToIAASUnitArg,
	) ([]corestorage.ID, error)

	// GetIAASUnitContext returns the IAAS context information required for the
	// construction of a context factory for a unit.
	GetIAASUnitContext(ctx context.Context, unitName string) (applicationinternal.IAASUnitContext, error)

	// GetCAASUnitContext returns the CAAS context information required for the
	// construction of a context factory for a unit.
	GetCAASUnitContext(ctx context.Context, unitName string) (applicationinternal.CAASUnitContext, error)

	// AttachStorageInstanceToUnit attaches an existing storage instance to a unit.
	// The following error types can be expected:
	// - [storageerrors.StorageInstanceNotFound] when the storage instance does
	// not exist.
	// - [storageerrors.StorageInstanceNotAlive] when the storage instance is not
	// alive.
	// - [applicationerrors.UnitNotFound] when the unit does not exist.
	// - [applicationerrors.UnitNotAlive] when the unit is not alive.
	// - [applicationerrors.StorageInstanceAlreadyAttachedToUnit] when the
	// storage instance is already attached to the unit.
	// - [applicationerrors.UnitAttachmentCountExceedsLimit] when the unit
	// already has too many attachments for the storage name.
	// - [applicationerrors.UnitCharmChanged] when the unit's charm has changed.
	// - [applicationerrors.UnitMachineChanged] when the unit's machine has
	// changed.
	// - [applicationerrors.StorageInstanceUnexpectedAttachments] when the
	// storage instance has attachments outside the expected set.
	AttachStorageInstanceToUnit(
		ctx context.Context,
		unitUUID coreunit.UUID,
		storageArg applicationinternal.AttachStorageInstanceToUnitArg,
	) error
}

// AttachStorageInstanceToUnit ensures the specified storage instance can be
// attached to the specified unit and then attaches it.
//
// The following error types can be expected:
// - [coreerrors.NotValid] when the storage or unit UUID is not valid.
// - [storageerrors.StorageInstanceNotFound] when the storage instance does not
// exist.
// - [storageerrors.StorageInstanceNotAlive] when the storage instance is not
// alive.
// - [applicationerrors.UnitNotFound] when the unit does not exist.
// - [applicationerrors.UnitNotAlive] when the unit is not alive.
// - [applicationerrors.StorageNameNotSupported] when the unit's charm does not
// define the storage name.
// - [applicationerrors.StorageInstanceCharmNameMismatch] when the storage
// instance charm name does not match the unit charm.
// - [applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition]
// when the storage kind does not match the charm storage definition.
// - [applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition]
// when the storage size is below the charm minimum.
// - [applicationerrors.StorageCountLimitExceeded] when attaching would exceed
// the charm storage maximum count, including when a concurrent attachment has
// caused the count to be exceeded since validation.
// - [applicationerrors.StorageInstanceAlreadyAttachedToUnit] when the storage
// instance is already attached to the unit.
// - [applicationerrors.StorageInstanceAttachSharedAccessNotSupported] when the
// storage instance has existing attachments but the unit's charm storage
// definition does not support shared access.
// - [applicationerrors.StorageInstanceUnexpectedAttachments] when the storage
// instance attachments changed concurrently during the attach operation.
// - [applicationerrors.StorageInstanceAttachMachineOwnerMismatch] when the
// storage instance owning machine does not match the unit's machine.
// - [applicationerrors.UnitCharmChanged] when the unit's charm has changed
// concurrently during the attach operation.
// - [applicationerrors.UnitMachineChanged] when the unit's machine has changed
// concurrently during the attach operation.
func (s *ProviderService) AttachStorageInstanceToUnit(
	ctx context.Context, storageUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := storageUUID.Validate(); err != nil {
		return errors.Errorf("storage uuid is not valid: %w", err).Add(
			coreerrors.NotValid)
	}
	if err := unitUUID.Validate(); err != nil {
		return errors.Errorf("unit uuid is not valid: %w", err).Add(
			coreerrors.NotValid)
	}

	storageAttachInfo, err := s.st.GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		ctx, unitUUID, storageUUID,
	)
	if err != nil {
		return errors.Errorf(
			"getting unit %q info and storage instance %q info for attachment: %w",
			unitUUID, storageUUID, err,
		)
	}

	// Can this storage instance be attached to this unit?
	err = validateStorageInstanceForUnitAttachment(ctx, storageAttachInfo)
	if err != nil {
		return err
	}

	// Generate the new storage instance attachment arg.
	unitAttachStorageArg, err := s.storageService.MakeAttachStorageInstanceToUnitArg(
		ctx,
		storageAttachInfo,
	)
	if err != nil {
		return errors.Errorf(
			"making attach storage instance arguments: %w", err,
		)
	}

	err = s.st.AttachStorageInstanceToUnit(ctx, unitUUID, unitAttachStorageArg)
	if errors.Is(err, applicationerrors.UnitAttachmentCountExceedsLimit) {
		charmStorageDef := storageAttachInfo.UnitNamedStorageInfo.CharmStorageDefinitionForValidation
		countExceededErr := applicationerrors.StorageCountLimitExceeded{
			Minimum:     charmStorageDef.CountMin,
			Requested:   int(storageAttachInfo.UnitNamedStorageInfo.AlreadyAttachedCount) + 1,
			StorageName: charmStorageDef.Name,
		}
		if charmStorageDef.CountMax >= 0 {
			countExceededErr.Maximum = &charmStorageDef.CountMax
		}
		return countExceededErr
	}
	return errors.Capture(err)
}

// validateStorageInstanceForUnitAttachment validates whether a storage
// instance can be attached to a unit based on unit state, charm storage
// definition, and existing attachments.
//
// The following errors may be returned:
// - [storageerrors.StorageInstanceNotAlive] when the storage instance is not alive.
// - [applicationerrors.UnitNotAlive] when the unit is not alive.
// - [applicationerrors.StorageInstanceCharmNameMismatch] when the storage
// instance charm name does not match the unit charm.
// - [applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition]
// when the storage instance kind does not match the charm storage definition.
// - [applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition]
// when the storage instance size is below the charm storage minimum.
// - [applicationerrors.StorageCountLimitExceeded] when attaching would exceed
// the charm storage maximum.
// - [applicationerrors.StorageInstanceAlreadyAttachedToUnit] when the storage
// instance is already attached to the unit.
// - [applicationerrors.StorageInstanceUnexpectedAttachments] when the charm
// storage definition is not shared and the storage instance has existing
// attachments.
// - [applicationerrors.StorageInstanceAttachMachineOwnerMismatch] when the
// storage instance owning machine does not match the unit's machine.
func validateStorageInstanceForUnitAttachment(
	ctx context.Context,
	info applicationinternal.StorageInstanceInfoForUnitAttach,
) error {
	// Validate that the storage instance is alive.
	if info.StorageInstanceInfo.Life != life.Alive {
		return errors.Errorf(
			"storage instance %q is not alive",
			info.StorageInstanceInfo.UUID,
		).Add(storageerrors.StorageInstanceNotAlive)
	}

	// Validate that the unit is alive.
	if info.UnitNamedStorageInfo.Life != life.Alive {
		return errors.Errorf(
			"unit %q is not alive",
			info.UnitNamedStorageInfo.Name,
		).Add(applicationerrors.UnitNotAlive)
	}

	// If the Storage Instance has a charm name set then it must match the
	// Unit's charm metadata name. Should these values not match then it
	// indicates that the Storage Instance was not supposed to be used with the
	// Unit's charm.
	if info.StorageInstanceInfo.CharmName != nil &&
		*info.StorageInstanceInfo.CharmName != info.UnitNamedStorageInfo.CharmMetadataName {
		return errors.Errorf(
			"storage instance %q charm name %q does not match unit charm %q",
			info.StorageInstanceInfo.UUID,
			*info.StorageInstanceInfo.CharmName,
			info.UnitNamedStorageInfo.CharmMetadataName,
		).Add(applicationerrors.StorageInstanceCharmNameMismatch)
	}

	charmStorageDef := info.UnitNamedStorageInfo.CharmStorageDefinitionForValidation
	expectedKind, err := storage.StorageKindFromCharmStorageType(charmStorageDef.Type)
	if err != nil {
		return errors.Errorf(
			"determining storage kind for charm storage definition %q: %w",
			charmStorageDef.Name, err,
		)
	}

	// The Storage Instance kind must be of the same type the Charm is
	// expecting. i.e we can not attach a block device to a filesystem.
	if info.StorageInstanceInfo.Kind != expectedKind {
		return errors.Errorf(
			"storage instance %q kind %q is not valid for charm storage definition %q of kind %q",
			info.StorageInstanceInfo.UUID,
			info.StorageInstanceInfo.Kind,
			charmStorageDef.Name,
			charmStorageDef.Type,
		).Add(applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition)
	}

	// Validate that the size of the storage instance doesn't exceed the minimum
	// supported by the charm.
	sizeMIB := storage.CalculateStorageInstanceSizeForAttachment(info.StorageInstanceInfo)
	if sizeMIB < charmStorageDef.MinimumSize {
		return errors.Errorf(
			"storage instance %q size %d MiB is below charm storage definition %q minimum size of %d MiB",
			info.StorageInstanceInfo.UUID,
			sizeMIB,
			charmStorageDef.Name,
			charmStorageDef.MinimumSize,
		).Add(applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition)
	}

	// Validating that attaching this storage instance to the unit doesn't
	// violate the max count of the charm's storage definition.
	if charmStorageDef.CountMax >= 0 {
		wantCount := int(info.UnitNamedStorageInfo.AlreadyAttachedCount) + 1
		if wantCount > charmStorageDef.CountMax {
			return applicationerrors.StorageCountLimitExceeded{
				Maximum:     &charmStorageDef.CountMax,
				Minimum:     charmStorageDef.CountMin,
				Requested:   wantCount,
				StorageName: charmStorageDef.Name,
			}
		}
	}

	// Validate that the storage instance is not already attached to the unit.
	// We do this after the checks above, by this stage we know that the storage
	// instance is valid for attachment.
	//
	// It is an explicit decision to return an error for this case as it should
	// be the callers discretion if this is a case they are concerned with.
	// Our job is to report that the operation as requested cannot be performed.
	for _, attachment := range info.StorageInstanceAttachments {
		if attachment.UnitUUID == info.UnitNamedStorageInfo.UUID {
			return errors.Errorf(
				"storage instance %q already attached to unit %q",
				info.StorageInstanceInfo.UUID,
				info.UnitNamedStorageInfo.Name,
			).Add(applicationerrors.StorageInstanceAlreadyAttachedToUnit)
		}
	}

	// Validate that if the storage instance already has existing attachments
	// that the charm storage definition supports shared storage.
	if !charmStorageDef.Shared && len(info.StorageInstanceAttachments) > 0 {
		return errors.Errorf(
			"storage instance %q has existing attachments but charm storage definition %q is not shared",
			info.StorageInstanceInfo.UUID,
			charmStorageDef.Name,
		).Add(applicationerrors.StorageInstanceAttachSharedAccessNotSupported)
	}

	// Validate that the storage instance owning machines if any are compatible
	// with the machine the unit is running on.
	err = validateStorageInstanceOwningMachine(info)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// validateStorageInstancesForUnitAttachment validates multiple storage
// instances, returning the first validation error encountered.
func validateStorageInstancesForUnitAttachment(
	ctx context.Context,
	infos []applicationinternal.StorageInstanceInfoForUnitAttach,
) error {
	for _, info := range infos {
		if err := validateStorageInstanceForUnitAttachment(ctx, info); err != nil {
			return err
		}
	}
	return nil
}

// validateStorageInstanceAttachmentForNewUnit validates that the specified
// storage instances can be attached to a new unit. It builds the unit storage
// definition context from the supplied storage directives.
//
// The following errors may be returned:
// - [applicationerrors.StorageNameNotSupported] when a storage name is not
// present in the storage directives.
// - [storageerrors.StorageInstanceNotAlive] when the storage instance is not alive.
// - [applicationerrors.UnitNotAlive] when the unit is not alive.
// - [applicationerrors.StorageInstanceCharmNameMismatch] when the storage
// instance charm name does not match the unit charm.
// - [applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition]
// when the storage instance kind does not match the charm storage definition.
// - [applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition]
// when the storage instance size is below the charm storage minimum.
// - [applicationerrors.StorageCountLimitExceeded] when attaching would exceed
// the charm storage maximum.
// - [applicationerrors.StorageInstanceAlreadyAttachedToUnit] when the storage
// instance is already attached to the unit.
// - [applicationerrors.StorageInstanceUnexpectedAttachments] when the charm
// storage definition is not shared and the storage instance has existing
// attachments.
// - [applicationerrors.StorageInstanceAttachMachineOwnerMismatch] when the
// storage instance owning machine does not match the unit's machine.
func validateStorageInstanceAttachmentForNewUnit(
	ctx context.Context,
	unitUUID coreunit.UUID,
	unitMachineUUID *coremachine.UUID,
	unitNetNodeUUID domainnetwork.NetNodeUUID,
	storageDirectives []applicationinternal.StorageDirective,
	attachInfos []applicationinternal.StorageInstanceInfoForAttach,
) error {
	if len(attachInfos) == 0 {
		return nil
	}

	storageDirectivesByName := make(
		map[domainstorage.Name]applicationinternal.StorageDirective,
		len(storageDirectives),
	)
	for _, directive := range storageDirectives {
		storageDirectivesByName[directive.Name] = directive
	}

	alreadyAttachedByStorageName := make(map[domainstorage.Name]uint32)
	validationInfos := make(
		[]applicationinternal.StorageInstanceInfoForUnitAttach,
		0, len(attachInfos),
	)
	for _, info := range attachInfos {
		storageName := domainstorage.Name(info.StorageName)
		directive, ok := storageDirectivesByName[storageName]
		if !ok {
			return errors.Errorf(
				"storage name %q is not supported", storageName,
			).Add(applicationerrors.StorageNameNotSupported)
		}

		alreadyAttached := alreadyAttachedByStorageName[storageName]
		validationInfos = append(validationInfos, applicationinternal.StorageInstanceInfoForUnitAttach{
			StorageInstanceInfo:        info.StorageInstanceInfo,
			StorageInstanceAttachments: info.StorageInstanceAttachments,
			UnitNamedStorageInfo: applicationinternal.UnitNamedStorageInfo{
				CharmStorageDefinitionForValidation: applicationinternal.CharmStorageDefinitionForValidation{
					Name:        string(directive.Name),
					Type:        directive.CharmStorageType,
					CountMin:    0,
					CountMax:    directive.MaxCount,
					MinimumSize: directive.Size,
					Shared:      false,
				},
				UUID:                 unitUUID,
				AlreadyAttachedCount: alreadyAttached,
				CharmMetadataName:    directive.CharmMetadataName,
				Life:                 life.Alive,
				MachineUUID:          unitMachineUUID,
				NetNodeUUID:          unitNetNodeUUID,
			},
		})
		// Increment already attached so it is calculated correctly for the next
		// instance.
		alreadyAttachedByStorageName[storageName] += 1
	}

	return validateStorageInstancesForUnitAttachment(ctx, validationInfos)
}

func storageInstanceCompositionsFromAttachInfos(
	attachInfos []applicationinternal.StorageInstanceInfoForAttach,
) []applicationinternal.StorageInstanceComposition {
	compositions := make([]applicationinternal.StorageInstanceComposition, 0, len(attachInfos))
	for _, info := range attachInfos {
		comp := applicationinternal.StorageInstanceComposition{
			StorageName: domainstorage.Name(info.StorageName),
			UUID:        info.UUID,
		}

		if info.Filesystem != nil {
			comp.Filesystem = &applicationinternal.StorageInstanceCompositionFilesystem{
				ProvisionScope: info.Filesystem.ProvisionScope,
				UUID:           info.Filesystem.UUID,
			}
		}

		if info.Volume != nil {
			comp.Volume = &applicationinternal.StorageInstanceCompositionVolume{
				ProvisionScope: info.Volume.ProvisionScope,
				UUID:           info.Volume.UUID,
			}
		}

		compositions = append(compositions, comp)
	}

	return compositions
}

// validateStorageInstanceOwningMachine validates that any owning machine
// associated with a storage instance matches the unit's machine.
//
// The filesystem or volume owning machine UUIDs are checked independently. If
// either is set and does not match the unit's machine UUID, or if the unit has
// no machine UUID while the storage instance does, an error is returned.
//
// The following errors may be returned:
// - [applicationerrors.StorageInstanceAttachMachineOwnerMismatch] when the
// owning machine UUID does not match the unit's machine UUID or the unit has
// no machine assigned but the storage instance does.
func validateStorageInstanceOwningMachine(
	info applicationinternal.StorageInstanceInfoForUnitAttach,
) error {
	unitMachineUUID := info.UnitNamedStorageInfo.MachineUUID
	unitMachineName := "unset"
	if unitMachineUUID != nil {
		unitMachineName = unitMachineUUID.String()
	}

	if info.StorageInstanceInfo.Filesystem != nil &&
		info.StorageInstanceInfo.Filesystem.OwningMachineUUID != nil {
		owningMachineUUID := info.StorageInstanceInfo.Filesystem.OwningMachineUUID
		if unitMachineUUID == nil || *unitMachineUUID != *owningMachineUUID {
			return errors.Errorf(
				"storage instance %q filesystem owning machine %q does not match unit machine %q",
				info.StorageInstanceInfo.UUID,
				owningMachineUUID.String(),
				unitMachineName,
			).Add(applicationerrors.StorageInstanceAttachMachineOwnerMismatch)
		}
	}

	if info.StorageInstanceInfo.Volume != nil &&
		info.StorageInstanceInfo.Volume.OwningMachineUUID != nil {
		owningMachineUUID := info.StorageInstanceInfo.Volume.OwningMachineUUID
		if unitMachineUUID == nil || *unitMachineUUID != *owningMachineUUID {
			return errors.Errorf(
				"storage instance %q volume owning machine %q does not match unit machine %q",
				info.StorageInstanceInfo.UUID,
				owningMachineUUID.String(),
				unitMachineName,
			).Add(applicationerrors.StorageInstanceAttachMachineOwnerMismatch)
		}
	}

	return nil
}

func (s *ProviderService) makeIAASUnitArgs(
	ctx context.Context,
	units []AddIAASUnitArg,
	storageDirectives []applicationinternal.StorageDirective,
	platform deployment.Platform,
	constraints constraints.Constraints,
) ([]application.AddIAASUnitArg, error) {
	args := make([]application.AddIAASUnitArg, len(units))
	for i, u := range units {
		placement, err := deployment.ParsePlacement(u.Placement)
		if err != nil {
			return nil, errors.Errorf("invalid placement: %w", err)
		}

		var (
			machineUUID        coremachine.UUID
			machineNetNodeUUID domainnetwork.NetNodeUUID
		)
		// If the placement of the unit is on to an already established machine
		// we need to resolve this to a machine uuid and netnode uuid.
		if placement.Type == deployment.PlacementTypeMachine {
			mUUID, mNNUUID, err := s.st.GetMachineUUIDAndNetNodeForName(
				ctx, placement.Directive,
			)
			if err != nil {
				return nil, errors.Errorf(
					"getting machine %q for unit placement directive: %w",
					placement.Directive, err,
				)
			}
			machineUUID = mUUID
			machineNetNodeUUID = mNNUUID
		} else {
			// If the placement is not on to an already established machine we need
			// to generate a new machine uuid and netnode uuid for the unit.
			var err error
			machineUUID, err = coremachine.NewUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new machine uuid for IAAS unit: %w", err,
				)
			}

			machineNetNodeUUID, err = domainnetwork.NewNetNodeUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new machine net node uuid for IAAS unit: %w", err,
				)
			}
		}

		unitUUID, err := coreunit.NewUUID()
		if err != nil {
			return nil, errors.Errorf(
				"generating new unit uuid for IAAS unit: %w", err,
			)
		}
		// We use the same netnode uuid as the machine for the unit.
		netNodeUUID := machineNetNodeUUID

		// Validate that any existing storage instance attachments requested can
		// be used for this unit.
		attachInfos, err := s.st.GetStorageAttachInfoForStorageInstances(
			ctx, u.StorageInstancesToAttach,
		)
		if err != nil {
			return nil, errors.Errorf(
				"getting storage instance attachment info: %w",
				err,
			)
		}

		err = validateStorageInstanceAttachmentForNewUnit(
			ctx,
			unitUUID,
			&machineUUID,
			netNodeUUID,
			storageDirectives,
			attachInfos,
		)
		if err != nil {
			return nil, errors.Errorf(
				"validating storage instance attachments for new unit: %w",
				err,
			)
		}

		existingStorage := storageInstanceCompositionsFromAttachInfos(attachInfos)

		// make unit storage args. IAAS units always have their storage
		// attached to the machine's net node.
		unitStorageArgs, err := s.storageService.MakeUnitStorageArgs(
			ctx,
			machineNetNodeUUID,
			storageDirectives,
			existingStorage,
			nil,
		)
		if err != nil {
			return nil, errors.Errorf(
				"making storage arguments for IAAS unit: %w", err,
			)
		}

		iassUnitStorageArgs, err := s.storageService.MakeIAASUnitStorageArgs(
			unitStorageArgs.StorageInstances)
		if err != nil {
			return nil, errors.Errorf(
				"making IAAS storage arguments for IAAS unit: %w", err,
			)
		}

		arg := application.AddIAASUnitArg{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: unitStorageArgs,
				Constraints:          constraints,
				Placement:            placement,
				NetNodeUUID:          netNodeUUID,
				UnitUUID:             unitUUID,
				UnitStatusArg:        s.makeIAASUnitStatusArgs(),
			},
			CreateIAASUnitStorageArg: iassUnitStorageArgs,
			Platform:                 platform,
			Nonce:                    u.Nonce,
			MachineNetNodeUUID:       machineNetNodeUUID,
			MachineUUID:              machineUUID,
		}
		args[i] = arg
	}

	return args, nil
}

func (s *ProviderService) makeCAASUnitArgs(
	ctx context.Context,
	units []AddUnitArg,
	storageDirectives []applicationinternal.StorageDirective,
	constraints constraints.Constraints,
) ([]application.AddCAASUnitArg, error) {
	args := make([]application.AddCAASUnitArg, len(units))
	for i, u := range units {
		placement, err := deployment.ParsePlacement(u.Placement)
		if err != nil {
			return nil, errors.Errorf("invalid placement: %w", err)
		}

		unitUUID, err := coreunit.NewUUID()
		if err != nil {
			return nil, errors.Errorf(
				"generating new unit uuid for caas unit: %w", err,
			)
		}

		netNodeUUID, err := domainnetwork.NewNetNodeUUID()
		if err != nil {
			return nil, errors.Errorf(
				"making new net node uuid for caas unit: %w", err,
			)
		}

		// Get existing storage instance information for attaching to this unit.
		attachInfos, err := s.st.GetStorageAttachInfoForStorageInstances(
			ctx, u.StorageInstancesToAttach,
		)
		if err != nil {
			return nil, errors.Errorf(
				"getting storage instance attachment info: %w",
				err,
			)
		}

		// Validate that any existing storage instance attachments requested can
		// be used for this unit.
		err = validateStorageInstanceAttachmentForNewUnit(
			ctx,
			unitUUID,
			nil,
			netNodeUUID,
			storageDirectives,
			attachInfos,
		)
		if err != nil {
			return nil, errors.Errorf(
				"validating storage instance attachments for new unit: %w",
				err,
			)
		}

		existingStorage := storageInstanceCompositionsFromAttachInfos(attachInfos)

		// make unit storage args. CAAS units always have their storage
		// attached to the unit's net node.
		unitStorageArgs, err := s.storageService.MakeUnitStorageArgs(
			ctx,
			netNodeUUID,
			storageDirectives,
			existingStorage,
			nil,
		)
		if err != nil {
			return nil, errors.Errorf("making storage for CAAS unit: %w", err)
		}

		arg := application.AddCAASUnitArg{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: unitStorageArgs,
				Constraints:          constraints,
				NetNodeUUID:          netNodeUUID,
				UnitUUID:             unitUUID,
				Placement:            placement,
				UnitStatusArg:        s.makeCAASUnitStatusArgs(),
			},
		}
		args[i] = arg
	}

	return args, nil
}

func (s *Service) makeIAASUnitStatusArgs() application.UnitStatusArg {
	return s.makeUnitStatusArgs(corestatus.MessageWaitForMachine)
}

func (s *Service) makeCAASUnitStatusArgs() application.UnitStatusArg {
	return s.makeUnitStatusArgs(corestatus.MessageInstallingAgent)
}

func (s *Service) makeUnitStatusArgs(workloadMessage string) application.UnitStatusArg {
	now := new(s.clock.Now().UTC())
	return application.UnitStatusArg{
		AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
			Status: status.UnitAgentStatusAllocating,
			Since:  now,
		},
		WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusWaiting,
			Message: workloadMessage,
			Since:   now,
		},
	}
}

// UpdateCAASUnit updates the specified CAAS unit, returning an error satisfying
// [applicationerrors.ApplicationIsDead] if the unit's application is dead.
func (s *Service) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params UpdateCAASUnitParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	appName := unitName.Application()
	_, appLife, err := s.st.GetApplicationLifeByName(ctx, appName)
	if err != nil {
		return errors.Errorf("getting application %q life: %w", appName, err)
	}
	if appLife == life.Dead {
		return errors.Errorf(
			"application %q is dead", appName,
		).Add(applicationerrors.ApplicationIsDead)
	}

	agentStatus, err := encodeUnitAgentStatus(params.AgentStatus)
	if err != nil {
		return errors.Errorf("encoding agent status: %w", err)
	}
	workloadStatus, err := encodeWorkloadStatus(params.WorkloadStatus)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}
	k8sPodStatus, err := encodeK8sPodStatus(params.CloudContainerStatus)
	if err != nil {
		return errors.Errorf("encoding k8s pod status: %w", err)
	}

	cassUnitUpdate := application.UpdateCAASUnitParams{
		ProviderID:     params.ProviderID,
		Address:        params.Address,
		Ports:          params.Ports,
		AgentStatus:    agentStatus,
		WorkloadStatus: workloadStatus,
		K8sPodStatus:   k8sPodStatus,
	}

	if err := s.st.UpdateCAASUnit(ctx, unitName, cassUnitUpdate); err != nil {
		return errors.Errorf("updating caas unit %q: %w", unitName, err)
	}
	return nil
}

// UpdateUnitCharm updates the currently running charm marker for the given
// unit.
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
// - [applicationerrors.UnitIsDead] if the unit is dead.
// - [applicationerrors.CharmNotFound] if the charm charm does not exist.
func (s *ProviderService) UpdateUnitCharm(ctx context.Context, unitName coreunit.Name, locator charm.CharmLocator) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := unitName.Validate()
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Errorf("getting uuid of unit %q: %w", unitName, err)
	}

	charmUUID, err := s.getCharmID(ctx, argsFromLocator(locator))
	if err != nil {
		return errors.Errorf("getting charm UUID: %w", err)
	}

	args, err := s.st.GetUnitStorageRefreshArgs(ctx, unitUUID, charmUUID)
	if err != nil {
		return errors.Errorf(
			"getting current and next unit %q storage directives", unitName,
		).Add(err)
	}

	if args.CurrentCharmUUID == args.RefreshCharmUUID {
		// The charm is already at the correct version. This happens when the
		// uniter starts.
		return nil
	}

	sic, sac, err := s.st.GetUnitOwnedStorageInstances(ctx, unitUUID)
	if err != nil {
		return errors.Errorf(
			"getting unit %q storage instances and attachments: %w",
			unitName, err,
		)
	}
	ownedStorageComposition := storageInstanceCompositionsFromAttachInfos(sic)

	unitStorageArgs, err := s.storageService.MakeUnitStorageArgs(
		ctx,
		args.NetNodeUUID,
		args.RefreshStorageDirectives,
		ownedStorageComposition,
		sac,
	)
	if err != nil {
		return errors.Errorf("making storage for unit %q: %w", unitName, err)
	}

	updateArgs := applicationinternal.UpdateUnitCharmArg{
		UUID:        unitUUID,
		CharmUUID:   charmUUID,
		UnitStorage: unitStorageArgs,
	}

	if args.MachineUUID != nil {
		iaasUnitStorageArgs, err := s.storageService.MakeIAASUnitStorageArgs(
			unitStorageArgs.StorageInstances)
		if err != nil {
			return errors.Errorf(
				"making IAAS storage arguments for IAAS unit: %w", err,
			)
		}
		updateArgs.MachineUUID = args.MachineUUID
		updateArgs.IAASUnitStorage = &iaasUnitStorageArgs
	}

	err = s.st.UpdateUnitCharm(ctx, updateArgs)
	if err != nil {
		return errors.Errorf(
			"updating unit %q charm to %q", unitName, charmUUID,
		).Add(err)
	}

	return nil
}

// GetUnitUUID returns the UUID for the named unit.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/unit.InvalidUnitName] if the unit name is invalid.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the unit doesn't exist.
func (s *Service) GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting UUID of unit %q: %w", unitName, err)
	}
	return unitUUID, nil
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
func (s *Service) GetUnitLife(ctx context.Context, unitName coreunit.Name) (corelife.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting life for %q: %w", unitName, err)
	}
	return unitLife.Value()
}

// GetUnitPrincipal gets the subordinates principal unit. If no principal unit
// is found, for example, when the unit is not a subordinate, then false is
// returned.
func (s *Service) GetUnitPrincipal(ctx context.Context, unitName coreunit.Name) (coreunit.Name, bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", false, errors.Capture(err)
	}

	return s.st.GetUnitPrincipal(ctx, unitName)
}

// GetUnitMachineName gets the name of the unit's machine.
//
// The following errors may be returned:
//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
//     machine assigned.
//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
//   - [applicationerrors.UnitIsDead] if the unit is dead.
func (s *Service) GetUnitMachineName(ctx context.Context, unitName coreunit.Name) (coremachine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return "", errors.Capture(err)
	}

	unitMachine, err := s.st.GetUnitMachineName(ctx, unitUUID.String())
	if err != nil {
		return "", errors.Capture(err)
	}

	return coremachine.Name(unitMachine), nil
}

// GetUnitMachineUUID gets the unit's machine UUID.
//
// The following errors may be returned:
//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
//     machine assigned.
//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
//   - [applicationerrors.UnitIsDead] if the unit is dead.
func (s *Service) GetUnitMachineUUID(ctx context.Context, unitName coreunit.Name) (coremachine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return "", errors.Capture(err)
	}

	unitMachine, err := s.st.GetUnitMachineUUID(ctx, unitUUID.String())
	if err != nil {
		return "", errors.Capture(err)
	}

	return coremachine.UUID(unitMachine), nil
}

// GetUnitMachineNameAndUUID gets the name and UUID of the unit's machine.
//
// The following errors may be returned:
//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
//     machine assigned.
//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
//   - [applicationerrors.UnitIsDead] if the unit is dead.
func (s *Service) GetUnitMachineNameAndUUID(ctx context.Context, unitUUID coreunit.UUID) (coremachine.Name, coremachine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitUUID.Validate(); err != nil {
		return "", "", errors.Capture(err)
	}

	unitMachineName, err := s.st.GetUnitMachineName(ctx, unitUUID.String())
	if err != nil {
		return "", "", errors.Capture(err)
	}

	unitMachineUUID, err := s.st.GetUnitMachineUUID(ctx, unitUUID.String())
	if err != nil {
		return "", "", errors.Capture(err)
	}

	return coremachine.Name(unitMachineName), coremachine.UUID(unitMachineUUID), nil
}

// GetUnitRefreshAttributes returns the unit refresh attributes for the
// specified unit. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned.
// This doesn't take into account life, so it can return the life of a unit
// even if it's dead.
func (s *Service) GetUnitRefreshAttributes(ctx context.Context, unitName coreunit.Name) (application.UnitAttributes, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return application.UnitAttributes{}, errors.Capture(err)
	}

	return s.st.GetUnitRefreshAttributes(ctx, unitName)
}

// GetAllUnitNames returns a slice of all unit names in the model.
func (s *Service) GetAllUnitNames(ctx context.Context) ([]coreunit.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	names, err := s.st.GetAllUnitNames(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return names, nil
}

// GetUnitNamesForApplication returns a slice of the unit names for the given application
// The following errors may be returned:
// - [applicationerrors.ApplicationIsDead] if the application is dead
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (s *Service) GetUnitNamesForApplication(ctx context.Context, appName string) ([]coreunit.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appUUID, err := s.st.GetApplicationUUIDByName(ctx, appName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	names, err := s.st.GetUnitNamesForApplication(ctx, appUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return names, nil
}

// GetUnitNamesOnMachine returns a slice of the unit names on the given machine.
// The following errors may be returned:
// - [applicationerrors.MachineNotFound] if the machine does not exist
func (s *Service) GetUnitNamesOnMachine(ctx context.Context, machineName coremachine.Name) ([]coreunit.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetMachineNetNodeUUIDFromName(ctx, machineName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	names, err := s.st.GetUnitNamesForNetNode(ctx, netNodeUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return names, nil
}

// SetUnitWorkloadVersion sets the workload version for the given unit.
func (s *Service) SetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name, version string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	return s.st.SetUnitWorkloadVersion(ctx, unitName, version)
}

// GetUnitWorkloadVersion returns the workload version for the given unit.
func (s *Service) GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	version, err := s.st.GetUnitWorkloadVersion(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting workload version for %q: %w", unitName, err)
	}
	return version, nil
}

// GetUnitK8sPodInfo returns information about the k8s pod for the given unit.
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [applicationerrors.UnitIsDead] if the unit is dead
func (s *Service) GetUnitK8sPodInfo(ctx context.Context, name coreunit.Name) (application.K8sPodInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := name.Validate(); err != nil {
		return application.K8sPodInfo{}, errors.Capture(err)
	}

	return s.st.GetUnitK8sPodInfo(ctx, name)
}

// GetUnitsK8sPodInfo returns information about the k8s pods for all alive units.
func (s *Service) GetUnitsK8sPodInfo(ctx context.Context) (map[coreunit.Name]application.K8sPodInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetUnitsK8sPodInfo(ctx)
}

// GetUnitSubordinates returns the names of all the subordinate units of the
// given principal unit.
//
// If the principal unit cannot be found, [applicationerrors.UnitNotFound] is
// returned.
func (s *Service) GetUnitSubordinates(ctx context.Context, unitName coreunit.Name) ([]coreunit.Name, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	return s.st.GetUnitSubordinates(ctx, unitName)
}

// GetAllUnitLifeForApplication returns a map of the unit names and their lives
// for the given application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (s *Service) GetAllUnitLifeForApplication(ctx context.Context, appID coreapplication.UUID) (map[coreunit.Name]corelife.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	namesAndLives, err := s.st.GetAllUnitLifeForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	namesAndCoreLives := map[coreunit.Name]corelife.Value{}
	for name, lifeID := range namesAndLives {
		unitName, err := coreunit.NewName(name)
		if err != nil {
			return nil, errors.Errorf("parsing unit name %q: %w", name, err)
		}
		namesAndCoreLives[unitName], err = life.Life(lifeID).Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
	}
	return namesAndCoreLives, nil
}

// GetAllUnitCloudContainerIDsForApplication returns a map of the unit names
// and their cloud container provider IDs for the given application.
//   - If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound]
//     is returned.
//   - If the application UUID is not valid, [coreerrors.NotValid] is returned.
func (s *Service) GetAllUnitCloudContainerIDsForApplication(ctx context.Context, appUUID coreapplication.UUID) (map[coreunit.Name]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	idMap, err := s.st.GetAllUnitCloudContainerIDsForApplication(ctx, appUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return idMap, nil
}

// IAASUnitContext describes the IAAS context information required for the
// construction of a context factory for a unit.
type IAASUnitContext struct {
	CloudAPIVersion                   string
	LegacyProxySettings               proxy.Settings
	JujuProxySettings                 proxy.Settings
	PrivateAddress                    *string
	OpenedMachinePortRangesByEndpoint map[coreunit.Name]network.GroupedPortRanges
}

// GetIAASUnitContext returns IAAS context information required for the
// construction of a context factory.
//
// This is a fat method that gathers disparate information about a unit, but is
// necessary to avoid multiple round trips to the database when constructing a
// context factory for a unit.
func (s *ProviderService) GetIAASUnitContext(ctx context.Context, unitName coreunit.Name) (IAASUnitContext, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return IAASUnitContext{}, errors.Capture(err)
	}

	result, err := s.st.GetIAASUnitContext(ctx, unitName.String())
	if err != nil {
		return IAASUnitContext{}, errors.Errorf("getting context for unit %q: %w", unitName, err)
	}

	cloudAPIVersion, err := s.getCloudAPIVersion(ctx)
	if err != nil {
		s.logger.Warningf(ctx, "getting cloud api version: %v", err)
	}

	privateAddress, err := getIPAddressFromIAASPrivateAddress(result.PrivateAddress)
	if err != nil {
		s.logger.Warningf(ctx, "getting private address for unit %q: %v", unitName, err)
	}

	return IAASUnitContext{
		CloudAPIVersion:                   cloudAPIVersion,
		LegacyProxySettings:               encodeProxySettings(result.LegacyProxySettings),
		JujuProxySettings:                 encodeProxySettings(result.JujuProxySettings),
		PrivateAddress:                    privateAddress,
		OpenedMachinePortRangesByEndpoint: result.OpenedMachinePortRangesByEndpoint,
	}, nil
}

// getIPAddressFromIAASPrivateAddress encodes the private address for an IAAS
// unit. If the address is nil an error is returned stating the fact. If the
// address is not a valid CIDR string, then it's an unexpected format.
//
// Note: IP addresses from kubernetes do not contain subnet mask suffixes yet,
// so don't use this method for encoding CAAS unit addresses.
func getIPAddressFromIAASPrivateAddress(addr *string) (*string, error) {
	if addr == nil {
		return nil, errors.Errorf("no private address")
	}

	ipAddr, _, err := net.ParseCIDR(*addr)
	if err != nil {
		return nil, errors.Errorf("parsing private address %q: %w", *addr, err)
	}

	return new(ipAddr.String()), nil
}

// CAASUnitContext describes the CAAS context information required for the
// construction of a context factory for a unit.
type CAASUnitContext struct {
	CloudAPIVersion            string
	LegacyProxySettings        proxy.Settings
	JujuProxySettings          proxy.Settings
	OpenedPortRangesByEndpoint map[coreunit.Name]network.GroupedPortRanges
}

// GetCAASUnitContext returns CAAS context information required for the
// construction of a context factory.
//
// This is a fat method that gathers disparate information about a unit, but is
// necessary to avoid multiple round trips to the database when constructing a
// context factory for a unit.
func (s *ProviderService) GetCAASUnitContext(ctx context.Context, unitName coreunit.Name) (CAASUnitContext, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return CAASUnitContext{}, errors.Capture(err)
	}

	result, err := s.st.GetCAASUnitContext(ctx, unitName.String())
	if err != nil {
		return CAASUnitContext{}, errors.Errorf("getting context for unit %q: %w", unitName, err)
	}

	cloudAPIVersion, err := s.getCloudAPIVersion(ctx)
	if err != nil {
		s.logger.Warningf(ctx, "getting cloud api version: %v", err)
	}

	return CAASUnitContext{
		CloudAPIVersion:            cloudAPIVersion,
		LegacyProxySettings:        encodeProxySettings(result.LegacyProxySettings),
		JujuProxySettings:          encodeProxySettings(result.JujuProxySettings),
		OpenedPortRangesByEndpoint: result.OpenedPortRangesByEndpoint,
	}, nil
}

func (s *ProviderService) getCloudAPIVersion(ctx context.Context) (string, error) {
	env, err := s.cloudInfoGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		// If the cloud doesn't support returning environment info, we can assume
		// that it's an older cloud and return an empty string for the API version.
		return "", nil
	} else if err != nil {
		return "", errors.Errorf("opening provider: %w", err)
	}

	return env.APIVersion()
}

func encodeProxySettings(settings applicationinternal.ProxySettings) proxy.Settings {
	return proxy.Settings{
		Http:    settings.HTTP,
		Https:   settings.HTTPS,
		NoProxy: settings.NoProxy,
		Ftp:     settings.FTP,
	}
}
