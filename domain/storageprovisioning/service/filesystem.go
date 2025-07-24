// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corechangestream "github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
)

// FilesystemState defines the interface required for performing filesystem
// provisioning operations in the model.
type FilesystemState interface {
	// GetFilesystem retrieves the [storageprovisioning.Filesystem] for the
	// supplied filesystem uuid.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
	// exists for the provided filesystem uuid.
	GetFilesystem(
		context.Context,
		domainstorageprovisioning.FilesystemUUID,
	) (storageprovisioning.Filesystem, error)

	// GetFilesystemAttachment retrieves the
	// [storageprovisioning.FilesystemAttachment] for the supplied filesystem
	// attachment uuid.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no
	// filesystem attachment does not exist for the provided filesystem uuid.
	// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
	// exists for the provided filesystem uuid.
	GetFilesystemAttachment(
		context.Context, domainstorageprovisioning.FilesystemAttachmentUUID,
	) (storageprovisioning.FilesystemAttachment, error)

	// GetFilesystemAttachmentIDs returns the
	// [domainstorageprovisioning.FilesystemAttachmentID] information for each
	// filesystem attachment uuid supplied. If a uuid does not exist or isn't
	// attached to either a machine or a unit then it will not exist in the
	// result.
	//
	// It is not considered an error if a filesystem attachment uuid no longer
	// exists as it is expected the caller has already satisfied this
	// requirement themselves.
	//
	// This function exists to help keep supporting storage provisioning facades
	// that have a very week data model about what a filesystem attachment is
	// attached to.
	//
	// All returned values will have either the machine name or unit name value
	// filled out in the [domainstorageprovisioning.FilesystemAttachmentID] struct.
	GetFilesystemAttachmentIDs(ctx context.Context, uuids []string) (map[string]domainstorageprovisioning.FilesystemAttachmentID, error)

	// GetFilesystemAttachmentLife returns the current life value for a
	// filesystem attachment uuid.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound]
	// when no filesystem attachment exists for the provided uuid.
	GetFilesystemAttachmentLife(
		context.Context,
		domainstorageprovisioning.FilesystemAttachmentUUID,
	) (domainlife.Life, error)

	// GetFilesystemAttachmentLifeForNetNode returns a mapping of filesystem
	// attachment uuids to the current life value for each machine provisioned
	// filesystem attachment that is to be provisioned by the machine owning the
	// supplied net node.
	GetFilesystemAttachmentLifeForNetNode(ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID) (map[string]domainlife.Life, error)

	// GetFilesystemAttachmentUUIDForIDNetNode returns the filesystem attachment uuid
	// for the supplied filesystem id which is attached to the given net node
	// uuid.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the supplied values.
	// - [networkerrors.NetNodeNotFound] when no net node exists for the supplied
	// net node uuid.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound]
	// when no filesystem attachment exists for the supplied values.
	GetFilesystemAttachmentUUIDForIDNetNode(
		context.Context,
		domainstorageprovisioning.FilesystemUUID,
		domainnetwork.NetNodeUUID,
	) (domainstorageprovisioning.FilesystemAttachmentUUID, error)

	// GetFilesystemLife returns the current life value for a filesystem uuid.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the provided filesystem uuid.
	GetFilesystemLife(
		context.Context, domainstorageprovisioning.FilesystemUUID,
	) (domainlife.Life, error)

	// GetFilesystemLifeForNetNode returns a mapping of filesystem ids to current
	// life value for each machine provisioned filesystem that is to be
	// provisioned by the machine owning the supplied net node.
	GetFilesystemLifeForNetNode(ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID) (map[string]domainlife.Life, error)

	// GetFilesystemUUIDForID returns the uuid for a filesystem with the supplied
	// id.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the provided filesystem uuid.
	GetFilesystemUUIDForID(context.Context, string) (domainstorageprovisioning.FilesystemUUID, error)

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

	// GetFilesystemTemplatesForApplication returns all the filesystem templates for
	// a given application.
	GetFilesystemTemplatesForApplication(context.Context, coreapplication.ID) ([]storageprovisioning.FilesystemTemplate, error)
}

// GetFilesystem retrieves the [storageprovisioning.Filesystem] for the
// supplied filesystem id.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
// exists for the provided filesystem id.
func (s *Service) GetFilesystem(
	ctx context.Context,
	filesystemID string,
) (storageprovisioning.Filesystem, error) {
	uuid, err := s.GetFilesystemUUIDForID(ctx, corestorage.ID(filesystemID))
	if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		return storageprovisioning.Filesystem{}, errors.Errorf(
			"filesystem with id %q does not exist", filesystemID,
		).Add(storageprovisioningerrors.FilesystemNotFound)
	} else if err != nil {
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

// GetFilesystemAttachmentForUnit retrieves the
// [storageprovisioning.FilesystemAttachment]
// for the supplied unit uuid and filesystem id.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided unit uuid
// is not valid.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when no
// unit exists for the supplied unit uuid.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound] when no filesystem attachment
// exists for the provided filesystem id.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound] when no filesystem exists for
// the provided filesystem id.
func (s *Service) GetFilesystemAttachmentForUnit(
	ctx context.Context,
	unitUUID coreunit.UUID,
	filesystemID string,
) (storageprovisioning.FilesystemAttachment, error) {
	if err := unitUUID.Validate(); err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetUnitNetNodeUUID(ctx, unitUUID)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	uuid, err := s.st.GetFilesystemAttachmentUUIDForIDNetNode(ctx, filesystemID, netNodeUUID)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	return s.st.GetFilesystemAttachment(ctx, uuid)
}

// GetFilesystemAttachmentForMachine retrieves the [storageprovisioning.FilesystemAttachment]
// for the supplied net node uuid and filesystem id.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUUID.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound] when no filesystem attachment
// exists for the provided filesystem id.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound] when no filesystem exists for
// the provided filesystem id.
func (s *Service) GetFilesystemAttachmentForMachine(
	ctx context.Context,
	machineUUID coremachine.UUID,
	filesystemID string,
) (storageprovisioning.FilesystemAttachment, error) {
	if err := machineUUID.Validate(); err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	uuid, err := s.st.GetFilesystemAttachmentUUIDForIDNetNode(ctx, filesystemID, netNodeUUID)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	return s.st.GetFilesystemAttachment(ctx, uuid)
}

// GetFilesystemAttachmentIDs returns the
// [domainstorageprovisioning.FilesystemAttachmentID] information for each of the
// supplied filesystem attachment uuids. If a filesystem attachment does exist
// for a supplied uuid or if a filesystem attachment is not attached to either a
// machine or unit then this uuid will be left out of the final result.
//
// It is not considered an error if a filesystem attachment uuid no longer
// exists as it is expected the caller has already satisfied this requirement
// themselves.
//
// This function exists to help keep supporting storage provisioning facades
// that have a very week data model about what a filesystem attachment is
// attached to.
//
// All returned values will have either the machine name or unit name value
// filled out in the [domainstorageprovisioning.FilesystemAttachmentID] struct.
func (s *Service) GetFilesystemAttachmentIDs(
	ctx context.Context, uuids []string,
) (map[string]domainstorageprovisioning.FilesystemAttachmentID, error) {
	return s.st.GetFilesystemAttachmentIDs(ctx, uuids)
}

// GetFilesystemAttachmentLife returns the current life value for a filesystem
// attachment uuid.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the filesystem attachment uuid is not valid.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound]
// when no filesystem attachment exists for the provided uuid.
func (s *Service) GetFilesystemAttachmentLife(
	ctx context.Context,
	uuid domainstorageprovisioning.FilesystemAttachmentUUID,
) (corelife.Value, error) {
	if err := uuid.Validate(); err != nil {
		return "", errors.Errorf(
			"validating filesystem attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetFilesystemAttachmentLife(ctx, uuid)
	if err != nil {
		return "", errors.Capture(err)
	}
	return life.Value()
}

// GetFilesystemAttachmentUUIDForIDMachine returns the filesystem attachment
// uuid for the supplied filesystem id which is attached to the machine.
//
// The following errors may be returned:
// - [corestorage.InvalidStorageID] when the provided id is not valid.
// - [coreerrors.NotValid] when the provided unit uuid is not valid.
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the supplied id.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the supplied values.
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// machine uuid.
func (s *Service) GetFilesystemAttachmentUUIDForIDMachine(
	ctx context.Context,
	id string,
	machineUUID coremachine.UUID,
) (domainstorageprovisioning.FilesystemAttachmentUUID, error) {
	if err := machineUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	fsUUID, err := s.st.GetFilesystemUUIDForID(ctx, id)
	if err != nil {
		return "", errors.Errorf(
			"getting filesystem uuid for id %q: %w", id, err,
		)
	}

	uuid, err := s.st.GetFilesystemAttachmentUUIDForIDNetNode(
		ctx, fsUUID, netNodeUUID,
	)
	if errors.Is(err, networkerrors.NetNodeNotFound) {
		return "", errors.Errorf(
			"machine %q does not exist", machineUUID.String(),
		).Add(machineerrors.MachineNotFound)
	} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		return "", errors.Errorf(
			"filesystem %q does not exist", id,
		).Add(storageprovisioningerrors.FilesystemNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetFilesystemAttachmentUUIDForIDUnit returns the filesystem attachment uuid
// for the supplied filesystem id which is attached to the unit.
//
// The following errors may be returned:
// - [corestorage.InvalidStorageID] when the provided id is not valid.
// - [coreerrors.NotValid] when the provided unit uuid is not valid.
// - [storageprovisioningerrors.FilesystemNotFound] when no fileystem exists
// for the supplied id.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound]
// when no filesystem attachment exists for the supplied values.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when no unit
// exists for the provided unit uuid.
func (s *Service) GetFilesystemAttachmentUUIDForIDUnit(
	ctx context.Context,
	id corestorage.ID,
	unitUUID coreunit.UUID,
) (domainstorageprovisioning.FilesystemAttachmentUUID, error) {
	if err := id.Validate(); err != nil {
		return "", errors.Capture(err)
	}
	if err := unitUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetUnitNetNodeUUID(ctx, unitUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	fsUUID, err := s.st.GetFilesystemUUIDForID(ctx, id.String())
	if err != nil {
		return "", errors.Errorf(
			"getting filesystem uuid for id %q: %w", id.String(), err,
		)
	}

	uuid, err := s.st.GetFilesystemAttachmentUUIDForIDNetNode(
		ctx, fsUUID, netNodeUUID,
	)
	if errors.Is(err, networkerrors.NetNodeNotFound) {
		return "", errors.Errorf(
			"unit %q does not exist", unitUUID.String(),
		).Add(applicationerrors.UnitNotFound)
	} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		return "", errors.Errorf(
			"filesystem %q does not exist", id.String(),
		).Add(storageprovisioningerrors.FilesystemNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}
	if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetFilesystemLife returns the current life value for a filesystem uuid.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the filesystem uuid is not valid.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
// when no filesystem exists for the provided filesystem uuid.
func (s *Service) GetFilesystemLife(
	ctx context.Context,
	uuid domainstorageprovisioning.FilesystemUUID,
) (corelife.Value, error) {
	if err := uuid.Validate(); err != nil {
		return "", errors.Errorf(
			"validating filesystem uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetFilesystemLife(ctx, uuid)
	if err != nil {
		return "", errors.Capture(err)
	}
	return life.Value()
}

// GetFilesystemUUIDForID returns the uuid for a filesystem with the supplied
// id.
//
// The following errors may be returned:
// - [corestorage.InvalidStorageID] when the provided id is not valid.
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
// when no filesystem exists for the provided filesystem uuid.
func (s *Service) GetFilesystemUUIDForID(
	ctx context.Context, id corestorage.ID,
) (domainstorageprovisioning.FilesystemUUID, error) {
	if err := id.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	uuid, err := s.st.GetFilesystemUUIDForID(ctx, id.String())
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
	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedFilesystems()
	return s.watcherFactory.NewNamespaceWatcher(
		initialQuery,
		eventsource.NamespaceFilter(ns, corechangestream.All))
}

// WatchMachineProvisionedFilesystems returns a watcher that emits filesystem IDs,
// whenever the given machine's provisioned filsystem's life changes.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the supplied machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine uuid.
func (s *Service) WatchMachineProvisionedFilesystems(
	ctx context.Context, machineUUID coremachine.UUID,
) (watcher.StringsWatcher, error) {
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
		initialQuery, mapper, filter)
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
	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedFilesystemAttachments()
	return s.watcherFactory.NewNamespaceWatcher(initialQuery,
		eventsource.NamespaceFilter(ns, corechangestream.All))
}

// WatchMachineProvisionedFilesystemAttachments returns a watcher that emits
// filesystem attachment UUIDs, whenever the given machine's provisioned
// filsystem attachment's life changes.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUUID.
func (s *Service) WatchMachineProvisionedFilesystemAttachments(
	ctx context.Context, machineUUID coremachine.UUID,
) (watcher.StringsWatcher, error) {
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
		initialQuery, mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// GetFilesystemTemplatesForApplication returns all the filesystem templates for
// a given application.
func (s *Service) GetFilesystemTemplatesForApplication(
	ctx context.Context, appUUID coreapplication.ID,
) ([]storageprovisioning.FilesystemTemplate, error) {
	if err := appUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	fsTemplates, err := s.st.GetFilesystemTemplatesForApplication(ctx, appUUID)
	if err != nil {
		return nil, errors.Errorf(
			"getting filesystem templates for app %q: %w", appUUID, err,
		)
	}
	return fsTemplates, nil
}
