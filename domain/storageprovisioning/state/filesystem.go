// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
)

// GetFilesystemTemplatesForApplication returns all the filesystem templates for
// a given application.
func (st *State) GetFilesystemTemplatesForApplication(
	ctx context.Context,
	appUUID coreapplication.ID,
) ([]storageprovisioning.FilesystemTemplate, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	id := entityUUID{
		UUID: appUUID.String(),
	}

	fsTemplateQuery, err := st.Prepare(`
WITH
	app_fs_template_with_type AS (	
		SELECT asd.storage_name,
			asd.size_mib,
			asd.count,
			asd.storage_type,
			cs.read_only,
			cs.location,
			cs.count_max
		FROM application_storage_directive asd
		JOIN charm_storage cs ON asd.charm_uuid = cs.charm_uuid AND asd.storage_name = cs.name
		WHERE application_uuid = $entityUUID.uuid
		AND asd.storage_pool_uuid IS NULL
	),
	app_fs_template_from_pool AS (
		SELECT asd.storage_name,
			asd.size_mib,
			asd.count,
			sp.type AS storage_type,
			cs.read_only,
			cs.location,
			cs.count_max
		FROM application_storage_directive asd
		JOIN storage_pool sp ON asd.storage_pool_uuid = sp.uuid
		JOIN charm_storage cs ON asd.charm_uuid = cs.charm_uuid AND asd.storage_name = cs.name
		WHERE application_uuid = $entityUUID.uuid
		AND asd.storage_type IS NULL
	),
	app_fs_template AS (
		SELECT * FROM app_fs_template_with_type 
		UNION
		SELECT * FROM app_fs_template_from_pool
	)
SELECT &filesystemTemplate.* FROM app_fs_template
`, filesystemTemplate{}, id)
	if err != nil {
		return nil, errors.Capture(err)
	}

	fsAttributeQuery, err := st.Prepare(`
SELECT (asd.storage_name, key, value) AS (&storageNameAttributes.*) 
FROM storage_pool_attribute spa
JOIN storage_pool sp ON spa.storage_pool_uuid = sp.uuid
JOIN application_storage_directive asd ON sp.uuid = asd.storage_pool_uuid
WHERE application_uuid = $entityUUID.uuid
ORDER BY asd.storage_name
`, storageNameAttributes{}, id)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var fsTemplates []filesystemTemplate
	var fsAttributes []storageNameAttributes

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkApplicationExists(ctx, tx, appUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("application %q does not exist", appUUID)
		}
		err = tx.Query(ctx, fsTemplateQuery, id).GetAll(&fsTemplates)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		err = tx.Query(ctx, fsAttributeQuery, id).GetAll(&fsAttributes)
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

	attrs := map[string]map[string]string{}
	for _, attr := range fsAttributes {
		storageAttrs := attrs[attr.StorageName]
		if storageAttrs == nil {
			storageAttrs = map[string]string{}
			attrs[attr.StorageName] = storageAttrs
		}
		storageAttrs[attr.Key] = attr.Value
	}

	r := make([]storageprovisioning.FilesystemTemplate, 0, len(fsTemplates))
	for _, v := range fsTemplates {
		r = append(r, storageprovisioning.FilesystemTemplate{
			StorageName:  v.StorageName,
			Count:        v.Count,
			MaxCount:     v.MaxCount,
			SizeMiB:      v.SizeMiB,
			ProviderType: v.ProviderType,
			ReadOnly:     v.ReadOnly,
			Location:     v.Location,
			Attributes:   attrs[v.StorageName],
		})
	}
	return r, nil
}

// checkFilesystemExists checks if a filesystem for the provided uuid exists.
// True is returned when the filesystem exists.
func (st *State) checkFilesystemExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid storageprovisioning.FilesystemUUID,
) (bool, error) {
	entityUUIDInput := entityUUID{UUID: uuid.String()}

	checkQuery, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_filesystem
WHERE  uuid = $entityUUID.uuid
`,
		entityUUIDInput,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkQuery, entityUUIDInput).Get(&entityUUIDInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// CheckFilesystemExists checks if a filesystem exists for the supplied
// filesystem id. True is returned when a filesystem exists for the supplied
// id.
func (st *State) CheckFilesystemForIDExists(
	ctx context.Context, fsID string,
) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	filesystemIDInput := filesystemID{ID: fsID}
	checkQuery, err := st.Prepare(`
SELECT &filesystemID.*
FROM   storage_filesystem
WHERE  filesystem_id=$filesystemID.filesystem_id
`,
		filesystemIDInput,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	var exists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, checkQuery, filesystemIDInput).Get(&filesystemIDInput)
		if err == nil {
			exists = true
			return nil
		} else if errors.Is(err, sqlair.ErrNoRows) {
			exists = false
			return nil
		}
		return err
	})

	if err != nil {
		return false, errors.Capture(err)
	}

	return exists, nil
}

// GetFilesystem retrieves the [domainstorageprovisioning.Filesystem] for the
// supplied filesystem uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem
// exists for the provided filesystem uuid.
func (st *State) GetFilesystem(
	ctx context.Context,
	uuid storageprovisioning.FilesystemUUID,
) (storageprovisioning.Filesystem, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.Filesystem{}, errors.Capture(err)
	}

	var (
		uuidInput = filesystemUUID{UUID: uuid.String()}
		dbVal     filesystem
	)

	stmt, err := st.Prepare(`
SELECT (
  sfs.filesystem_id,
  sfs.provider_id,
  sv.volume_id,
  sfs.size_mib
) AS (&filesystem.*)
FROM      storage_filesystem sfs
LEFT JOIN storage_instance_filesystem sifs ON sfs.uuid = sifs.storage_filesystem_uuid
LEFT JOIN storage_instance si ON sifs.storage_instance_uuid = si.uuid
LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
WHERE     sfs.uuid = $filesystemUUID.uuid
`,
		uuidInput, dbVal,
	)
	if err != nil {
		return storageprovisioning.Filesystem{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, uuidInput).Get(&dbVal)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("filesystem %q not found", uuid).
				Add(storageprovisioningerrors.FilesystemNotFound)
		}
		return err
	})

	if err != nil {
		return storageprovisioning.Filesystem{}, errors.Capture(err)
	}

	var backingVolume *storageprovisioning.FilesystemBackingVolume
	if dbVal.VolumeID.Valid {
		backingVolume = &storageprovisioning.FilesystemBackingVolume{
			VolumeID: dbVal.VolumeID.V,
		}
	}

	return storageprovisioning.Filesystem{
		BackingVolume: backingVolume,
		FilesystemID:  dbVal.FilesystemID,
		ProviderID:    dbVal.ProviderID,
		SizeMiB:       dbVal.SizeMiB,
	}, nil
}

// GetFilesystemAttachment retrieves the
// [domainstorageprovisioning.FilesystemAttachment] for the supplied filesystem
// attachment uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the provided filesystem attachment uuid.
func (st *State) GetFilesystemAttachment(
	ctx context.Context,
	uuid storageprovisioning.FilesystemAttachmentUUID,
) (storageprovisioning.FilesystemAttachment, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	var (
		uuidInput = filesystemAttachmentUUID{UUID: uuid.String()}
		dbVal     filesystemAttachment
	)

	stmt, err := st.Prepare(`
SELECT &filesystemAttachment.*
FROM   storage_filesystem_attachment sfa
JOIN   storage_filesystem sf ON sfa.storage_filesystem_uuid = sf.uuid
WHERE  sfa.uuid = $filesystemAttachmentUUID.uuid
`,
		uuidInput, dbVal,
	)
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, uuidInput).Get(&dbVal)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf(
				"filesystem attachment %q not found", uuid,
			).Add(storageprovisioningerrors.FilesystemAttachmentNotFound)
		}
		return err
	})
	if err != nil {
		return storageprovisioning.FilesystemAttachment{}, errors.Capture(err)
	}
	return storageprovisioning.FilesystemAttachment{
		FilesystemID: dbVal.FilesystemID,
		MountPoint:   dbVal.MountPoint,
		ReadOnly:     dbVal.ReadOnly,
	}, nil
}

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
// All returned values will have either the machine name or unit name value
// filled out in the [domainstorageprovisioning.FilesystemAttachmentID] struct.
func (st *State) GetFilesystemAttachmentIDs(
	ctx context.Context, uuids []string,
) (map[string]storageprovisioning.FilesystemAttachmentID, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
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

// GetFilesystemAttachmentLife returns the current life value for a
// filesystem attachment uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the provided uuid.
func (st *State) GetFilesystemAttachmentLife(
	ctx context.Context,
	uuid storageprovisioning.FilesystemAttachmentUUID,
) (domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return 0, errors.Capture(err)
	}

	var (
		uuidInput = filesystemAttachmentUUID{UUID: uuid.String()}
		lifeDBVal entityLife
	)

	lifeQuery, err := st.Prepare(`
SELECT &entityLife.*
FROM   storage_filesystem_attachment
WHERE  uuid = $filesystemAttachmentUUID.uuid
`,
		uuidInput, lifeDBVal,
	)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lifeQuery, uuidInput).Get(&lifeDBVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"filesystem attachment %q does not exist", uuid,
			).Add(storageprovisioningerrors.FilesystemAttachmentNotFound)
		}
		return err
	})

	if err != nil {
		return 0, errors.Capture(err)
	}

	return domainlife.Life(lifeDBVal.LifeID), nil
}

// GetFilesystemAttachmentLifeForNetNode returns a mapping of filesystem
// attachment uuids to the current life value for each machine provisioned
// filesystem attachment that is to be provisioned by the machine owning the
// supplied net node.
func (st *State) GetFilesystemAttachmentLifeForNetNode(
	ctx context.Context,
	netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	db, err := st.DB(ctx)
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
	uuid domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	netNodeInput := netNodeUUID{UUID: uuid.String()}
	stmt, err := st.Prepare(`
SELECT DISTINCT &attachmentLife.*
FROM            storage_filesystem_attachment
WHERE           provision_scope_id=1
AND             net_node_uuid=$netNodeUUID.uuid
		`, attachmentLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var fsAttachmentLives attachmentLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", uuid)
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

// GetFilesystemAttachmentUUIDForFilesystemNetNode returns the filesystem
// attachment uuid for the supplied filesystem uuid which is attached to the
// given net node uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the supplied uuid.
// - [networkerrors.NetNodeNotFound] when no net node exists for the supplied
// net node uuid.
// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
// attachment exists for the supplied values.
func (st *State) GetFilesystemAttachmentUUIDForFilesystemNetNode(
	ctx context.Context,
	fsUUID storageprovisioning.FilesystemUUID,
	nodeUUID domainnetwork.NetNodeUUID,
) (storageprovisioning.FilesystemAttachmentUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		fsUUIDInput  = filesystemUUID{UUID: fsUUID.String()}
		netNodeInput = netNodeUUID{UUID: nodeUUID.String()}
		dbVal        entityUUID
	)

	uuidQuery, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_filesystem_attachment
WHERE  storage_filesystem_uuid = $filesystemUUID.uuid
AND    net_node_uuid = $netNodeUUID.uuid
	`,
		dbVal, netNodeInput, fsUUIDInput,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkFilesystemExists(ctx, tx, fsUUID)
		if err != nil {
			return errors.Errorf(
				"checking if filesystem %q exists: %w", fsUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"filesystem %q does not exist", fsUUID,
			).Add(storageprovisioningerrors.FilesystemNotFound)
		}

		exists, err = st.checkNetNodeExists(ctx, tx, nodeUUID)
		if err != nil {
			return errors.Errorf(
				"checking net node uuid %q exists: %w", nodeUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"net node %q does not exist", nodeUUID,
			).Add(networkerrors.NetNodeNotFound)
		}

		err = tx.Query(ctx, uuidQuery, fsUUIDInput, netNodeInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"filesystem attachment does not exist",
			).Add(storageprovisioningerrors.FilesystemAttachmentNotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return storageprovisioning.FilesystemAttachmentUUID(dbVal.UUID), nil
}

// GetFilesystemLife returns the current life value for a filesystem uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the provided uuid.
func (st *State) GetFilesystemLife(
	ctx context.Context,
	uuid storageprovisioning.FilesystemUUID,
) (domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return 0, errors.Capture(err)
	}

	var (
		uuidInput = filesystemUUID{UUID: uuid.String()}
		lifeDBVal entityLife
	)

	lifeQuery, err := st.Prepare(`
SELECT &entityLife.*
FROM   storage_filesystem
WHERE  uuid = $filesystemUUID.uuid
`,
		uuidInput, lifeDBVal,
	)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lifeQuery, uuidInput).Get(&lifeDBVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"filesystem %q does not exist", uuid,
			).Add(storageprovisioningerrors.FilesystemNotFound)
		}
		return err
	})

	if err != nil {
		return 0, errors.Capture(err)
	}

	return domainlife.Life(lifeDBVal.LifeID), nil
}

// GetFilesystemLifeForNetNode returns a mapping of filesystem ids to current
// life value for each machine provisioned filesystem that is to be
// provisioned by the machine owning the supplied net node.
func (st *State) GetFilesystemLifeForNetNode(
	ctx context.Context,
	netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	db, err := st.DB(ctx)
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
	uuid domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	netNodeInput := netNodeUUID{UUID: uuid.String()}
	stmt, err := st.Prepare(`
SELECT DISTINCT (sf.filesystem_id, sf.life_id) AS (&filesystemLife.*)
FROM            storage_filesystem sf
JOIN            storage_filesystem_attachment sfa ON sf.uuid=sfa.storage_filesystem_uuid
WHERE           sf.provision_scope_id=1
AND             sfa.net_node_uuid=$netNodeUUID.uuid
		`, filesystemLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var fsLives filesystemLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", uuid)
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

// GetFilesystemUUIDForID returns the uuid for a filesystem with the supplied
// id.
//
// The following errors may be returned:
// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
// for the provided filesystem id.
func (st *State) GetFilesystemUUIDForID(
	ctx context.Context, fsID string,
) (storageprovisioning.FilesystemUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		idInput = filesystemID{ID: fsID}
		dbVal   entityUUID
	)
	uuidQuery, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_filesystem
WHERE  filesystem_id = $filesystemID.filesystem_id
`,
		idInput, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, uuidQuery, idInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"filesystem for id %q does not exist", fsID,
			).Add(storageprovisioningerrors.FilesystemNotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return storageprovisioning.FilesystemUUID(dbVal.UUID), nil
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
) (string, eventsource.Query[map[string]domainlife.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (
		map[string]domainlife.Life, error,
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
) (string, eventsource.Query[map[string]domainlife.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (map[string]domainlife.Life, error) {
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
SELECT &entityUUID.*
FROM   storage_filesystem_attachment
WHERE  provision_scope_id=0
		`, entityUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var fsAttachmentUUIDs []entityUUID
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

// SetFilesystemProvisionedInfo sets on the provided filesystem the
// information about the provisioned filesystem.
func (st *State) SetFilesystemProvisionedInfo(
	ctx context.Context,
	filesystemUUID storageprovisioning.FilesystemUUID,
	info storageprovisioning.FilesystemProvisionedInfo,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	fs := filesystemProvisionedInfo{
		UUID:       filesystemUUID.String(),
		ProviderID: info.ProviderID,
		SizeMiB:    info.SizeMiB,
	}
	stmt, err := st.Prepare(`
UPDATE storage_filesystem
SET    provider_id = $filesystemProvisionedInfo.provider_id,
       size_mib = $filesystemProvisionedInfo.size_mib
WHERE  uuid = $filesystemProvisionedInfo.uuid
`, fs)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkFilesystemExists(ctx, tx, filesystemUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf(
				"filesystem %q does not exist", filesystemUUID,
			).Add(storageprovisioningerrors.FilesystemNotFound)
		}
		return tx.Query(ctx, stmt, fs).Run()
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// SetFilesystemAttachmentProvisionedInfo sets on the provided filesystem
// attachment information about the provisoned filesystem attachment.
func (st *State) SetFilesystemAttachmentProvisionedInfo(
	ctx context.Context,
	filesystemAttachmentUUID storageprovisioning.FilesystemAttachmentUUID,
	info storageprovisioning.FilesystemAttachmentProvisionedInfo,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	fs := filesystemAttachmentProvisionedInfo{
		UUID:       filesystemAttachmentUUID.String(),
		MountPoint: info.MountPoint,
		ReadOnly:   info.ReadOnly,
	}
	stmt, err := st.Prepare(`
UPDATE storage_filesystem_attachment
SET    mount_point = $filesystemAttachmentProvisionedInfo.mount_point,
       read_only = $filesystemAttachmentProvisionedInfo.read_only
WHERE  uuid = $filesystemAttachmentProvisionedInfo.uuid
`, fs)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		err := tx.Query(ctx, stmt, fs).Get(&outcome)
		if err != nil {
			return err
		}

		result := outcome.Result()
		if affected, err := result.RowsAffected(); err != nil {
			return errors.Errorf("getting number of affected rows: %w", err)
		} else if affected == 0 {
			return errors.Errorf(
				"filesystem attachment %v not found",
				filesystemAttachmentUUID,
			).Add(storageprovisioningerrors.FilesystemAttachmentNotFound)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}
