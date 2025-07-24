// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
)

// GetFilesystem retrieves the [storageprovisioning.Filesystem] for the
// supplied filesystem id.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
// exists for the provided filesystem id.
func (st *State) GetFilesystem(
	ctx context.Context,
	filesystemID string,
) (storageprovisioning.Filesystem, error) {
	db, err := st.DB()
	if err != nil {
		return storageprovisioning.Filesystem{}, errors.Capture(err)
	}

	fs := filesystem{FilesystemID: filesystemID}
	stmt, err := st.Prepare(`
SELECT (
	sfs.filesystem_id,
	sv.volume_id,
	sfs.size_mib
) AS (&filesystem.*)
FROM      storage_filesystem sfs
LEFT JOIN storage_instance_filesystem sifs ON sfs.uuid = sifs.storage_filesystem_uuid
LEFT JOIN storage_instance si ON sifs.storage_instance_uuid = si.uuid
LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
WHERE     sfs.filesystem_id=$filesystem.filesystem_id
`,
		fs,
	)
	if err != nil {
		return storageprovisioning.Filesystem{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, fs).Get(&fs)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("filesystem %q not found", filesystemID).
				Add(storageprovisioningerrors.FilesystemNotFound)
		}
		return err
	})
	if err != nil {
		return storageprovisioning.Filesystem{}, errors.Capture(err)
	}

	var backingVolume *storageprovisioning.FilesystemBackingVolume
	if fs.VolumeID.Valid {
		backingVolume = &storageprovisioning.FilesystemBackingVolume{
			VolumeID: fs.VolumeID.V,
		}
	}

	return storageprovisioning.Filesystem{
		BackingVolume: backingVolume,
		FilesystemID:  fs.FilesystemID,
		Size:          fs.Size,
	}, nil
}

// GetFilesystemAttachment retrieves the [storageprovisioning.FilesystemAttachment]
// for the supplied net node uuid and filesystem id.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem attachment
// exists for the provided filesystem id.
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists for
// the provided filesystem id.
func (st *State) GetFilesystemAttachment(
	ctx context.Context,
	netNodeUUID domainnetwork.NetNodeUUID,
	filesystemIDStr string,
) (storageprovisioning.FilesystemAttachment, error) {
	db, err := st.DB()
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	fs := filesystemID{ID: filesystemIDStr}
	checkFilesystemExistsStmt, err := st.Prepare(`
SELECT &filesystemID.*
FROM   storage_filesystem
WHERE  filesystem_id = $filesystemID.filesystem_id`, fs)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	netNodeInput := netNodeUUIDRef{UUID: netNodeUUID.String()}
	attachment := filesystemAttachment{
		FilesystemID: filesystemIDStr,
	}

	stmt, err := st.Prepare(`
SELECT &filesystemAttachment.*
FROM   storage_filesystem_attachment sfa
JOIN   storage_filesystem sf ON sfa.storage_filesystem_uuid = sf.uuid
WHERE  sf.filesystem_id = $filesystemAttachment.filesystem_id
AND    sfa.net_node_uuid = $netNodeUUIDRef.net_node_uuid
`, attachment, netNodeInput)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, netNodeUUID)
		if err != nil {
			return errors.Capture(err)
		} else if !exists {
			return errors.Errorf("net node %q does not exist", netNodeUUID)
		}

		err = tx.Query(ctx, checkFilesystemExistsStmt, fs).Get(&fs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("filesystem %q not found", filesystemIDStr).
				Add(storageprovisioningerrors.FilesystemNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, stmt, attachment, netNodeInput).Get(&attachment)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf(
				"filesystem attachment for filesystem %q on net node %q not found",
				filesystemIDStr, netNodeUUID,
			).Add(storageprovisioningerrors.FilesystemAttachmentNotFound)
		}
		return err
	})
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}
	return storageprovisioning.FilesystemAttachment{
		FilesystemID: attachment.FilesystemID,
		MountPoint:   attachment.MountPoint,
		ReadOnly:     attachment.ReadOnly,
	}, nil
}

// GetFilesystemAttachmentIDs returns the
// [storageprovisioning.FilesystemAttachmentID] information for each
// filesystem attachment uuid supplied. If a uuid does not exist or isn't
// attached to either a machine or a unit then it will not exist in the
// result.
//
// It is not considered an error if a filesystem attachment uuid no longer
// exists as it is expected the caller has already satisfied this
// requirement themselves.
//
// All returned values will have either the machine name or unit name value
// filled out in the [storageprovisioning.FilesystemAttachmentID] struct.
func (st *State) GetFilesystemAttachmentIDs(
	ctx context.Context, uuids []string,
) (map[string]storageprovisioning.FilesystemAttachmentID, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	uuidInputs := filesystemAttachmentUUIDs(uuids)

	// To statisfy the unit name column of this union query a filesystem attachment
	// must be for a netnode uuid that is on a unit where that unit does not
	// share a netnode with a machine. If units are for machines they share a
	// netnode.
	q := `
SELECT &filesystemAttachmentIDs.* FROM (
    SELECT sfa.uuid,
           sf.filesystem_id,
           m.name AS machine_name,
           NULL AS unit_name
    FROM   storage_filesystem_attachment sfa
    JOIN   storage_filesystem sf ON sfa.storage_filesystem_uuid = sf.uuid
    JOIN   machine m ON sfa.net_node_uuid = m.net_node_uuid
    WHERE  sfa.uuid IN ($filesystemAttachmentUUIDs[:])
    UNION
    SELECT     sfa.uuid,
               sf.filesystem_id,
               NULL AS machine_name,
               u.name AS unit_name
    FROM       storage_filesystem_attachment sfa
    JOIN       storage_filesystem sf ON sfa.storage_filesystem_uuid = sf.uuid
    LEFT JOIN  machine m ON sfa.net_node_uuid == m.net_node_uuid
    JOIN       unit u ON sfa.net_node_uuid = u.net_node_uuid
    WHERE      sfa.uuid IN ($filesystemAttachmentUUIDs[:])
    AND        m.net_node_uuid IS NULL
)
`

	uuidToIDsStmt, err := st.Prepare(q, filesystemAttachmentIDs{}, uuidInputs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []filesystemAttachmentIDs
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, uuidToIDsStmt, uuidInputs).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make(map[string]storageprovisioning.FilesystemAttachmentID, len(dbVals))
	for _, v := range dbVals {
		id := storageprovisioning.FilesystemAttachmentID{
			FilesystemID: v.FilesystemID,
		}
		if v.MachineName.Valid {
			machineName := coremachine.Name(v.MachineName.String)
			id.MachineName = &machineName
		}
		if v.UnitName.Valid {
			unitName := coreunit.Name(v.UnitName.String)
			id.UnitName = &unitName
		}

		rval[v.UUID] = id
	}
	return rval, nil
}

// GetFilesystemAttachmentLifeForNetNode returns a mapping of filesystem
// attachment uuids to the current life value for each machine provisioned
// filesystem attachment that is to be provisioned by the machine owning the
// supplied net node.
func (st *State) GetFilesystemAttachmentLifeForNetNode(
	ctx context.Context,
	netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getFilesystemAttachmentLifeForNetNode(ctx, db, netNodeUUID)
}

// getFilesystemAttachmentLifeForNetNode returns a mapping of filesystem
// attachment uuids to the current life value for each machine provisioned
// filesystem attachment that is to be provisioned by the machine owning the
// supplied net node.
func (st *State) getFilesystemAttachmentLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]life.Life, error) {
	netNodeInput := netNodeUUIDRef{UUID: netNodeUUID.String()}
	stmt, err := st.Prepare(`
SELECT DISTINCT &attachmentLife.*
FROM            storage_filesystem_attachment
WHERE           provision_scope_id=1
AND             net_node_uuid=$netNodeUUIDRef.net_node_uuid
		`, attachmentLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var fsAttachmentLives attachmentLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, netNodeUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", netNodeUUID)
		}
		err = tx.Query(ctx, stmt, netNodeInput).GetAll(&fsAttachmentLives)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return maps.Collect(fsAttachmentLives.Iter), nil
}

// GetFilesystemLifeForNetNode returns a mapping of filesystem ids to current
// life value for each machine provisioned filesystem that is to be
// provisioned by the machine owning the supplied net node.
func (st *State) GetFilesystemLifeForNetNode(
	ctx context.Context,
	netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getFilesystemLifeForNetNode(ctx, db, netNodeUUID)
}

// getFilesystemLifeForNetNode returns a mapping of filesystem ids to current
// life value for each machine provisioned filesystem that is to be
// provisioned by the machine owning the supplied net node.
func (st *State) getFilesystemLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]life.Life, error) {
	netNodeInput := netNodeUUIDRef{UUID: netNodeUUID.String()}
	stmt, err := st.Prepare(`
SELECT DISTINCT (sf.filesystem_id, sf.life_id) AS (&filesystemLife.*)
FROM            storage_filesystem sf
JOIN            storage_filesystem_attachment sfa ON sf.uuid=sfa.storage_filesystem_uuid
WHERE           sf.provision_scope_id=1
AND             sfa.net_node_uuid=$netNodeUUIDRef.net_node_uuid
		`, filesystemLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var fsLives filesystemLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, netNodeUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", netNodeUUID)
		}
		err = tx.Query(ctx, stmt, netNodeInput).GetAll(&fsLives)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return maps.Collect(fsLives.Iter), nil
}

// InitialWatchStatementMachineProvisionedFilesystems returns both the
// namespace for watching filesystem life changes where the filesystem is
// machine provisioned. On top of this the initial query for getting all
// filesystems in the model that are machine provisioned is returned.
//
// Only filesystems that can be provisioned by the machine connected to the
// supplied net node will be emitted.
func (st *State) InitialWatchStatementMachineProvisionedFilesystems(
	netNodeUUID domainnetwork.NetNodeUUID,
) (string, eventsource.Query[map[string]life.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (
		map[string]life.Life, error,
	) {
		return st.getFilesystemLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_filesystem_life_machine_provisioning", query
}

// InitialWatchStatementModelProvisionedFilesystems returns both the namespace
// for watching filesystem life changes where the filesystem is model
// provisioned. On top of this the initial query for getting all filesystems
// in the model that model provisioned is returned.
func (st *State) InitialWatchStatementModelProvisionedFilesystems() (string, eventsource.NamespaceQuery) {
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT &filesystemID.*
FROM storage_filesystem
WHERE provision_scope_id=0
`,
			filesystemID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var fsIDs []filesystemID
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&fsIDs)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, errors.Capture(err)
		}
		rval := make([]string, 0, len(fsIDs))
		for _, v := range fsIDs {
			rval = append(rval, v.ID)
		}
		return rval, nil
	}
	return "storage_filesystem_life_model_provisioning", query
}

// InitialWatchStatementMachineProvisionedFilesystemAttachments returns
// both the namespace for watching filesystem attachment life changes where
// the filesystem attachment is machine provisioned and the initial query
// for getting the current set of machine provisioned filesystem attachments.
//
// Only filesystem attachments that can be provisioned by the machine
// connected to the supplied net node will be emitted.
func (st *State) InitialWatchStatementMachineProvisionedFilesystemAttachments(
	netNodeUUID domainnetwork.NetNodeUUID,
) (string, eventsource.Query[map[string]life.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (map[string]life.Life, error) {
		return st.getFilesystemAttachmentLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_filesystem_attachment_life_machine_provisioning", query
}

// InitialWatchStatementModelProvisionedFilesystems returns both the namespace
// for watching filesystem life changes where the filesystem is model
// provisioned. On top of this the initial query for getting all filesystems
// in the model that model provisioned is returned.
func (st *State) InitialWatchStatementModelProvisionedFilesystemAttachments() (string, eventsource.NamespaceQuery) {
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT &attachmentUUID.*
FROM   storage_filesystem_attachment
WHERE  provision_scope_id=0
		`, attachmentUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var fsAttachmentUUIDs []attachmentUUID
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&fsAttachmentUUIDs)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, errors.Capture(err)
		}
		rval := make([]string, 0, len(fsAttachmentUUIDs))
		for _, v := range fsAttachmentUUIDs {
			rval = append(rval, v.UUID)
		}
		return rval, nil
	}
	return "storage_filesystem_attachment_life_model_provisioning", query
}
