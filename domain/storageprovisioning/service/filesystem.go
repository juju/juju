// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"path"
	"strconv"

	coreapplication "github.com/juju/juju/core/application"
	corechangestream "github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/domain/storageprovisioning/internal"
	"github.com/juju/juju/internal/errors"
)

// FilesystemState defines the interface required for performing filesystem
// provisioning operations in the model.
type FilesystemState interface {
	// CheckFilesystemExists checks if a filesystem exists for the supplied
	// filesystem ID. True is returned when a filesystem exists for the supplied
	// id.
	CheckFilesystemForIDExists(context.Context, string) (bool, error)

	// GetFilesystem retrieves the [storageprovisioning.Filesystem] for the
	// supplied filesystem UUID.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
	// exists for the provided filesystem UUID.
	GetFilesystem(
		context.Context,
		storageprovisioning.FilesystemUUID,
	) (storageprovisioning.Filesystem, error)

	// GetFilesystemAttachment retrieves the
	// [storageprovisioning.FilesystemAttachment] for the supplied filesystem
	// attachment UUID.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
	// exists for the provided filesystem UUID.
	GetFilesystemAttachment(
		context.Context, storageprovisioning.FilesystemAttachmentUUID,
	) (storageprovisioning.FilesystemAttachment, error)

	// GetFilesystemAttachmentIDs returns the
	// [domainstorageprovisioning.FilesystemAttachmentID] information for each
	// filesystem attachment UUID supplied. If a UUID does not exist or isn't
	// attached to either a machine or a unit then it will not exist in the
	// result.
	//
	// It is not considered an error if a filesystem attachment UUID no longer
	// exists as it is expected the caller has already satisfied this
	// requirement themselves.
	//
	// This function exists to help keep supporting storage provisioning facades
	// that have a very week data model about what a filesystem attachment is
	// attached to.
	//
	// All returned values will have either the machine name or unit name value
	// filled out in the [domainstorageprovisioning.FilesystemAttachmentID] struct.
	GetFilesystemAttachmentIDs(ctx context.Context, uuids []string) (map[string]storageprovisioning.FilesystemAttachmentID, error)

	// GetFilesystemAttachmentLife returns the current life value for a
	// filesystem attachment UUID.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound]
	// when no filesystem attachment exists for the provided UUID.
	GetFilesystemAttachmentLife(
		context.Context,
		storageprovisioning.FilesystemAttachmentUUID,
	) (domainlife.Life, error)

	// GetFilesystemAttachmentLifeForNetNode returns a mapping of filesystem
	// attachment UUIDs to the current life value for each machine provisioned
	// filesystem attachment that is to be provisioned by the machine owning the
	// supplied net node.
	GetFilesystemAttachmentLifeForNetNode(ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID) (map[string]domainlife.Life, error)

	// GetFilesystemAttachmentParams retrieves the attachment params for the
	// given filesysatem attachment.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no
	// filesystem attachment exists for the supplied uuid.
	GetFilesystemAttachmentParams(
		context.Context, storageprovisioning.FilesystemAttachmentUUID,
	) (storageprovisioning.FilesystemAttachmentParams, error)

	// GetFilesystemAttachmentUUIDForFilesystemNetNode returns the filesystem
	// attachment UUID for the supplied filesystem UUID which is attached to the
	// given net node UUID.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the supplied values.
	// - [networkerrors.NetNodeNotFound] when no net node exists for the supplied
	// net node UUID.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound]
	// when no filesystem attachment exists for the supplied values.
	GetFilesystemAttachmentUUIDForFilesystemNetNode(
		context.Context,
		storageprovisioning.FilesystemUUID,
		domainnetwork.NetNodeUUID,
	) (storageprovisioning.FilesystemAttachmentUUID, error)

	// GetFilesystemLife returns the current life value for a filesystem UUID.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the provided filesystem UUID.
	GetFilesystemLife(
		context.Context, storageprovisioning.FilesystemUUID,
	) (domainlife.Life, error)

	// GetFilesystemLifeForNetNode returns a mapping of filesystem IDs to current
	// life value for each machine provisioned filesystem that is to be
	// provisioned by the machine owning the supplied net node.
	GetFilesystemLifeForNetNode(context.Context, domainnetwork.NetNodeUUID) (map[string]domainlife.Life, error)

	// GetFilesystemParams returns the filesystem params for the supplied uuid.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
	// exists for the uuid.
	GetFilesystemParams(
		context.Context, storageprovisioning.FilesystemUUID,
	) (storageprovisioning.FilesystemParams, error)

	// GetFilesystemRemovalParams returns the filesystem removal params for the
	// supplied uuid.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
	// exists for the uuid.
	GetFilesystemRemovalParams(
		context.Context, storageprovisioning.FilesystemUUID,
	) (storageprovisioning.FilesystemRemovalParams, error)

	// GetFilesystemUUIDForID returns the UUID for a filesystem with the
	// supplied id.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the provided filesystem UUID.
	GetFilesystemUUIDForID(
		context.Context, string,
	) (storageprovisioning.FilesystemUUID, error)

	// InitialWatchStatementMachineProvisionedFilesystems returns both the
	// namespace for watching filesystem life changes where the filesystem is
	// machine provisioned and the query for getting the current set of machine
	// provisioned filesystems.
	//
	// Only filesystems that can be provisioned by the machine connected to the
	// supplied net node will be emitted.
	InitialWatchStatementMachineProvisionedFilesystems(netNodeUUID domainnetwork.NetNodeUUID) (string, eventsource.Query[map[string]domainlife.Life])

	// InitialWatchStatementModelProvisionedFilesystems returns both the
	// namespace for watching filesystem life changes where the filesystem is
	// model provisioned and the initial query for getting the current set of
	// model provisioned filesystems in the model.
	InitialWatchStatementModelProvisionedFilesystems() (string, eventsource.NamespaceQuery)

	// InitialWatchStatementMachineProvisionedFilesystemAttachments returns
	// both the namespace for watching filesystem attachment life changes where
	// the filesystem attachment is machine provisioned and the initial query
	// for getting the current set of machine provisioned filesystem attachments.
	//
	// Only filesystem attachments that can be provisioned by the machine
	// connected to the supplied net node will be emitted.
	InitialWatchStatementMachineProvisionedFilesystemAttachments(netNodeUUID domainnetwork.NetNodeUUID) (string, eventsource.Query[map[string]domainlife.Life])

	// InitialWatchStatementModelProvisionedFilesystemAttachments returns both
	// the namespace for watching filesystem attachment life changes where the
	// filesystem attachment is model provisioned and the initial query for
	// getting the current set of model provisioned filesystem attachments.
	InitialWatchStatementModelProvisionedFilesystemAttachments() (string, eventsource.NamespaceQuery)

	// GetFilesystemTemplatesForApplication returns all the filesystem templates
	// for a given application.
	GetFilesystemTemplatesForApplication(
		context.Context,
		coreapplication.UUID,
	) ([]internal.FilesystemTemplate, error)

	// SetFilesystemProvisionedInfo sets on the provided filesystem the information
	// about the provisioned filesystem.
	SetFilesystemProvisionedInfo(ctx context.Context, filesystemUUID storageprovisioning.FilesystemUUID, info storageprovisioning.FilesystemProvisionedInfo) error

	// SetFilesystemAttachmentProvisionedInfo sets on the provided filesystem
	// attachment information about the provisoned filesystem attachment.
	SetFilesystemAttachmentProvisionedInfo(ctx context.Context, filesystemAttachmentUUID storageprovisioning.FilesystemAttachmentUUID, info storageprovisioning.FilesystemAttachmentProvisionedInfo) error
}

// CheckFilesystemForIDExists checks if a filesystem exists for the supplied
// filesystem ID. True is returned when a filesystem exists.
func (s *Service) CheckFilesystemForIDExists(
	ctx context.Context, id string,
) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.CheckFilesystemForIDExists(ctx, id)
}

// defaultFilesystemAttachmentDir returns the default directory where filesystem
// attachments will be mounted to.
func defaultFilesystemAttachmentDir() string {
	return "/var/lib/juju/storage"
}

// GetFilesystemAttachmentForMachine retrieves the [storageprovisioning.FilesystemAttachment]
// for the supplied net node uuid and filesystem id.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUID.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the provided values.
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the provided filesystem ID.
func (s *Service) GetFilesystemAttachmentForMachine(
	ctx context.Context,
	filesystemID string,
	machineUUID coremachine.UUID,
) (storageprovisioning.FilesystemAttachment, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		ctx, filesystemID, machineUUID,
	)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	return s.st.GetFilesystemAttachment(ctx, uuid)
}

// GetFilesystemAttachmentForUnit retrieves the
// [storageprovisioning.FilesystemAttachment]
// for the supplied filesystem ID that is attached to the supplied unit.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided unit UUID
// is not valid.
// - [applicationerrors.UnitNotFound] when no
// unit exists for the supplied unit UUID.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the supplied values.
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the provided filesystem ID.
func (s *Service) GetFilesystemAttachmentForUnit(
	ctx context.Context,
	filesystemID string,
	unitUUID coreunit.UUID,
) (storageprovisioning.FilesystemAttachment, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		ctx, filesystemID, unitUUID,
	)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	return s.st.GetFilesystemAttachment(ctx, uuid)
}

// GetFilesystemAttachmentIDs returns the
// [storageprovisioning.FilesystemAttachmentID] information for each of the
// supplied filesystem attachment UUIDs. If a filesystem attachment does exist
// for a supplied UUID or if a filesystem attachment is not attached to either a
// machine or unit then this UUID will be left out of the final result.
//
// It is not considered an error if a filesystem attachment UUID no longer
// exists as it is expected the caller has already satisfied this requirement
// themselves.
//
// This function exists to help keep supporting storage provisioning facades
// that have a very week data model about what a filesystem attachment is
// attached to.
//
// All returned values will have either the machine name or unit name value
// filled out in the [storageprovisioning.FilesystemAttachmentID] struct.
func (s *Service) GetFilesystemAttachmentIDs(
	ctx context.Context, uuids []string,
) (map[string]storageprovisioning.FilesystemAttachmentID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetFilesystemAttachmentIDs(ctx, uuids)
}

// GetFilesystemAttachmentLife returns the current life value for a filesystem
// attachment UUID.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the filesystem attachment UUID is not valid.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the provided UUID.
func (s *Service) GetFilesystemAttachmentLife(
	ctx context.Context,
	uuid storageprovisioning.FilesystemAttachmentUUID,
) (domainlife.Life, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return -1, errors.Errorf(
			"validating filesystem attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetFilesystemAttachmentLife(ctx, uuid)
	if err != nil {
		return -1, errors.Capture(err)
	}
	return life, nil
}

// GetFilesystemAttachmentParams retrieves the attachment parameters for a given
// filesystem attachment. This function guarantees to always return a mount
// point for the attachment.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied filesystem attachment UUID is not
// valid.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the supplied uuid.
func (s *Service) GetFilesystemAttachmentParams(
	ctx context.Context,
	uuid storageprovisioning.FilesystemAttachmentUUID,
) (storageprovisioning.FilesystemAttachmentParams, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return storageprovisioning.FilesystemAttachmentParams{}, errors.New(
			"filesystem attachment uuid is not valid",
		).Add(coreerrors.NotValid)
	}

	params, err := s.st.GetFilesystemAttachmentParams(ctx, uuid)
	if err != nil {
		return storageprovisioning.FilesystemAttachmentParams{}, errors.Capture(err)
	}

	if params.MountPoint != "" {
		// The mount point has already been set on the attachment. There is
		// nothing more to do.
		return params, nil
	}

	params.MountPoint = calculateFilesystemAttachmentMountPoint(
		params.CharmStorageLocation,
		uuid,
	)
	return params, nil
}

// calculateFilesystemAttachmentMountPoint calculates the mount point for a
// filesystem attachment. If the charmStorageLocation supplied is empty the the
// value from [defaultFilesystemAttachmentDir] will be used as the base of the
// mount point.
//
// This function guarantees to be idempotent given the same attachment and charm
// location.
func calculateFilesystemAttachmentMountPoint(
	charmStorageLocation string,
	uuid storageprovisioning.FilesystemAttachmentUUID,
) string {
	refLocation := charmStorageLocation
	if charmStorageLocation == "" {
		refLocation = defaultFilesystemAttachmentDir()
	}

	return path.Join(refLocation, uuid.String())
}

// calculateFilesystemTemplateAttachmentMountPoint calculates the mount point
// for a filesystem attachment on a Kubernetes pod. Because it is not know at
// the time of calculating this value the exact composition of the attachments
// we use the storage name and index to build uniqueness.
func calculateFilesystemTemplateAttachmentMountPoint(
	storageName,
	charmStorageLocation string,
	idx int,
) string {
	refLocation := charmStorageLocation
	if charmStorageLocation == "" {
		refLocation = defaultFilesystemAttachmentDir()
	}

	// The decision was to always append the storage name and index to the ref
	// location even when the storage is a singleton was done to avoid conflicts
	// with other charm storage. Because this calculation has no insight into
	// if the charm has made a mistake specified the same storage location for
	// multiple storage names. Doing it this way makes this calculation always
	// safe.
	return path.Join(refLocation, storageName, strconv.Itoa(idx))
}

// calculateFilesystemTemplateAttachmentMountPoints calculates all of the mount
// points for a filesystem template based on the desired count. The result is
// a slice equaling count in length with a unique mount point.
func calculateFilesystemTemplateAttachmentMountPoints(
	storageName,
	charmStorageLocation string,
	count int,
) []string {
	retVal := make([]string, 0, count)
	for range count {
		retVal = append(
			retVal,
			calculateFilesystemTemplateAttachmentMountPoint(
				storageName, charmStorageLocation, len(retVal),
			),
		)
	}
	return retVal
}

// GetFilesystemAttachmentUUIDForFilesystemIDMachine returns the filesystem attachment
// UUID for the supplied filesystem ID which is attached to the machine.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided unit UUID is not valid.
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the supplied id.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the supplied values.
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// machine UUID.
func (s *Service) GetFilesystemAttachmentUUIDForFilesystemIDMachine(
	ctx context.Context,
	filesystemID string,
	machineUUID coremachine.UUID,
) (storageprovisioning.FilesystemAttachmentUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	fsUUID, err := s.st.GetFilesystemUUIDForID(ctx, filesystemID)
	if err != nil {
		return "", errors.Errorf(
			"getting filesystem uuid for id %q: %w", filesystemID, err,
		)
	}

	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	uuid, err := s.st.GetFilesystemAttachmentUUIDForFilesystemNetNode(
		ctx, fsUUID, netNodeUUID,
	)
	if errors.Is(err, networkerrors.NetNodeNotFound) {
		return "", errors.Errorf(
			"machine %q does not exist", machineUUID.String(),
		).Add(machineerrors.MachineNotFound)
	} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		return "", errors.Errorf(
			"filesystem %q does not exist", filesystemID,
		).Add(storageprovisioningerrors.FilesystemNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetFilesystemAttachmentUUIDForFilesystemIDUnit returns the filesystem attachment UUID
// for the supplied filesystem ID which is attached to the unit.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided unit UUID is not valid.
// - [storageprovisioningerrors.FilesystemNotFound] when no fileystem exists
// for the supplied id.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the supplied values.
// - [applicationerrors.UnitNotFound] when no unit exists for the provided unit
// UUID.
func (s *Service) GetFilesystemAttachmentUUIDForFilesystemIDUnit(
	ctx context.Context,
	filesystemID string,
	unitUUID coreunit.UUID,
) (storageprovisioning.FilesystemAttachmentUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitUUID.Validate(); err != nil {
		return "", errors.Errorf("validating unit uuid: %w", err)
	}

	netNodeUUID, err := s.st.GetUnitNetNodeUUID(ctx, unitUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	fsUUID, err := s.st.GetFilesystemUUIDForID(ctx, filesystemID)
	if err != nil {
		return "", errors.Capture(err)
	}

	uuid, err := s.st.GetFilesystemAttachmentUUIDForFilesystemNetNode(
		ctx, fsUUID, netNodeUUID,
	)
	if errors.Is(err, networkerrors.NetNodeNotFound) {
		return "", errors.Errorf(
			"unit %q does not exist", unitUUID.String(),
		).Add(applicationerrors.UnitNotFound)
	} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		return "", errors.Errorf(
			"filesystem %q does not exist", filesystemID,
		).Add(storageprovisioningerrors.FilesystemNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetFilesystemForID retrieves the [storageprovisioning.Filesystem] for the
// supplied filesystem ID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
// exists for the provided filesystem ID.
func (s *Service) GetFilesystemForID(
	ctx context.Context,
	filesystemID string,
) (storageprovisioning.Filesystem, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.GetFilesystemUUIDForID(ctx, filesystemID)
	if err != nil {
		return storageprovisioning.Filesystem{}, errors.Capture(err)
	}

	fs, err := s.st.GetFilesystem(ctx, uuid)
	if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		return storageprovisioning.Filesystem{}, errors.Errorf(
			"filesystem with id %q does not exist", filesystemID,
		).Add(storageprovisioningerrors.FilesystemNotFound)
	}

	return fs, errors.Capture(err)
}

// GetFilesystemLife returns the current life value for a filesystem UUID.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the filesystem UUID is not valid.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
// when no filesystem exists for the provided filesystem UUID.
func (s *Service) GetFilesystemLife(
	ctx context.Context,
	uuid storageprovisioning.FilesystemUUID,
) (domainlife.Life, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return -1, errors.Errorf(
			"validating filesystem uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetFilesystemLife(ctx, uuid)
	if err != nil {
		return -1, errors.Capture(err)
	}
	return life, nil
}

// GetFilesystemParams returns the filesystem params for the supplied uuid.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied filesystem UUID is not valid.
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the uuid.
func (s *Service) GetFilesystemParams(
	ctx context.Context,
	uuid storageprovisioning.FilesystemUUID,
) (storageprovisioning.FilesystemParams, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return storageprovisioning.FilesystemParams{}, errors.New(
			"filesystem uuid is not valid",
		).Add(coreerrors.NotValid)
	}

	return s.st.GetFilesystemParams(ctx, uuid)
}

// GetFilesystemRemovalParams returns the filesystem removal params for the
// supplied uuid.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied filesystem UUID is not valid.
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the uuid.
// - [storageprovisioningerrors.FilesystemNotDead] when the filesystem was found
// but is either alive or dying, when it is expected to be dead.
func (s *Service) GetFilesystemRemovalParams(
	ctx context.Context, uuid storageprovisioning.FilesystemUUID,
) (storageprovisioning.FilesystemRemovalParams, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return storageprovisioning.FilesystemRemovalParams{}, errors.New(
			"filesystem uuid is not valid",
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetFilesystemLife(ctx, uuid)
	if err != nil {
		return storageprovisioning.FilesystemRemovalParams{}, errors.Errorf(
			"getting filesystem life for %q: %w", uuid, err,
		)
	}
	if life != domainlife.Dead {
		return storageprovisioning.FilesystemRemovalParams{}, errors.Errorf(
			"filesystem %q is not dead", uuid.String(),
		).Add(storageprovisioningerrors.FilesystemNotDead)
	}

	return s.st.GetFilesystemRemovalParams(ctx, uuid)
}

// GetFilesystemUUIDForID returns the UUID for a filesystem with the supplied
// id.
//
// The following errors may be returned:
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
// when no filesystem exists for the provided filesystem UUID.
func (s *Service) GetFilesystemUUIDForID(
	ctx context.Context, filesystemID string,
) (storageprovisioning.FilesystemUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.st.GetFilesystemUUIDForID(ctx, filesystemID)
	if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// WatchModelProvisionedFilesystems returns a watcher that emits filesystem IDs,
// whenever a model provisioned filsystem's life changes.
func (s *Service) WatchModelProvisionedFilesystems(
	ctx context.Context,
) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedFilesystems()
	return s.watcherFactory.NewNamespaceWatcher(
		ctx,
		initialQuery,
		"model provisioned filesystem watcher",
		eventsource.NamespaceFilter(ns, corechangestream.All))
}

// WatchMachineProvisionedFilesystems returns a watcher that emits filesystem IDs,
// whenever the given machine's provisioned filsystem's life changes.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the supplied machine UUID
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUID.
func (s *Service) WatchMachineProvisionedFilesystems(
	ctx context.Context, machineUUID coremachine.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeGetter := func(ctx context.Context) (map[string]domainlife.Life, error) {
		return s.st.GetFilesystemLifeForNetNode(ctx, netNodeUUID)
	}

	ns, initialLifeQuery := s.st.InitialWatchStatementMachineProvisionedFilesystems(netNodeUUID)
	initialQuery, mapper := makeEntityLifePrerequisites(initialLifeQuery, lifeGetter)
	filter := eventsource.PredicateFilter(
		ns, corechangestream.All, eventsource.EqualsPredicate(netNodeUUID.String()),
	)

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		fmt.Sprintf("machine provisioned filesystem watcher for %q", machineUUID),
		mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// WatchModelProvisionedFilesystemAttachments returns a watcher that emits
// filesystem attachment UUIDs, whenever a model provisioned filsystem
// attachment's life changes.
func (s *Service) WatchModelProvisionedFilesystemAttachments(
	ctx context.Context,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedFilesystemAttachments()
	return s.watcherFactory.NewNamespaceWatcher(
		ctx,
		initialQuery,
		"model provisioned filesystem attachment watcher",
		eventsource.NamespaceFilter(ns, corechangestream.All),
	)
}

// WatchMachineProvisionedFilesystemAttachments returns a watcher that emits
// filesystem attachment UUIDs, whenever the given machine's provisioned
// filsystem attachment's life changes.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine UUID
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUID.
func (s *Service) WatchMachineProvisionedFilesystemAttachments(
	ctx context.Context, machineUUID coremachine.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeGetter := func(ctx context.Context) (map[string]domainlife.Life, error) {
		return s.st.GetFilesystemAttachmentLifeForNetNode(ctx, netNodeUUID)
	}

	ns, initialLifeQuery := s.st.InitialWatchStatementMachineProvisionedFilesystemAttachments(netNodeUUID)
	initialQuery, mapper := makeEntityLifePrerequisites(initialLifeQuery, lifeGetter)
	filter := eventsource.PredicateFilter(
		ns, corechangestream.All, eventsource.EqualsPredicate(netNodeUUID.String()),
	)

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		fmt.Sprintf("machine provisioned filesystem attachment watcher for %q", machineUUID),
		mapper,
		filter,
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// GetFilesystemTemplatesForApplication returns all the filesystem templates for
// a given application.
func (s *Service) GetFilesystemTemplatesForApplication(
	ctx context.Context, appUUID coreapplication.UUID,
) ([]storageprovisioning.FilesystemTemplate, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	fsTemplates, err := s.st.GetFilesystemTemplatesForApplication(ctx, appUUID)
	if err != nil {
		return nil, errors.Errorf(
			"getting filesystem templates for app %q: %w", appUUID, err,
		)
	}

	retVal := make([]storageprovisioning.FilesystemTemplate, 0, len(fsTemplates))
	for _, fsTemplate := range fsTemplates {
		mountPoints := calculateFilesystemTemplateAttachmentMountPoints(
			fsTemplate.StorageName,
			fsTemplate.CharmLocationHint,
			fsTemplate.Count,
		)

		mountTemplate := storageprovisioning.FilesystemTemplate{
			Attributes:   fsTemplate.Attributes,
			Count:        fsTemplate.Count,
			Location:     fsTemplate.CharmLocationHint,
			MaxCount:     fsTemplate.MaxCount,
			MountPoints:  mountPoints,
			ProviderType: fsTemplate.ProviderType,
			ReadOnly:     fsTemplate.ReadOnly,
			SizeMiB:      fsTemplate.SizeMiB,
			StorageName:  fsTemplate.StorageName,
		}
		retVal = append(retVal, mountTemplate)
	}

	return retVal, nil
}

// SetFilesystemProvisionedInfo sets on the provided filesystem the information
// about the provisioned filesystem.
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the provided filesystem id.
func (s *Service) SetFilesystemProvisionedInfo(
	ctx context.Context,
	filesystemID string,
	info storageprovisioning.FilesystemProvisionedInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	filesystemUUID, err := s.st.GetFilesystemUUIDForID(ctx, filesystemID)
	if err != nil {
		return errors.Capture(err)
	}

	err = s.st.SetFilesystemProvisionedInfo(ctx, filesystemUUID, info)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetFilesystemAttachmentProvisionedInfoForMachine sets on the provided
// filesystem the information about the provisioned filesystem attachment.
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the provided filesystem id.
func (s *Service) SetFilesystemAttachmentProvisionedInfoForMachine(
	ctx context.Context,
	filesystemID string, machineUUID coremachine.UUID,
	info storageprovisioning.FilesystemAttachmentProvisionedInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return errors.Capture(err)
	}

	filesystemUUID, err := s.st.GetFilesystemUUIDForID(ctx, filesystemID)
	if err != nil {
		return errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return errors.Capture(err)
	}
	fsAttachmentUUID, err := s.st.GetFilesystemAttachmentUUIDForFilesystemNetNode(
		ctx, filesystemUUID, netNodeUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = s.st.SetFilesystemAttachmentProvisionedInfo(ctx, fsAttachmentUUID,
		info)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetFilesystemAttachmentProvisionedInfoForUnit sets on the provided filesystem
// the information about the provisioned filesystem attachment.
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the provided filesystem id.
func (s *Service) SetFilesystemAttachmentProvisionedInfoForUnit(
	ctx context.Context,
	filesystemID string, unitUUID coreunit.UUID,
	info storageprovisioning.FilesystemAttachmentProvisionedInfo,
) error {
	if err := unitUUID.Validate(); err != nil {
		return errors.Capture(err)
	}

	filesystemUUID, err := s.st.GetFilesystemUUIDForID(ctx, filesystemID)
	if err != nil {
		return errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetUnitNetNodeUUID(ctx, unitUUID)
	if err != nil {
		return errors.Capture(err)
	}
	fsAttachmentUUID, err := s.st.GetFilesystemAttachmentUUIDForFilesystemNetNode(
		ctx, filesystemUUID, netNodeUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = s.st.SetFilesystemAttachmentProvisionedInfo(ctx, fsAttachmentUUID,
		info)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}
