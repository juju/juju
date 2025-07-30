// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

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
	GetFilesystemLifeForNetNode(ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID) (map[string]domainlife.Life, error)

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

	// GetFilesystemTemplatesForApplication returns all the filesystem templates for
	// a given application.
	GetFilesystemTemplatesForApplication(context.Context, coreapplication.ID) ([]storageprovisioning.FilesystemTemplate, error)
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

// GetFilesystemAttachmentForMachine retrieves the
// [storageprovisioning.FilesystemAttachment] for the supplied filesystem ID
// and machine UUID.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine UUID
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
		return 0, errors.Errorf(
			"validating filesystem attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetFilesystemAttachmentLife(ctx, uuid)
	if err != nil {
		return 0, errors.Capture(err)
	}
	return life, nil
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
		return 0, errors.Errorf(
			"validating filesystem uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetFilesystemLife(ctx, uuid)
	if err != nil {
		return 0, errors.Capture(err)
	}
	return life, nil
}

// GetFilesystemUUIDForID returns the UUID for a filesystem with the supplied
// id.
//
// The following errors may be returned:
// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
// when no filesystem exists for the provided filesystem UUID.
func (s *Service) GetFilesystemUUIDForID(
	ctx context.Context, id string,
) (storageprovisioning.FilesystemUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.st.GetFilesystemUUIDForID(ctx, id)
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
		initialQuery,
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
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedFilesystemAttachments()
	return s.watcherFactory.NewNamespaceWatcher(initialQuery,
		eventsource.NamespaceFilter(ns, corechangestream.All))
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
