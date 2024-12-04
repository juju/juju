// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/errors"
)

// GetCharmID returns the charm ID by the natural key, for a
// specific revision and source.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharmID(ctx context.Context, name string, revision int, source charm.CharmSource) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	sourceID, err := encodeCharmSource(source)
	if err != nil {
		return "", internalerrors.Errorf("failed to encode charm source: %w", err)
	}

	var ident charmID
	charmRef := charmReferenceNameRevisionSource{
		ReferenceName: name,
		Revision:      revision,
		Source:        sourceID,
	}

	query := `
SELECT &charmID.*
FROM charm
WHERE reference_name = $charmReferenceNameRevisionSource.reference_name
AND revision = $charmReferenceNameRevisionSource.revision
AND source_id = $charmReferenceNameRevisionSource.source_id;
`
	stmt, err := s.Prepare(query, ident, charmRef)
	if err != nil {
		return "", errors.Errorf("failed to prepare query: %w", err)
	}

	var id corecharm.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, charmRef).Get(&ident); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return errors.Errorf("getting charm ID: %w", err)
		}
		id = corecharm.ID(ident.UUID)
		return nil
	}); err != nil {
		return "", errors.Errorf("getting charm id by revision and source: %w", err)
	}
	return id, nil
}

// IsControllerCharm returns whether the charm is a controller charm.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	var result charmName
	ident := charmID{UUID: id.String()}

	query := `
SELECT cm.name AS &charmName.name
FROM charm
JOIN charm_metadata AS cm ON charm.uuid = cm.charm_uuid
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, result)
	if err != nil {
		return false, errors.Errorf("failed to prepare query: %w", err)
	}

	var isController bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return errors.Errorf("getting charm ID: %w", err)
		}
		isController = result.Name == "juju-controller"
		return nil
	}); err != nil {
		return false, errors.Errorf("failed to is controller charm: %w", err)
	}
	return isController, nil
}

// IsSubordinateCharm returns whether the charm is a subordinate charm.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) IsSubordinateCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	var result charmSubordinate
	ident := charmID{UUID: id.String()}

	query := `
SELECT cm.subordinate AS &charmSubordinate.subordinate
FROM charm
JOIN charm_metadata AS cm ON charm.uuid = cm.charm_uuid
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, result)
	if err != nil {
		return false, errors.Errorf("failed to prepare query: %w", err)
	}

	var isSubordinate bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return errors.Errorf("getting charm ID: %w", err)
		}
		isSubordinate = result.Subordinate
		return nil
	}); err != nil {
		return false, errors.Errorf("failed to is subordinate charm: %w", err)
	}
	return isSubordinate, nil
}

// SupportsContainers returns whether the charm supports containers.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) SupportsContainers(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT charm_container.charm_uuid AS &charmID.uuid
FROM charm
LEFT JOIN charm_container
ON charm.uuid = charm_container.charm_uuid
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident)
	if err != nil {
		return false, errors.Errorf("failed to prepare query: %w", err)
	}

	var supportsContainers bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []charmID
		if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return errors.Errorf("getting charm ID: %w", err)
		}
		var num int
		for _, r := range result {
			if r.UUID == id.String() {
				num++
			}
		}
		supportsContainers = num > 0
		return nil
	}); err != nil {
		return false, errors.Errorf("failed to supports containers: %w", err)
	}
	return supportsContainers, nil
}

// IsCharmAvailable returns whether the charm is available for use.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) IsCharmAvailable(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	var result charmAvailable
	ident := charmID{UUID: id.String()}

	query := `
SELECT charm.available AS &charmAvailable.available
FROM charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, result)
	if err != nil {
		return false, errors.Errorf("failed to prepare query: %w", err)
	}

	var isAvailable bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return errors.Errorf("getting charm ID: %w", err)
		}
		isAvailable = result.Available
		return nil
	}); err != nil {
		return false, errors.Errorf("failed to is charm available: %w", err)
	}
	return isAvailable, nil
}

// SetCharmAvailable sets the charm as available for use.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) SetCharmAvailable(ctx context.Context, id corecharm.ID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	selectQuery := `
SELECT &charmID.*
FROM charm
WHERE uuid = $charmID.uuid;
	`

	selectStmt, err := s.Prepare(selectQuery, ident)
	if err != nil {
		return errors.Errorf("failed to prepare query: %w", err)
	}

	updateQuery := `
UPDATE charm
SET available = true
WHERE uuid = $charmID.uuid;
`

	updateStmt, err := s.Prepare(updateQuery, ident)
	if err != nil {
		return errors.Errorf("failed to prepare query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmID
		if err := tx.Query(ctx, selectStmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return errors.Errorf("failed to set charm available: %w", err)
		}

		if err := tx.Query(ctx, updateStmt, ident).Run(); err != nil {
			return errors.Errorf("failed to set charm available: %w", err)
		}
		return nil
	}); err != nil {
		return errors.Errorf("failed to set charm available: %w", err)
	}

	return nil
}

// GetCharmArchivePath returns the archive storage path for the charm using
// the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharmArchivePath(ctx context.Context, id corecharm.ID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var archivePath charmArchivePath
	ident := charmID{UUID: id.String()}

	query := `
SELECT &charmArchivePath.*
FROM charm
WHERE uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, archivePath, ident)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&archivePath); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return err
		}
		return nil
	}); err != nil {
		return "", errors.Errorf("getting charm archive path: %w", err)
	}

	return archivePath.ArchivePath, nil
}

// GetCharmArchiveMetadata returns the archive storage path and the sha256 hash
// for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharmArchiveMetadata(ctx context.Context, id corecharm.ID) (archivePath string, hash string, err error) {
	db, err := s.DB()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	var archivePathAndHashes []charmArchivePathAndHash
	ident := charmID{UUID: id.String()}

	query := `
SELECT &charmArchivePathAndHash.*
FROM charm
JOIN charm_hash ON charm.uuid = charm_hash.charm_uuid
WHERE charm.uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, charmArchivePathAndHash{}, ident)
	if err != nil {
		return "", "", errors.Errorf("preparing query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).GetAll(&archivePathAndHashes); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return err
		}
		return nil
	}); err != nil {
		return "", "", errors.Errorf("getting charm archive metadata: %w", err)
	}
	if len(archivePathAndHashes) > 1 {
		return "", "", errors.Errorf("getting charm archive metadata: %w", applicationerrors.MultipleCharmHashes)
	}

	return archivePathAndHashes[0].ArchivePath, archivePathAndHashes[0].Hash, nil
}

// GetCharmMetadata returns the metadata for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharmMetadata(ctx context.Context, id corecharm.ID) (charm.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var charmMetadata charm.Metadata
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmMetadata, err = s.getMetadata(ctx, tx, ident)
		return errors.Capture(err)
	}); err != nil {
		return charm.Metadata{}, errors.Errorf("getting charm metadata: %w", err)
	}

	return charmMetadata, nil
}

// GetCharmMetadataName returns the name from the metadata for the charm using
// the charm ID. If the charm does not exist, a [errors.CharmNotFound] error is
// returned.
func (s *State) GetCharmMetadataName(ctx context.Context, id corecharm.ID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT name AS &charmMetadata.name
FROM v_charm_metadata
WHERE uuid = $charmID.uuid;`

	var metadata charmMetadata
	stmt, err := s.Prepare(query, metadata, ident)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&metadata); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return errors.Errorf("selecting charm metadata: %w", err)
		}
		return nil
	}); err != nil {
		return "", errors.Errorf("getting charm metadata: %w", err)
	}

	return metadata.Name, nil
}

// GetCharmMetadataDescription returns the description for the metadata for the
// charm using the charm ID. If the charm does not exist, a
// [errors.CharmNotFound] error is returned.
func (s *State) GetCharmMetadataDescription(ctx context.Context, id corecharm.ID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT description AS &charmMetadata.description
FROM v_charm_metadata
WHERE uuid = $charmID.uuid;`

	var metadata charmMetadata
	stmt, err := s.Prepare(query, metadata, ident)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&metadata); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return errors.Errorf("selecting charm metadata: %w", err)
		}
		return nil
	}); err != nil {
		return "", errors.Errorf("getting charm metadata: %w", err)
	}

	return metadata.Description, nil
}

// GetCharmManifest returns the manifest for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharmManifest(ctx context.Context, id corecharm.ID) (charm.Manifest, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Manifest{}, errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var manifest charm.Manifest
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		manifest, err = s.getCharmManifest(ctx, tx, ident)
		return errors.Capture(err)
	}); err != nil {
		return charm.Manifest{}, errors.Capture(err)
	}

	return manifest, nil
}

// GetCharmLXDProfile returns the LXD profile for the charm using the
// charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharmLXDProfile(ctx context.Context, id corecharm.ID) ([]byte, charm.Revision, error) {
	db, err := s.DB()
	if err != nil {
		return nil, -1, errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var (
		profile  []byte
		revision charm.Revision
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		profile, revision, err = s.getCharmLXDProfile(ctx, tx, ident)
		return errors.Capture(err)
	}); err != nil {
		return nil, -1, errors.Capture(err)
	}

	return profile, revision, nil
}

// GetCharmConfig returns the config for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharmConfig(ctx context.Context, id corecharm.ID) (charm.Config, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Config{}, errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var charmConfig charm.Config
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmConfig, err = s.getCharmConfig(ctx, tx, ident)
		return errors.Capture(err)
	}); err != nil {
		return charm.Config{}, errors.Capture(err)
	}
	return charmConfig, nil

}

// GetCharmActions returns the actions for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharmActions(ctx context.Context, id corecharm.ID) (charm.Actions, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Actions{}, errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var actions charm.Actions
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		actions, err = s.getCharmActions(ctx, tx, ident)
		return errors.Capture(err)
	}); err != nil {
		return charm.Actions{}, errors.Capture(err)
	}

	return actions, nil
}

// GetCharm returns the charm using the charm ID.
// DownloadInfo is optional, and is only returned for charms from a charm store.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) GetCharm(ctx context.Context, id corecharm.ID) (charm.Charm, *charm.DownloadInfo, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Charm{}, nil, errors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var (
		ch charm.Charm
		di *charm.DownloadInfo
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		ch, di, err = s.getCharm(ctx, tx, ident)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return ch, nil, errors.Errorf("getting charm: %w", err)
	}

	return ch, di, nil
}

// SetCharm persists the charm metadata, actions, config and manifest to
// state.
func (s *State) SetCharm(ctx context.Context, charm charm.Charm, downloadInfo *charm.DownloadInfo) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	id, err := corecharm.NewID()
	if err != nil {
		return "", errors.Errorf("setting charm: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check the charm doesn't already exist, if it does, return an already
		// exists error. Also doing this early, prevents the moving straight
		// to a write transaction.
		if _, err := s.checkCharmReferenceExists(ctx, tx, charm.ReferenceName, charm.Revision); err != nil {
			return errors.Capture(err)
		}

		if err := s.setCharm(ctx, tx, id, charm, downloadInfo); err != nil {
			return errors.Capture(err)
		}

		return nil
	}); err != nil {
		return "", errors.Errorf("setting charm: %w", err)
	}

	return id, nil
}

// ListCharmLocatorsByNames returns a list of charm locators.
// The locator allows the reconstruction of the charm URL for the client
// response.
func (s *State) ListCharmLocators(ctx context.Context) ([]charm.CharmLocator, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT &charmLocator.*
FROM v_charm_locator;
`
	stmt, err := s.Prepare(query, charmLocator{})
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var results []charmLocator
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).GetAll(&results); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting charm locators: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("listing charm locators: %w", err)
	}

	return decodeCharmLocators(results)
}

// ListCharmLocatorsByNames returns a list of charm locators for the specified
// charm names.
// The locator allows the reconstruction of the charm URL for the client
// response. If no names are provided, then nothing is returned.
func (s *State) ListCharmLocatorsByNames(ctx context.Context, names []string) ([]charm.CharmLocator, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	type nameSelector []string

	query := `
SELECT &charmLocator.*
FROM v_charm_locator
WHERE name IN ($nameSelector[:]);
`
	stmt, err := s.Prepare(query, charmLocator{}, nameSelector(names))
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var results []charmLocator
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, nameSelector(names)).GetAll(&results); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting charm locators by names: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Errorf("listing charm locators by names: %w", err)
	}

	return decodeCharmLocators(results)
}

// GetCharmDownloadInfo returns the download info for the charm using the
// charm ID.
func (s *State) GetCharmDownloadInfo(ctx context.Context, id corecharm.ID) (*charm.DownloadInfo, error) {
	db, err := s.DB()
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var downloadInfo *charm.DownloadInfo
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		info, err := s.getCharmDownloadInfo(ctx, tx, ident)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return internalerrors.Capture(err)
		}
		downloadInfo = &info

		return nil
	}); err != nil {
		return nil, internalerrors.Errorf("getting charm download info: %w", err)
	}

	return downloadInfo, nil
}

func decodeCharmLocators(results []charmLocator) ([]charm.CharmLocator, error) {
	return transform.SliceOrErr(results, func(c charmLocator) (charm.CharmLocator, error) {
		source, err := decodeCharmSource(c.SourceID)
		if err != nil {
			return charm.CharmLocator{}, err
		}

		architecture, err := decodeArchitecture(c.ArchitectureID)
		if err != nil {
			return charm.CharmLocator{}, err
		}

		return charm.CharmLocator{
			Name:         c.Name,
			Revision:     c.Revision,
			Source:       source,
			Architecture: architecture,
		}, nil
	})
}

var tablesToDeleteFrom = []string{
	"charm_action",
	"charm_category",
	"charm_config",
	"charm_container_mount",
	"charm_container",
	"charm_device",
	"charm_download_info",
	"charm_extra_binding",
	"charm_hash",
	"charm_manifest_base",
	"charm_payload",
	"charm_relation",
	"charm_resource",
	"charm_metadata",
	"charm_storage_property",
	"charm_storage",
	"charm_tag",
	"charm_term",
}

type deleteStatement struct {
	stmt      *sqlair.Statement
	tableName string
}

// DeleteCharm removes the charm from the state.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *State) DeleteCharm(ctx context.Context, id corecharm.ID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	selectQuery, err := s.Prepare(`SELECT charm.uuid AS &charmID.* FROM charm WHERE uuid = $charmID.uuid;`, charmID{})
	if err != nil {
		return errors.Errorf("failed to prepare query: %w", err)
	}

	deleteQuery, err := s.Prepare(`DELETE FROM charm WHERE uuid = $charmID.uuid;`, charmID{})
	if err != nil {
		return errors.Errorf("failed to prepare query: %w", err)
	}

	// Prepare the delete statements for each table.
	stmts := make([]deleteStatement, len(tablesToDeleteFrom))
	for i, table := range tablesToDeleteFrom {
		query := fmt.Sprintf("DELETE FROM %s WHERE charm_uuid = $charmUUID.charm_uuid;", table)

		stmt, err := s.Prepare(query, charmUUID{})
		if err != nil {
			return errors.Errorf("failed to prepare query: %w", err)
		}

		stmts[i] = deleteStatement{
			stmt:      stmt,
			tableName: table,
		}
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, selectQuery, charmID{UUID: id.String()}).Get(&charmID{}); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
		}

		// Delete the foreign key references first.
		for _, stmt := range stmts {
			if err := tx.Query(ctx, stmt.stmt, charmUUID{UUID: id.String()}).Run(); err != nil {
				return errors.Errorf("failed to delete related data for %q: %w", stmt.tableName, err)
			}
		}

		// Then delete the charm itself.
		if err := tx.Query(ctx, deleteQuery, charmID{UUID: id.String()}).Run(); err != nil {
			return errors.Errorf("failed to delete charm: %w", err)
		}

		return nil
	}); err != nil {
		return errors.Errorf("failed to delete charm: %w", err)
	}

	return nil
}
