// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

// hashKind is the type of hash to store.
type hashKind = int

const (
	// sha256HashKind is the ID for the SHA256 hash kind.
	sha256HashKind hashKind = 0
)

// CharmState is used to access the database.
type CharmState struct {
	*commonStateBase
}

// NewCharmState creates a state to access the database.
func NewCharmState(factory database.TxnRunnerFactory) *CharmState {
	return &CharmState{
		commonStateBase: &commonStateBase{
			StateBase: domain.NewStateBase(factory),
		},
	}
}

// GetCharmIDByRevision returns the charm ID by the natural key, for a
// specific revision.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmIDByRevision(ctx context.Context, name string, revision int) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	var ident charmID
	args := charmReferenceNameRevision{
		ReferenceName: name,
		Revision:      revision,
	}

	query := `
SELECT &charmID.*
FROM charm
INNER JOIN charm_origin
ON charm.uuid = charm_origin.charm_uuid
WHERE charm_origin.reference_name = $charmReferenceNameRevision.reference_name
AND charm_origin.revision = $charmReferenceNameRevision.revision;
`
	stmt, err := s.Prepare(query, ident, args)
	if err != nil {
		return "", internalerrors.Errorf("failed to prepare query: %w", err)
	}

	var id corecharm.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, args).Get(&ident); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("failed to get charm ID: %w", err)
		}
		id = corecharm.ID(ident.UUID)
		return nil
	}); err != nil {
		return "", internalerrors.Errorf("failed to get charm id by revision: %w", err)
	}
	return id, nil
}

// IsControllerCharm returns whether the charm is a controller charm.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, internalerrors.Capture(err)
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
		return false, internalerrors.Errorf("failed to prepare query: %w", err)
	}

	var isController bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("failed to get charm ID: %w", err)
		}
		isController = result.Name == "juju-controller"
		return nil
	}); err != nil {
		return false, internalerrors.Errorf("failed to is controller charm: %w", err)
	}
	return isController, nil
}

// IsSubordinateCharm returns whether the charm is a subordinate charm.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) IsSubordinateCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, internalerrors.Capture(err)
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
		return false, internalerrors.Errorf("failed to prepare query: %w", err)
	}

	var isSubordinate bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("failed to get charm ID: %w", err)
		}
		isSubordinate = result.Subordinate
		return nil
	}); err != nil {
		return false, internalerrors.Errorf("failed to is subordinate charm: %w", err)
	}
	return isSubordinate, nil
}

// SupportsContainers returns whether the charm supports containers.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) SupportsContainers(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, internalerrors.Capture(err)
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
		return false, internalerrors.Errorf("failed to prepare query: %w", err)
	}

	var supportsContainers bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []charmID
		if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("failed to get charm ID: %w", err)
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
		return false, internalerrors.Errorf("failed to supports containers: %w", err)
	}
	return supportsContainers, nil
}

// IsCharmAvailable returns whether the charm is available for use.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) IsCharmAvailable(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, internalerrors.Capture(err)
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
		return false, internalerrors.Errorf("failed to prepare query: %w", err)
	}

	var isAvailable bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("failed to get charm ID: %w", err)
		}
		isAvailable = result.Available
		return nil
	}); err != nil {
		return false, internalerrors.Errorf("failed to is charm available: %w", err)
	}
	return isAvailable, nil
}

// SetCharmAvailable sets the charm as available for use.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) SetCharmAvailable(ctx context.Context, id corecharm.ID) error {
	db, err := s.DB()
	if err != nil {
		return internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	selectQuery := `
SELECT &charmID.*
FROM charm
WHERE uuid = $charmID.uuid;
	`

	selectStmt, err := s.Prepare(selectQuery, ident)
	if err != nil {
		return internalerrors.Errorf("failed to prepare query: %w", err)
	}

	updateQuery := `
UPDATE charm
SET available = true
WHERE uuid = $charmID.uuid;
`

	updateStmt, err := s.Prepare(updateQuery, ident)
	if err != nil {
		return internalerrors.Errorf("failed to prepare query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmID
		if err := tx.Query(ctx, selectStmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("failed to set charm available: %w", err)
		}

		if err := tx.Query(ctx, updateStmt, ident).Run(); err != nil {
			return internalerrors.Errorf("failed to set charm available: %w", err)
		}
		return nil
	}); err != nil {
		return internalerrors.Errorf("failed to set charm available: %w", err)
	}

	return nil
}

// ReserveCharmRevision defines a placeholder for a new charm revision.
// The original charm will need to exist, the returning charm ID will be
// the new charm ID for the revision.
func (s *CharmState) ReserveCharmRevision(ctx context.Context, id corecharm.ID, revision int) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	var reserveCharmResult reserveCharm
	ident := charmID{UUID: id.String()}

	selectQuery := `
SELECT &reserveCharm.*
FROM charm
JOIN charm_metadata AS cm ON charm.uuid = cm.charm_uuid
JOIN charm_origin AS co ON charm.uuid = co.charm_uuid
WHERE uuid = $charmID.uuid;
`
	selectStmt, err := s.Prepare(selectQuery, ident, reserveCharmResult)
	if err != nil {
		return "", internalerrors.Errorf("failed to prepare query: %w", err)
	}

	insertCharmQuery := `INSERT INTO charm (*) VALUES ($charmID.*);`
	insertCharmStmt, err := s.Prepare(insertCharmQuery, ident)
	if err != nil {
		return "", internalerrors.Errorf("failed to prepare query: %w", err)
	}

	insertCharmMetadataQuery := `INSERT INTO charm_metadata (*) VALUES ($setCharmMetadata.*);`
	insertCharmMetadataStmt, err := s.Prepare(insertCharmMetadataQuery, setCharmMetadata{})
	if err != nil {
		return "", internalerrors.Errorf("failed to prepare query: %w", err)
	}

	insertCharmOriginQuery := `INSERT INTO charm_origin (*) VALUES ($setCharmOrigin.*);`
	insertCharmOriginStmt, err := s.Prepare(insertCharmOriginQuery, setCharmOrigin{})
	if err != nil {
		return "", internalerrors.Errorf("failed to prepare query: %w", err)
	}

	newID, err := corecharm.NewID()
	if err != nil {
		return "", internalerrors.Errorf("failed to reserve charm revision: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, selectStmt, ident).Get(&reserveCharmResult); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("failed to reserve charm revision: %w", err)
		}

		if err := tx.Query(ctx, insertCharmStmt, charmID{UUID: newID.String()}).Run(); err != nil {
			return internalerrors.Errorf("failed to reserve charm revision: inserting charm: %w", err)
		}

		if err := tx.Query(ctx, insertCharmMetadataStmt, setCharmMetadata{CharmUUID: newID.String(), Name: reserveCharmResult.Name}).Run(); err != nil {
			return internalerrors.Errorf("failed to reserve charm revision: inserting charm_metadata: %w", err)
		}

		if err := tx.Query(ctx, insertCharmOriginStmt, setCharmOrigin{CharmUUID: newID.String(), ReferenceName: reserveCharmResult.Name}).Run(); err != nil {
			return internalerrors.Errorf("failed to reserve charm revision: inserting charm_origin: %w", err)
		}

		return nil
	}); err != nil {
		return "", internalerrors.Errorf("failed to reserve charm revision: %w", err)
	}

	return newID, nil
}

// GetCharmArchivePath returns the archive storage path for the charm using
// the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmArchivePath(ctx context.Context, id corecharm.ID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", internalerrors.Capture(err)
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
		return "", internalerrors.Errorf("failed to prepare query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&archivePath); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("failed to get charm archive path: %w", err)
		}
		return nil
	}); err != nil {
		return "", internalerrors.Errorf("failed to get charm archive path: %w", domain.CoerceError(err))
	}

	return archivePath.ArchivePath, nil
}

// GetCharmMetadata returns the metadata for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmMetadata(ctx context.Context, id corecharm.ID) (charm.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Metadata{}, internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var charmMetadata charm.Metadata
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmMetadata, err = s.getMetadata(ctx, tx, ident)
		return internalerrors.Capture(err)
	}); err != nil {
		return charm.Metadata{}, internalerrors.Errorf("failed to get charm metadata: %w", err)
	}

	return charmMetadata, nil
}

// GetCharmMetadataName returns the name from the metadata for the charm using
// the charm ID. If the charm does not exist, a [errors.CharmNotFound] error is
// returned.
func (s *CharmState) GetCharmMetadataName(ctx context.Context, id corecharm.ID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT name AS &charmMetadata.name
FROM v_charm_metadata
WHERE uuid = $charmID.uuid;`

	var metadata charmMetadata
	stmt, err := s.Prepare(query, metadata, ident)
	if err != nil {
		return "", internalerrors.Errorf("preparing query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&metadata); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("selecting charm metadata: %w", err)
		}
		return nil
	}); err != nil {
		return "", internalerrors.Errorf("getting charm metadata: %w", err)
	}

	return metadata.Name, nil
}

// GetCharmMetadataDescription returns the description for the metadata for the
// charm using the charm ID. If the charm does not exist, a
// [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmMetadataDescription(ctx context.Context, id corecharm.ID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT description AS &charmMetadata.description
FROM v_charm_metadata
WHERE uuid = $charmID.uuid;`

	var metadata charmMetadata
	stmt, err := s.Prepare(query, metadata, ident)
	if err != nil {
		return "", internalerrors.Errorf("preparing query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&metadata); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return internalerrors.Errorf("selecting charm metadata: %w", err)
		}
		return nil
	}); err != nil {
		return "", internalerrors.Errorf("getting charm metadata: %w", err)
	}

	return metadata.Description, nil
}

// GetCharmManifest returns the manifest for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmManifest(ctx context.Context, id corecharm.ID) (charm.Manifest, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Manifest{}, internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var manifest charm.Manifest
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		manifest, err = s.getCharmManifest(ctx, tx, ident)
		return internalerrors.Capture(err)
	}); err != nil {
		return charm.Manifest{}, internalerrors.Capture(err)
	}

	return manifest, nil
}

// GetCharmLXDProfile returns the LXD profile for the charm using the
// charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmLXDProfile(ctx context.Context, id corecharm.ID) ([]byte, charm.Revision, error) {
	db, err := s.DB()
	if err != nil {
		return nil, -1, internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var (
		profile  []byte
		revision charm.Revision
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		profile, revision, err = s.getCharmLXDProfile(ctx, tx, ident)
		return internalerrors.Capture(err)
	}); err != nil {
		return nil, -1, internalerrors.Capture(err)
	}

	return profile, revision, nil
}

// GetCharmConfig returns the config for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmConfig(ctx context.Context, id corecharm.ID) (charm.Config, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Config{}, internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var charmConfig charm.Config
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmConfig, err = s.getCharmConfig(ctx, tx, ident)
		return internalerrors.Capture(err)
	}); err != nil {
		return charm.Config{}, internalerrors.Capture(err)
	}
	return charmConfig, nil

}

// GetCharmActions returns the actions for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmActions(ctx context.Context, id corecharm.ID) (charm.Actions, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Actions{}, internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var actions charm.Actions
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		actions, err = s.getCharmActions(ctx, tx, ident)
		return internalerrors.Capture(err)
	}); err != nil {
		return charm.Actions{}, internalerrors.Capture(err)
	}

	return actions, nil
}

// GetCharm returns the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharm(ctx context.Context, id corecharm.ID) (charm.Charm, charm.CharmOrigin, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Charm{}, charm.CharmOrigin{}, internalerrors.Capture(err)
	}

	ident := charmID{UUID: id.String()}

	var (
		ch     charm.Charm
		origin charm.CharmOrigin
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		ch, err = s.getCharm(ctx, tx, ident)
		if err != nil {
			return internalerrors.Capture(err)
		}
		origin, err = s.getCharmOrigin(ctx, tx, ident)
		if err != nil {
			return internalerrors.Capture(err)
		}
		return nil
	}); err != nil {
		return ch, origin, internalerrors.Errorf("failed to get charm: %w", err)
	}

	return ch, origin, nil
}

// SetCharm persists the charm metadata, actions, config and manifest to
// state.
func (s *CharmState) SetCharm(ctx context.Context, charm charm.Charm, charmArgs charm.SetStateArgs) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", internalerrors.Capture(err)
	}

	id, err := corecharm.NewID()
	if err != nil {
		return "", internalerrors.Errorf("setting charm: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check the charm doesn't already exist, if it does, return an already
		// exists error. Also doing this early, prevents the moving straight
		// to a write transaction.
		if _, err := s.checkCharmReferenceExists(ctx, tx, charmArgs.ReferenceName, charmArgs.Revision); err != nil {
			return internalerrors.Capture(err)
		}

		if err := s.setCharm(ctx, tx, id, charm, charmArgs.ArchivePath); err != nil {
			return internalerrors.Capture(err)
		}

		if err := s.setCharmHash(ctx, tx, id, charmArgs.Hash); err != nil {
			return internalerrors.Capture(err)
		}

		if err := s.setCharmInitialOrigin(ctx, tx, id, charmArgs.ReferenceName, charmArgs.Source, charmArgs.Revision, charmArgs.Version); err != nil {
			return internalerrors.Capture(err)
		}

		if err := s.setCharmPlatform(ctx, tx, id, charmArgs.Platform); err != nil {
			return internalerrors.Capture(err)
		}

		return nil
	}); err != nil {
		return "", internalerrors.Errorf("setting charm: %w", err)
	}

	return id, nil
}

// ListCharmsWithOrigin returns a list of charms with the specified origin.
// We require the origin, so we can reconstruct the charm URL for the
// client response.
func (s *CharmState) ListCharmsWithOrigin(ctx context.Context) ([]charm.CharmWithOrigin, error) {
	db, err := s.DB()
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	query := `
SELECT &charmNameWithOrigin.*
FROM v_charm_list_name_origin;
`
	stmt, err := s.Prepare(query, charmNameWithOrigin{})
	if err != nil {
		return nil, internalerrors.Errorf("preparing query: %w", err)
	}

	var results []charmNameWithOrigin
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).GetAll(&results); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return internalerrors.Errorf("getting charms with origin: %w", err)
		}
		return nil
	}); err != nil {
		return nil, internalerrors.Errorf("listing charms with origin: %w", err)
	}

	return decodeListCharmsWithOrigin(results)
}

// ListCharmsWithOrigin returns a list of charms with the specified origin.
// We require the origin, so we can reconstruct the charm URL for the
// client response.
func (s *CharmState) ListCharmsWithOriginByNames(ctx context.Context, names []string) ([]charm.CharmWithOrigin, error) {
	db, err := s.DB()
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	type nameSelector []string

	query := `
SELECT &charmNameWithOrigin.*
FROM v_charm_list_name_origin
WHERE name IN ($nameSelector[:]);
`
	stmt, err := s.Prepare(query, charmNameWithOrigin{}, nameSelector(names))
	if err != nil {
		return nil, internalerrors.Errorf("preparing query: %w", err)
	}

	var results []charmNameWithOrigin
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, nameSelector(names)).GetAll(&results); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return internalerrors.Errorf("getting charms with origin: %w", err)
		}
		return nil
	}); err != nil {
		return nil, internalerrors.Errorf("listing charms with origin: %w", err)
	}

	return decodeListCharmsWithOrigin(results)
}

func decodeListCharmsWithOrigin(results []charmNameWithOrigin) ([]charm.CharmWithOrigin, error) {
	charmsWithOrigin := make([]charm.CharmWithOrigin, len(results))
	for i, c := range results {

		source, err := decodeOriginSource(c.Source)
		if err != nil {
			return nil, internalerrors.Errorf("decoding charm origin source: %w", err)
		}

		architecture, err := decodeArchitecture(c.ArchitectureID)
		if err != nil {
			return nil, internalerrors.Errorf("decoding architecture: %w", err)
		}

		charmsWithOrigin[i] = charm.CharmWithOrigin{
			Name: c.Name,
			CharmOrigin: charm.CharmOrigin{
				ReferenceName: c.ReferenceName,
				Source:        source,
				Revision:      c.Revision,
				Platform: charm.Platform{
					Architecture: architecture,
				},
			},
		}
	}

	return charmsWithOrigin, nil
}

var tablesToDeleteFrom = []string{
	"charm_action",
	"charm_category",
	"charm_config",
	"charm_container_mount",
	"charm_container",
	"charm_device",
	"charm_extra_binding",
	"charm_hash",
	"charm_manifest_base",
	"charm_origin",
	"charm_payload",
	"charm_platform",
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
func (s *CharmState) DeleteCharm(ctx context.Context, id corecharm.ID) error {
	db, err := s.DB()
	if err != nil {
		return internalerrors.Capture(err)
	}

	selectQuery, err := s.Prepare(`SELECT charm.uuid AS &charmID.* FROM charm WHERE uuid = $charmID.uuid;`, charmID{})
	if err != nil {
		return internalerrors.Errorf("failed to prepare query: %w", err)
	}

	deleteQuery, err := s.Prepare(`DELETE FROM charm WHERE uuid = $charmID.uuid;`, charmID{})
	if err != nil {
		return internalerrors.Errorf("failed to prepare query: %w", err)
	}

	// Prepare the delete statements for each table.
	stmts := make([]deleteStatement, len(tablesToDeleteFrom))
	for i, table := range tablesToDeleteFrom {
		query := fmt.Sprintf("DELETE FROM %s WHERE charm_uuid = $charmUUID.charm_uuid;", table)

		stmt, err := s.Prepare(query, charmUUID{})
		if err != nil {
			return internalerrors.Errorf("failed to prepare query: %w", err)
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
				return internalerrors.Errorf("failed to delete related data for %q: %w", stmt.tableName, err)
			}
		}

		// Then delete the charm itself.
		if err := tx.Query(ctx, deleteQuery, charmID{UUID: id.String()}).Run(); err != nil {
			return internalerrors.Errorf("failed to delete charm: %w", err)
		}

		return nil
	}); err != nil {
		return internalerrors.Errorf("failed to delete charm: %w", err)
	}

	return nil
}

func (s *CharmState) setCharmHash(ctx context.Context, tx *sqlair.TX, id corecharm.ID, hash string) error {
	ident := charmID{UUID: id.String()}
	args := setCharmHash{
		CharmUUID:  ident.UUID,
		HashKindID: sha256HashKind,
		Hash:       hash,
	}

	query := `INSERT INTO charm_hash (*) VALUES ($setCharmHash.*);`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return internalerrors.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, args).Run(); err != nil {
		return internalerrors.Errorf("failed to insert charm hash: %w", err)
	}

	return nil
}

func (s *CharmState) setCharmInitialOrigin(
	ctx context.Context, tx *sqlair.TX, id corecharm.ID,
	referenceName string,
	source charm.CharmSource, revision int, version string) error {
	ident := charmID{UUID: id.String()}

	encodedOriginSource, err := encodeOriginSource(source)
	if err != nil {
		return internalerrors.Errorf("failed to encode charm origin source: %w", err)
	}

	args := setInitialCharmOrigin{
		CharmUUID:     ident.UUID,
		ReferenceName: referenceName,
		SourceID:      encodedOriginSource,
		Revision:      revision,
		Version:       version,
	}

	query := `INSERT INTO charm_origin (*) VALUES ($setInitialCharmOrigin.*);`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return internalerrors.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, args).Run(); err != nil {
		return internalerrors.Errorf("failed to insert charm origin: %w", err)
	}

	return nil
}

func (s *CharmState) setCharmPlatform(ctx context.Context, tx *sqlair.TX, id corecharm.ID, platform charm.Platform) error {
	ident := charmID{UUID: id.String()}

	ostypeID, err := encodeOSType(platform.OSType)
	if err != nil {
		return internalerrors.Errorf("failed to encode os type: %w", err)
	}

	architectureID, err := encodeArchitecture(platform.Architecture)
	if err != nil {
		return internalerrors.Errorf("failed to encode architecture: %w", err)
	}

	args := charmPlatform{
		CharmUUID:      ident.UUID,
		OSTypeID:       ostypeID,
		ArchitectureID: architectureID,
		Channel:        platform.Channel,
	}

	query := `INSERT INTO charm_platform (*) VALUES ($charmPlatform.*);`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return internalerrors.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, args).Run(); err != nil {
		return internalerrors.Errorf("failed to insert charm platform: %w", err)
	}

	return nil
}

func encodeOSType(os charm.OSType) (int, error) {
	switch os {
	case charm.Ubuntu:
		return 0, nil
	default:
		return 0, internalerrors.Errorf("unsupported os type: %q", os)
	}
}

func encodeArchitecture(a charm.Architecture) (int, error) {
	switch a {
	case charm.AMD64:
		return 0, nil
	case charm.ARM64:
		return 1, nil
	case charm.PPC64EL:
		return 2, nil
	case charm.S390X:
		return 3, nil
	case charm.RISV64:
		return 4, nil
	default:
		return 0, internalerrors.Errorf("unsupported architecture: %q", a)
	}
}

func encodeOriginSource(source charm.CharmSource) (int, error) {
	switch source {
	case charm.LocalSource:
		return 0, nil
	case charm.CharmHubSource:
		return 1, nil
	default:
		return 0, internalerrors.Errorf("unsupported source type: %q", source)
	}
}

func decodeCharmOrigin(origin charmOrigin, platform charmPlatform) (charm.CharmOrigin, error) {
	source, err := decodeOriginSource(origin.Source)
	if err != nil {
		return charm.CharmOrigin{}, internalerrors.Errorf("failed to decode charm origin source: %w", err)
	}

	p, err := decodeCharmPlatform(platform)
	if err != nil {
		return charm.CharmOrigin{}, internalerrors.Errorf("failed to decode platform: %w", err)
	}

	return charm.CharmOrigin{
		ReferenceName: origin.ReferenceName,
		Source:        source,
		Revision:      origin.Revision,
		Platform:      p,
	}, nil
}

func decodeOriginSource(source string) (charm.CharmSource, error) {
	switch source {
	case "local":
		return charm.LocalSource, nil
	case "charmhub":
		return charm.CharmHubSource, nil
	default:
		return "", internalerrors.Errorf("unsupported source type: %s", source)
	}
}

func encodeCharmOriginSource(source charm.CharmSource) int {
	// This should have been validated multiple times at the service layer, so
	// we don't need to revalidate it here.

	// The default will be charmhub for now, as we need to pick something.
	switch source {
	case charm.LocalSource:
		return 0
	default:
		return 1
	}
}

func decodeCharmPlatform(platform charmPlatform) (charm.Platform, error) {
	osType, err := decodeOSType(platform.OSTypeID)
	if err != nil {
		return charm.Platform{}, internalerrors.Errorf("failed to decode os type: %w", err)
	}

	arch, err := decodeArchitecture(platform.ArchitectureID)
	if err != nil {
		return charm.Platform{}, internalerrors.Errorf("failed to decode architecture: %w", err)
	}

	return charm.Platform{
		Channel:      platform.Channel,
		OSType:       osType,
		Architecture: arch,
	}, nil
}
