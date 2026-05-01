// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	coredatabase "github.com/juju/juju/core/database"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/life"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

const (
	// defaultFileBackendUUID is the default uuid for the file backend.
	// Although, this is only used for testing, it's here to ensure that
	// we have a consistent uuid for all file based backends. This must not be
	// changed.
	defaultFileBackendUUID = "653813f9-2896-5332-8cbe-629a337a56a3"
)

// State implements the domain objectstore state.
type State struct {
	*domain.StateBase

	clock clock.Clock
}

// NewState returns a new State instance.
func NewState(factory coredatabase.TxnRunnerFactory, clock clock.Clock) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
	}
}

// GetMetadata returns the persistence metadata for the specified path.
func (s *State) GetMetadata(ctx context.Context, path string) (coreobjectstore.Metadata, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Capture(err)
	}

	metadata := dbMetadata{Path: path}

	stmt, err := s.Prepare(`
SELECT &dbMetadata.*
FROM v_object_store_metadata
WHERE path = $dbMetadata.path`, metadata)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Errorf("preparing select metadata statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, metadata).Get(&metadata)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return objectstoreerrors.ErrNotFound
			}
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Errorf("retrieving metadata %s: %w", path, err)
	}
	return decodeDbMetadata(metadata), nil
}

// GetMetadataBySHA256 returns the persistence metadata for the object
// with SHA256 starting with the provided prefix.
func (s *State) GetMetadataBySHA256(ctx context.Context, sha256 string) (coreobjectstore.Metadata, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Capture(err)
	}

	sha256Ident := sha256Ident{SHA256: sha256}
	var metadata dbMetadata

	stmt, err := s.Prepare(`
SELECT &dbMetadata.*
FROM v_object_store_metadata
WHERE sha_256 = $sha256Ident.sha_256`, metadata, sha256Ident)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Errorf("preparing select metadata statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sha256Ident).Get(&metadata)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return objectstoreerrors.ErrNotFound
			}
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Errorf("retrieving metadata with sha256 %s: %w", sha256, err)
	}

	return decodeDbMetadata(metadata), nil
}

// GetMetadataBySHA256Prefix returns the persistence metadata for the object
// with SHA256 starting with the provided prefix.
func (s *State) GetMetadataBySHA256Prefix(ctx context.Context, sha256 string) (coreobjectstore.Metadata, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Capture(err)
	}

	sha256IdentPrefix := sha256IdentPrefix{SHA256Prefix: sha256}
	var metadata dbMetadata

	stmt, err := s.Prepare(`
SELECT &dbMetadata.*
FROM v_object_store_metadata
WHERE sha_256 LIKE $sha256IdentPrefix.sha_256_prefix || '%'`, metadata, sha256IdentPrefix)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Errorf("preparing select metadata statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sha256IdentPrefix).Get(&metadata)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return objectstoreerrors.ErrNotFound
			}
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Errorf("retrieving metadata with sha256 %s: %w", sha256, err)
	}

	return decodeDbMetadata(metadata), nil
}

// ListMetadata returns the persistence metadata.
func (s *State) ListMetadata(ctx context.Context) ([]coreobjectstore.Metadata, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, err
	}

	stmt, err := s.Prepare(`
SELECT &dbMetadata.*
FROM v_object_store_metadata`, dbMetadata{})
	if err != nil {
		return nil, errors.Errorf("preparing select metadata statement: %w", err)
	}

	var metadata []dbMetadata
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&metadata)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving metadata: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(metadata, decodeDbMetadata), nil
}

// PutMetadata adds a new specified path for the persistence metadata.
func (s *State) PutMetadata(ctx context.Context, uuid string, metadata coreobjectstore.Metadata) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err = s.putMetadata(ctx, tx, uuid, metadata)
		return err
	})
	if err != nil {
		return "", errors.Errorf("adding path %s: %w", metadata.Path, err)
	}
	return uuid, nil
}

// GetControllerIDHints returns the controller ID hints for the specified
// SHA384. This is used to indicate which controller might have the object
// with the specified SHA384, which can be used for optimization in certain
// scenarios.
func (s *State) GetControllerIDHints(ctx context.Context, sha384 string) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type hint struct {
		SHA384           string `db:"sha_384"`
		ControllerIDHint string `db:"node_id"`
	}

	ctrlHint := hint{SHA384: sha384}

	stmt, err := s.Prepare(`
SELECT node_id AS &hint.node_id
FROM object_store_placement
JOIN object_store_metadata ON object_store_placement.uuid = object_store_metadata.uuid
WHERE object_store_metadata.sha_384 = $hint.sha_384`, ctrlHint)
	if err != nil {
		return nil, errors.Errorf("preparing select controller ID hint statement: %w", err)
	}

	var hintResult []hint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ctrlHint).GetAll(&hintResult)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("retrieving controller ID hint for sha384 %s: %w", sha384, err)
	}

	return transform.Slice(hintResult, func(h hint) string {
		return h.ControllerIDHint
	}), nil
}

// PutMetadataWithControllerIDHint adds a new specified path for the persistence
// metadata with a controller ID hint. This is used to route the request to the
// correct controller in a multi-controller environment.
func (s *State) PutMetadataWithControllerIDHint(
	ctx context.Context,
	uuid string,
	metadata coreobjectstore.Metadata,
	controllerIDHint string,
) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	type hint struct {
		UUID             string `db:"uuid"`
		ControllerIDHint string `db:"node_id"`
	}

	hintQuery, err := s.Prepare(`
INSERT INTO object_store_placement (uuid, node_id)
VALUES ($hint.*)
ON CONFLICT (uuid, node_id) DO NOTHING;
`, hint{})
	if err != nil {
		return "", errors.Errorf("preparing insert placement hint statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err = s.putMetadata(ctx, tx, uuid, metadata)
		if err != nil {
			return err
		}

		return tx.Query(ctx, hintQuery, hint{
			UUID:             uuid,
			ControllerIDHint: controllerIDHint,
		}).Run()
	})
	if err != nil {
		return "", errors.Errorf("adding path %s: %w", metadata.Path, err)
	}
	return uuid, nil
}

func (s *State) putMetadata(ctx context.Context, tx *sqlair.TX, uuid string, metadata coreobjectstore.Metadata) (string, error) {
	dbMetadata := dbMetadata{
		UUID:   uuid,
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Size:   metadata.Size,
	}

	dbMetadataPath := dbMetadataPath{
		UUID: uuid,
		Path: metadata.Path,
	}

	metadataStmt, err := s.Prepare(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES ($dbMetadata.*) 
ON CONFLICT (sha_256) DO NOTHING
ON CONFLICT (sha_384) DO NOTHING`, dbMetadata)
	if err != nil {
		return "", errors.Errorf("preparing insert metadata statement: %w", err)
	}

	pathStmt, err := s.Prepare(`
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES ($dbMetadataPath.*)`, dbMetadataPath)
	if err != nil {
		return "", errors.Errorf("preparing insert metadata path statement: %w", err)
	}

	metadataLookupStmt, err := s.Prepare(`
SELECT uuid AS &dbMetadataPath.metadata_uuid
FROM   object_store_metadata 
WHERE  (
	sha_384 = $dbMetadata.sha_384 OR
	sha_256 = $dbMetadata.sha_256
)
AND size = $dbMetadata.size`, dbMetadata, dbMetadataPath)
	if err != nil {
		return "", errors.Errorf("preparing select metadata statement: %w", err)
	}

	var outcome sqlair.Outcome
	if err := tx.Query(ctx, metadataStmt, dbMetadata).Get(&outcome); err != nil {
		return "", errors.Errorf("inserting metadata: %w", err)
	}

	metadataRows, err := outcome.Result().RowsAffected()
	if err != nil {
		return "", errors.Errorf("inserting metadata: %w", err)
	} else if metadataRows == 0 {
		// If the rows affected is 0, then the metadata already exists.
		// We need to get the uuid for the metadata, so that we can insert
		// the path based on that uuid.
		err := tx.Query(ctx, metadataLookupStmt, dbMetadata).Get(&dbMetadataPath)
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", objectstoreerrors.ErrHashAndSizeAlreadyExists
		} else if err != nil {
			return "", errors.Errorf("inserting metadata: %w", err)
		}
		// At this point we need to update the uuid that we'll
		// return back to be the one that was already in the db.
		pUUID, err := coreobjectstore.ParseUUID(dbMetadataPath.UUID)
		if err != nil {
			return "", errors.Errorf("parsing present uuid in metadata: %w", err)
		}
		uuid = pUUID.String()
	}

	err = tx.Query(ctx, pathStmt, dbMetadataPath).Get(&outcome)
	constraintErr := database.IsErrConstraintPrimaryKey(err)
	if constraintErr && metadataRows == 0 {
		return "", objectstoreerrors.ErrHashAndSizeAlreadyExists
	} else if constraintErr {
		return "", objectstoreerrors.ErrPathAlreadyExistsDifferentHash
	} else if err != nil {
		return "", errors.Errorf("inserting metadata path: %w", err)
	}
	if rows, err := outcome.Result().RowsAffected(); err != nil {
		return "", errors.Errorf("inserting metadata path: %w", err)
	} else if rows != 1 {
		return "", errors.Errorf("metadata path not inserted")
	}
	return uuid, nil
}

// AddControllerIDHint adds a controller ID hint for the specified SHA384. This
// is used to indicate that a controller might have the object with the
// specified SHA384, which can be used for optimization in certain scenarios.
// Returns an error if the metadata with the specified SHA384 does not exist.
func (s *State) AddControllerIDHint(ctx context.Context, sha384 string, controllerIDHint string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	type hint struct {
		UUID             string `db:"uuid"`
		ControllerIDHint string `db:"node_id"`
	}
	type selector struct {
		SHA384 string `db:"sha_384"`
		UUID   string `db:"uuid"`
	}

	arg := selector{SHA384: sha384}

	selectQuery, err := s.Prepare(`
SELECT uuid AS &selector.uuid
FROM object_store_metadata
WHERE sha_384 = $selector.sha_384`, arg)
	if err != nil {
		return errors.Errorf("preparing select metadata statement: %w", err)
	}

	insertQuery, err := s.Prepare(`
INSERT INTO object_store_placement (uuid, node_id)
VALUES ($hint.uuid, $hint.node_id)
ON CONFLICT (uuid, node_id) DO NOTHING;
`, hint{})
	if err != nil {
		return errors.Errorf("preparing insert placement hint statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var uuid selector
		if err := tx.Query(ctx, selectQuery, arg).Get(&uuid); errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrNotFound
		} else if err != nil {
			return errors.Errorf("selecting metadata: %w", err)
		}

		if err := tx.Query(ctx, insertQuery, hint{
			UUID:             uuid.UUID,
			ControllerIDHint: controllerIDHint,
		}).Run(); err != nil {
			return errors.Errorf("inserting placement hint: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("adding controller id hint for sha384 %s: %w", sha384, err)
	}
	return nil
}

// RemoveMetadata removes the specified key for the persistence path.
func (s *State) RemoveMetadata(ctx context.Context, path string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	dbMetadataPath := dbMetadataPath{
		Path: path,
	}

	metadataUUIDStmt, err := s.Prepare(`
SELECT &dbMetadataPath.metadata_uuid 
FROM object_store_metadata_path 
WHERE path = $dbMetadataPath.path`, dbMetadataPath)
	if err != nil {
		return errors.Errorf("preparing select metadata statement: %w", err)
	}
	pathStmt, err := s.Prepare(`
DELETE FROM object_store_metadata_path 
WHERE path = $dbMetadataPath.path`, dbMetadataPath)
	if err != nil {
		return errors.Errorf("preparing delete metadata path statement: %w", err)
	}

	type count struct {
		Count int `db:"count"`
	}

	countStmt, err := s.Prepare(`
SELECT COUNT(*) AS &count.count 
FROM object_store_metadata_path 
WHERE metadata_uuid = $dbMetadataPath.metadata_uuid`, dbMetadataPath, count{})
	if err != nil {
		return errors.Errorf("preparing count metadata paths statement: %w", err)
	}

	placementStmt, err := s.Prepare(`
DELETE FROM object_store_placement 
WHERE uuid = $dbMetadataPath.metadata_uuid`, dbMetadataPath)
	if err != nil {
		return errors.Errorf("preparing delete placement hints statement: %w", err)
	}

	metadataStmt, err := s.Prepare(`
DELETE FROM object_store_metadata 
WHERE uuid = $dbMetadataPath.metadata_uuid`, dbMetadataPath)
	if err != nil {
		return errors.Errorf("preparing delete metadata statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the metadata uuid, so we can delete the metadata if there
		// are no more paths associated with it.
		err := tx.Query(ctx, metadataUUIDStmt, dbMetadataPath).Get(&dbMetadataPath)
		if errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, pathStmt, dbMetadataPath).Run(); err != nil {
			return errors.Capture(err)
		}

		// Ensure that nothing else is using the same metadata before we delete
		// it and the placement hints.
		var counter count
		err = tx.Query(ctx, countStmt, dbMetadataPath).Get(&counter)
		if err != nil {
			return errors.Errorf("counting metadata paths: %w", err)
		} else if counter.Count > 0 {
			// There are still paths using the same metadata, so we don't want
			// to delete the metadata or the placement hints.
			return nil
		}

		if err := tx.Query(ctx, placementStmt, dbMetadataPath).Run(); err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, metadataStmt, dbMetadataPath).Run(); err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("removing path %s: %w", path, err)
	}
	return nil
}

// TransitionDrainingPhase atomically reads the current draining phase,
// validates the transition, and either starts a new drain or updates the
// existing phase. This eliminates the TOCTOU race that existed when reading
// and writing were in separate transactions.
//
// Note: this pushes transition validation logic into the state layer, which
// is not ideal from a layering perspective, but is necessary to prevent a
// TOCTOU race where two concurrent callers could read the same phase and
// both attempt to transition.
//
// Valid transitions:
//   - Unknown → Draining (starts a new drain)
//   - Draining → Error
//   - Draining → Completed
//   - Error or Completed → same terminal phase = no-op
//
// This method returns the following errors:
//   - [objectstoreerrors.ErrBackendNotFound]: if backends required for draining
//     cannot be found.
//   - [objectstoreerrors.ErrDrainingAlreadyInProgress]: if a drain is already
//     active and the target phase is Draining.
func (s *State) TransitionDrainingPhase(ctx context.Context, newDrainUUID string, phase coreobjectstore.Phase) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	newPhaseTypeID, err := encodePhaseTypeID(phase)
	if err != nil {
		return errors.Errorf("encoding phase type id: %w", err)
	}

	// Statement to read the current active drain info.
	selectStmt, err := s.Prepare(`
SELECT di.uuid AS &dbGetPhaseInfo.uuid,
       pt.type AS &dbGetPhaseInfo.phase,
       di.from_backend_uuid AS &dbGetPhaseInfo.from_backend_uuid,
       di.to_backend_uuid AS &dbGetPhaseInfo.active_backend_uuid
FROM object_store_drain_info AS di
JOIN object_store_drain_phase_type AS pt ON di.phase_type_id=pt.id
WHERE di.phase_type_id <= 1;
`, dbGetPhaseInfo{})
	if err != nil {
		return errors.Errorf("preparing select statement: %w", err)
	}

	// Statement to update an existing drain phase.
	updateStmt, err := s.Prepare(`
UPDATE object_store_drain_info
SET phase_type_id = $dbSetPhaseInfo.phase_type_id
WHERE uuid = $dbSetPhaseInfo.uuid;
`, dbSetPhaseInfo{})
	if err != nil {
		return errors.Errorf("preparing update statement: %w", err)
	}

	// Statements for starting a new drain (from Unknown → Draining).
	fromBackendStmt, err := s.Prepare(`
SELECT b.uuid AS &backendUUID.uuid
FROM object_store_backend AS b
WHERE b.life_id = 1`, backendUUID{})
	if err != nil {
		return errors.Errorf("preparing from backend statement: %w", err)
	}

	toBackendStmt, err := s.Prepare(`
SELECT b.uuid AS &backendUUID.uuid
FROM object_store_backend AS b
WHERE b.life_id = 0`, backendUUID{})
	if err != nil {
		return errors.Errorf("preparing to backend statement: %w", err)
	}

	insertStmt, err := s.Prepare(`
INSERT INTO object_store_drain_info (uuid, phase_type_id, from_backend_uuid, to_backend_uuid)
VALUES ($dbSetPhaseInfo.*);
`, dbSetPhaseInfo{})
	if err != nil {
		return errors.Errorf("preparing insert statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Step 1: Read the current active drain phase within this txn.
		var phaseInfo dbGetPhaseInfo
		err := tx.Query(ctx, selectStmt).Get(&phaseInfo)
		hasPhase := true
		if errors.Is(err, sqlair.ErrNoRows) {
			hasPhase = false
		} else if err != nil {
			return errors.Errorf("reading current drain phase: %w", err)
		}

		// Step 2: Validate the transition.
		current := coreobjectstore.PhaseUnknown
		if hasPhase {
			current = coreobjectstore.Phase(phaseInfo.Phase)
		}

		// Special case: trying to start a new drain while one is already
		// active returns a specific sentinel error.
		if phase.IsDraining() && hasPhase {
			return objectstoreerrors.ErrDrainingAlreadyInProgress
		}

		if _, err := current.TransitionTo(phase); errors.Is(err, coreobjectstore.ErrTerminalPhase) {
			// Already in a terminal state, no-op.
			return nil
		} else if err != nil {
			return errors.Errorf("invalid transition from %q to %q: %w", current, phase, err)
		}

		// Step 3: Apply the transition.
		if !phase.IsDraining() {
			// Transitioning an existing drain (e.g. Draining →
			// Error/Completed).
			args := dbSetPhaseInfo{
				UUID:        phaseInfo.UUID,
				PhaseTypeID: newPhaseTypeID,
			}
			if err := tx.Query(ctx, updateStmt, args).Run(); err != nil {
				return errors.Errorf("updating drain phase: %w", err)
			}
			return nil
		}

		// Starting a new drain: need from/to backends.
		var fromBackend backendUUID
		err = tx.Query(ctx, fromBackendStmt).Get(&fromBackend)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("migrating from: %w", objectstoreerrors.ErrBackendNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		var toBackend backendUUID
		err = tx.Query(ctx, toBackendStmt).Get(&toBackend)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("migrating to: %w", objectstoreerrors.ErrBackendNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		args := dbSetPhaseInfo{
			UUID:            newDrainUUID,
			PhaseTypeID:     newPhaseTypeID,
			FromBackendUUID: fromBackend.UUID,
			ToBackendUUID:   toBackend.UUID,
		}
		if err := tx.Query(ctx, insertStmt, args).Run(); err != nil {
			return errors.Errorf("inserting drain info: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("transitioning draining phase: %w", err)
	}
	return nil
}

// GetActiveDrainingInfo returns the current draining information of the object
// store. If there is no active draining phase, then this will return an error.
// This is used to determine if the object store is currently in a draining
// phase, and if so, which phase it is in, and the uuid of the draining
// information, which can be used to correlate with logs and other information
// about the draining process.
//
// This method returns the following errors:
//   - [objectstoreerrors.ErrDrainingPhaseNotFound]: if there is no active
//     draining phase.
func (s *State) GetActiveDrainingInfo(ctx context.Context) (domainobjectstore.DrainingInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return domainobjectstore.DrainingInfo{}, errors.Capture(err)
	}
	stmt, err := s.Prepare(`
SELECT di.uuid AS &dbGetPhaseInfo.uuid,
       pt.type AS &dbGetPhaseInfo.phase,
       di.from_backend_uuid AS &dbGetPhaseInfo.from_backend_uuid,
       di.to_backend_uuid AS &dbGetPhaseInfo.active_backend_uuid
FROM object_store_drain_info AS di
JOIN object_store_drain_phase_type AS pt ON di.phase_type_id=pt.id
WHERE di.phase_type_id <= 1;
`, dbGetPhaseInfo{})
	if err != nil {
		return domainobjectstore.DrainingInfo{}, errors.Errorf("preparing select draining phase statement: %w", err)
	}

	var phaseInfo dbGetPhaseInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&phaseInfo)
		if errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrDrainingPhaseNotFound
		} else if err != nil {
			return errors.Errorf("retrieving draining phase: %w", err)
		}
		return nil
	})
	if err != nil {
		return domainobjectstore.DrainingInfo{}, errors.Errorf("getting draining phase: %w", err)
	}

	return domainobjectstore.DrainingInfo{
		UUID:              phaseInfo.UUID,
		Phase:             phaseInfo.Phase,
		FromBackendUUID:   nullableOrNil(phaseInfo.FromBackendUUID),
		ActiveBackendUUID: phaseInfo.ActiveBackendUUID,
	}, nil
}

// GetObjectStoreBackend returns the current object store backend information.
// This is used to determine which backend the object store is currently using,
// and if it is using S3, then it will return the credentials for the S3
// backend.
//
// This method returns the following errors:
//   - [objectstoreerrors.ErrBackendNotFound]: if it cannot find the backend with
//     the specified uuid.
func (s *State) GetObjectStoreBackend(ctx context.Context, uuid string) (domainobjectstore.BackendInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return domainobjectstore.BackendInfo{}, errors.Capture(err)
	}

	type backendInfo struct {
		UUID      string           `db:"uuid"`
		TypeID    int              `db:"type_id"`
		TypeValue string           `db:"type"`
		LifeID    life.Life        `db:"life_id"`
		Endpoint  sql.Null[string] `db:"endpoint"`
		AccessKey sql.Null[string] `db:"static_key"`
		SecretKey sql.Null[string] `db:"static_secret"`
	}

	stmt, err := s.Prepare(`
SELECT &backendInfo.*
FROM object_store_backend
JOIN object_store_backend_type ON object_store_backend.type_id = object_store_backend_type.id
LEFT JOIN object_store_backend_s3_credential ON object_store_backend.uuid = object_store_backend_s3_credential.object_store_backend_uuid
WHERE uuid = $backendInfo.uuid`, backendInfo{})
	if err != nil {
		return domainobjectstore.BackendInfo{}, errors.Errorf("preparing select object store backend statement: %w", err)
	}

	var info backendInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, backendInfo{UUID: uuid}).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrBackendNotFound
		} else if err != nil {
			return errors.Errorf("retrieving object store backend: %w", err)
		}
		return nil
	})
	if err != nil {
		return domainobjectstore.BackendInfo{}, errors.Errorf("getting object store backend: %w", err)
	}

	return domainobjectstore.BackendInfo{
		UUID:            info.UUID,
		ObjectStoreType: info.TypeValue,
		LifeID:          info.LifeID,
		Endpoint:        nullableOrNil(info.Endpoint),
		AccessKey:       nullableOrNil(info.AccessKey),
		SecretKey:       nullableOrNil(info.SecretKey),
	}, nil
}

// GetActiveObjectStoreBackend returns the current active object store backend
// information. This is used to determine which backend the object store is
// currently using, and if it is using S3, then it will return the credentials
// for the S3 backend.
//
// This method returns the following errors:
//   - [objectstoreerrors.ErrBackendNotFound]: if it cannot find the active
//     backend.
func (s *State) GetActiveObjectStoreBackend(ctx context.Context) (domainobjectstore.BackendInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return domainobjectstore.BackendInfo{}, errors.Capture(err)
	}

	type backendInfo struct {
		UUID      string           `db:"uuid"`
		TypeID    int              `db:"type_id"`
		TypeValue string           `db:"type"`
		LifeID    life.Life        `db:"life_id"`
		Endpoint  sql.Null[string] `db:"endpoint"`
		AccessKey sql.Null[string] `db:"static_key"`
		SecretKey sql.Null[string] `db:"static_secret"`
	}

	stmt, err := s.Prepare(`
SELECT &backendInfo.*
FROM object_store_backend
JOIN object_store_backend_type ON object_store_backend.type_id = object_store_backend_type.id
LEFT JOIN object_store_backend_s3_credential ON object_store_backend.uuid = object_store_backend_s3_credential.object_store_backend_uuid
WHERE life_id = 0`, backendInfo{})
	if err != nil {
		return domainobjectstore.BackendInfo{}, errors.Errorf("preparing select active object store backend statement: %w", err)
	}

	var info backendInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrBackendNotFound
		} else if err != nil {
			return errors.Errorf("retrieving active object store backend: %w", err)
		}
		return nil
	})
	if err != nil {
		return domainobjectstore.BackendInfo{}, errors.Errorf("getting active object store backend: %w", err)
	}

	return domainobjectstore.BackendInfo{
		UUID:            info.UUID,
		ObjectStoreType: info.TypeValue,
		LifeID:          info.LifeID,
		Endpoint:        nullableOrNil(info.Endpoint),
		AccessKey:       nullableOrNil(info.AccessKey),
		SecretKey:       nullableOrNil(info.SecretKey),
	}, nil
}

// TransitionBackendToS3 sets the object store to use S3 with the provided
// credentials. This is used to update the object store information when the
// object store is set to use S3 as the backend.
//
// This method returns the following errors:
//   - [objectstoreerrors.ErrDrainingAlreadyInProgress]: if there is already an
//     active draining phase, as we don't want to update the backend information
//     while we're in the middle of a draining process.
//   - [objectstoreerrors.ErrBackendAlreadyExists]: if there is already a backend
//     with the specified uuid.
func (s *State) TransitionBackendToS3(ctx context.Context, uuid string, credential domainobjectstore.S3Credentials) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	type s3Credentials struct {
		UUID      string `db:"object_store_backend_uuid"`
		Endpoint  string `db:"endpoint"`
		AccessKey string `db:"static_key"`
		SecretKey string `db:"static_secret"`
	}
	type newBackend struct {
		UUID      string    `db:"uuid"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	type count struct {
		Count int `db:"count"`
	}

	s3Creds := s3Credentials{
		UUID:      uuid,
		Endpoint:  credential.Endpoint,
		AccessKey: credential.AccessKey,
		SecretKey: credential.SecretKey,
	}
	backend := newBackend{
		UUID:      uuid,
		UpdatedAt: s.clock.Now().UTC(),
	}

	// Force the active backend to be marked as dying, this will ensure that
	// we can ensure that there is only one active backend at a time, and that
	// we can monitor the process of draining from on to another.

	getPhaseInfoStmt, err := s.Prepare(`
SELECT COUNT(*) AS &count.count
FROM object_store_drain_info AS di
WHERE di.phase_type_id = 1`, count{})
	if err != nil {
		return errors.Errorf("preparing select draining phase statement: %w", err)
	}

	setBackendDyingStmt, err := s.Prepare(`
UPDATE object_store_backend
SET life_id = 1, updated_at = $newBackend.updated_at
WHERE life_id = 0`, backend)
	if err != nil {
		return errors.Errorf("preparing update object store backend statement: %w", err)
	}

	insertBackendInfoStmt, err := s.Prepare(`
INSERT INTO object_store_backend (uuid, life_id, type_id, updated_at)
VALUES ($newBackend.uuid, 0, 1, $newBackend.updated_at)`, backend)
	if err != nil {
		return errors.Errorf("preparing insert object store backend statement: %w", err)
	}

	s3InsertStmt, err := s.Prepare(`
INSERT INTO object_store_backend_s3_credential (
    object_store_backend_uuid, 
    endpoint, 
    static_key, 
    static_secret
)
VALUES ($s3Credentials.*)
`, s3Creds)
	if err != nil {
		return errors.Errorf("preparing insert object store information statement: %w", err)
	}
	// Check if there are any dying backends already, which indicates a
	// transition is already in progress. This prevents concurrent calls from
	// each creating a new active backend.
	getDyingBackendStmt, err := s.Prepare(`
SELECT COUNT(*) AS &count.count
FROM object_store_backend
WHERE life_id = 1`, count{})
	if err != nil {
		return errors.Errorf("preparing select dying backends statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Ensure that we're not currently in a draining phase, as we don't want
		// to update the backend information while we're in the middle of a
		// draining process.
		var phaseCount count
		if err := tx.Query(ctx, getPhaseInfoStmt).Get(&phaseCount); err != nil {
			return errors.Errorf("checking draining phase: %w", err)
		} else if phaseCount.Count > 0 {
			return objectstoreerrors.ErrDrainingAlreadyInProgress
		}

		// Ensure there are no dying backends, which would indicate a transition
		// is already in progress. This prevents concurrent calls from each
		// creating a new active backend.
		var dyingCount count
		if err := tx.Query(ctx, getDyingBackendStmt).Get(&dyingCount); err != nil {
			return errors.Errorf("checking dying backends: %w", err)
		} else if dyingCount.Count > 0 {
			return objectstoreerrors.ErrDrainingAlreadyInProgress
		}

		var outcome sqlair.Outcome
		if err := tx.Query(ctx, setBackendDyingStmt, backend).Get(&outcome); err != nil {
			return errors.Errorf("updating object store backend: %w", err)
		}
		if rows, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("updating object store backend: %w", err)
		} else if rows == 0 {
			// If there are no active backends, then we can't trust the state
			// of the object store, so we return an error.
			return errors.Errorf("no object store backend active")
		}

		// Now activate the new backend by inserting the new backend
		// information, and the credentials for the S3 backend.
		if err := tx.Query(ctx, insertBackendInfoStmt, backend).Get(&outcome); database.IsErrConstraintPrimaryKey(err) {
			return objectstoreerrors.ErrBackendAlreadyExists
		} else if err != nil {
			return errors.Errorf("inserting object store backend: %w", err)
		}
		if rows, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("inserting object store backend: %w", err)
		} else if rows == 0 {
			return errors.Errorf("activating s3 object store backend")
		}

		if err := tx.Query(ctx, s3InsertStmt, s3Creds).Run(); err != nil {
			return errors.Errorf("inserting object store information: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("setting object store information: %w", err)
	}
	return nil
}

// MarkObjectStoreBackendAsDrained atomically validates that the object store
// is in a draining phase, identifies the source backend from the active drain
// record, and marks it as drained. This eliminates the TOCTOU race that
// existed when the service read drain info in one transaction and then called
// this in another.
//
// Note: this pushes phase validation and backend lookup into the state layer
// to prevent a TOCTOU race where the drain record could be modified between
// reading and acting on it.
//
// This method returns the following errors:
//   - [objectstoreerrors.ErrDrainingPhaseNotFound]: if there is no active
//     draining phase.
//   - [objectstoreerrors.ErrBackendNotFound]: if the from-backend cannot be
//     found.
func (s *State) MarkObjectStoreBackendAsDrained(ctx context.Context) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// Read the active drain info to find the from_backend_uuid.
	drainStmt, err := s.Prepare(`
SELECT di.from_backend_uuid AS &dbGetPhaseInfo.from_backend_uuid,
       pt.type AS &dbGetPhaseInfo.phase
FROM object_store_drain_info AS di
JOIN object_store_drain_phase_type AS pt ON di.phase_type_id=pt.id
WHERE di.phase_type_id <= 1;
`, dbGetPhaseInfo{})
	if err != nil {
		return errors.Errorf("preparing drain info statement: %w", err)
	}

	type backendType struct {
		UUID   string    `db:"uuid"`
		TypeID int       `db:"type_id"`
		LifeID life.Life `db:"life_id"`
	}

	selectStmt, err := s.Prepare(`
SELECT type_id AS &backendType.type_id,
       life_id AS &backendType.life_id
FROM object_store_backend
WHERE uuid = $backendType.uuid`, backendType{})
	if err != nil {
		return errors.Errorf("preparing select backend statement: %w", err)
	}

	deleteStmt, err := s.Prepare(`
DELETE FROM object_store_backend_s3_credential
WHERE object_store_backend_uuid = $backendType.uuid`, backendType{})
	if err != nil {
		return errors.Errorf("preparing delete credentials statement: %w", err)
	}

	updateStmt, err := s.Prepare(`
UPDATE object_store_backend
SET life_id = 2
WHERE uuid = $backendType.uuid AND life_id = 1`, backendType{})
	if err != nil {
		return errors.Errorf("preparing update backend statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Step 1: Read active drain and validate phase.
		var drainInfo dbGetPhaseInfo
		if err := tx.Query(ctx, drainStmt).Get(&drainInfo); errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrDrainingPhaseNotFound
		} else if err != nil {
			return errors.Errorf("reading drain info: %w", err)
		}

		phase := coreobjectstore.Phase(drainInfo.Phase)
		if !phase.IsDraining() {
			return errors.Errorf("cannot mark backend as drained when phase is %q", phase)
		}

		if !drainInfo.FromBackendUUID.Valid {
			return errors.Errorf("invalid draining state: from backend uuid is nil")
		}

		// Step 2: Validate the backend exists.
		backend := backendType{UUID: drainInfo.FromBackendUUID.V}
		if err := tx.Query(ctx, selectStmt, backend).Get(&backend); errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrBackendNotFound
		} else if err != nil {
			return errors.Errorf("selecting backend: %w", err)
		}

		// Step 3: Delete S3 credentials if applicable.
		if backend.TypeID == 1 {
			if err := tx.Query(ctx, deleteStmt, backend).Run(); err != nil {
				return errors.Errorf("deleting backend credentials: %w", err)
			}
		}

		// Step 4: Mark the backend as dead.
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, updateStmt, backend).Get(&outcome); err != nil {
			return errors.Errorf("updating backend: %w", err)
		}
		if rows, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("checking rows affected: %w", err)
		} else if rows == 0 && backend.LifeID != life.Dead {
			return errors.Errorf("no draining backend found")
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("marking object store backend as drained: %w", err)
	}
	return nil
}

// InitialWatchStatement returns the initial watch statement for the
// persistence path.
func (s *State) InitialWatchStatement() (string, string) {
	return "object_store_metadata_path", "SELECT path FROM object_store_metadata_path"
}

// InitialWatchDrainingTable returns the table for the draining phase.
func (s *State) InitialWatchDrainingTable() string {
	return "object_store_drain_info"
}

// InitialWatchBackendTable returns the table for the object store backend.
func (s *State) InitialWatchBackendTable() (string, string) {
	return "object_store_backend", "SELECT uuid FROM object_store_backend WHERE life_id = 0"
}

func encodePhaseTypeID(phase coreobjectstore.Phase) (int, error) {
	switch phase {
	case coreobjectstore.PhaseUnknown:
		return 0, nil
	case coreobjectstore.PhaseDraining:
		return 1, nil
	case coreobjectstore.PhaseError:
		return 2, nil
	case coreobjectstore.PhaseCompleted:
		return 3, nil
	default:
		return -1, errors.Errorf("invalid phase %q", phase)
	}
}

func nullableOrNil[T any](s sql.Null[T]) *T {
	if s.Valid {
		return &s.V
	}
	return nil
}
