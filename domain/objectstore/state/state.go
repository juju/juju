// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coredatabase "github.com/juju/juju/core/database"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State implements the domain objectstore state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State instance.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
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

// StartDraining initiates the draining process for the object store.
func (s *State) StartDraining(ctx context.Context, uuid string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	phaseTypeID, err := encodePhaseTypeID(coreobjectstore.PhaseDraining)
	if err != nil {
		return errors.Errorf("encoding phase type id: %w", err)
	}

	fromBackendStmt, err := s.Prepare(`
SELECT b.uuid AS &backendUUID.uuid
FROM object_store_backend AS b
WHERE b.life_id = 1`, backendUUID{})
	if err != nil {
		return errors.Errorf("preparing active object store backend statement: %w", err)
	}

	toBackendStmt, err := s.Prepare(`
SELECT b.uuid AS &backendUUID.uuid
FROM object_store_backend AS b
WHERE b.life_id = 0`, backendUUID{})
	if err != nil {
		return errors.Errorf("preparing to object store backend statement: %w", err)
	}

	insertStmt, err := s.Prepare(`
INSERT INTO object_store_drain_info (uuid, phase_type_id, from_backend_uuid, to_backend_uuid)
VALUES ($dbSetPhaseInfo.*);
	`, dbSetPhaseInfo{})
	if err != nil {
		return errors.Errorf("preparing insert draining phase statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Find the active backend. We'll need to set this backend as dying
		// if the phase is set to draining.
		var fromBackend backendUUID
		err := tx.Query(ctx, fromBackendStmt).Get(&fromBackend)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("migrating from: %v", objectstoreerrors.ErrBackendNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		var toBackend backendUUID
		err = tx.Query(ctx, toBackendStmt).Get(&toBackend)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("migrating to: %v", objectstoreerrors.ErrBackendNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		args := dbSetPhaseInfo{
			UUID:            uuid,
			PhaseTypeID:     phaseTypeID,
			FromBackendUUID: fromBackend.UUID,
			ToBackendUUID:   toBackend.UUID,
		}

		err = tx.Query(ctx, insertStmt, args).Run()
		if database.IsErrConstraintUnique(err) {
			return objectstoreerrors.ErrDrainingAlreadyInProgress
		} else if err != nil {
			return errors.Errorf("inserting draining phase: %w", err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("setting draining phase: %w", err)
	}
	return nil
}

// SetDrainingPhase sets the phase of the object store to draining. Returns an
// error if it cannot find the active backend, the to backend, or if there is
// already an active draining phase.
func (s *State) SetDrainingPhase(ctx context.Context, uuid string, phase coreobjectstore.Phase) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	phaseTypeID, err := encodePhaseTypeID(phase)
	if err != nil {
		return errors.Errorf("encoding phase type id: %w", err)
	}

	insertStmt, err := s.Prepare(`
UPDATE object_store_drain_info
SET phase_type_id = $dbSetPhaseInfo.phase_type_id
WHERE uuid = $dbSetPhaseInfo.uuid;
	`, dbSetPhaseInfo{})
	if err != nil {
		return errors.Errorf("preparing insert draining phase statement: %w", err)
	}

	selectBackendStmt, err := s.Prepare(`
SELECT b.uuid AS &backendUUID.uuid
FROM object_store_backend AS b
WHERE b.life_id = 1`, backendUUID{})
	if err != nil {
		return errors.Errorf("preparing active object store backend statement: %w", err)
	}

	markAsDead, err := s.Prepare(`
UPDATE object_store_backend
SET life_id = 2
WHERE uuid = $backendUUID.uuid`, backendUUID{})
	if err != nil {
		return errors.Errorf("preparing update object store backend statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var fromBackend backendUUID
		err := tx.Query(ctx, selectBackendStmt).Get(&fromBackend)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("migrating from: %v", objectstoreerrors.ErrBackendNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		args := dbSetPhaseInfo{
			UUID:        uuid,
			PhaseTypeID: phaseTypeID,
		}

		err = tx.Query(ctx, insertStmt, args).Run()
		if database.IsErrConstraintUnique(err) {
			return objectstoreerrors.ErrDrainingAlreadyInProgress
		} else if err != nil {
			return errors.Errorf("inserting draining phase: %w", err)
		}

		if phase == coreobjectstore.PhaseCompleted {
			// If the phase is completed, then we need to mark the from backend
			// as dead.
			err := tx.Query(ctx, markAsDead, backendUUID{UUID: fromBackend.UUID}).Run()
			if err != nil {
				return errors.Errorf("marking from backend as dead: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("setting draining phase: %w", err)
	}
	return nil
}

// GetActiveDrainingInfo returns the current draining information of the object
// store. If there is no active draining phase, then this will return an error.
// This is used to determine if the object store is currently in a draining
// phase, and if so, which phase it is in, and the uuid of the draining
// information, which can be used to correlate with logs and other information
// about the draining process.
func (s *State) GetActiveDrainingInfo(ctx context.Context) (domainobjectstore.DrainingInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return domainobjectstore.DrainingInfo{}, errors.Capture(err)
	}
	stmt, err := s.Prepare(`
SELECT di.uuid AS &dbGetPhaseInfo.uuid,
       pt.type AS &dbGetPhaseInfo.phase,
       di.from_backend_uuid AS &dbGetPhaseInfo.from_backend_uuid,
       di.to_backend_uuid AS &dbGetPhaseInfo.to_backend_uuid
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
		UUID:            phaseInfo.UUID,
		Phase:           phaseInfo.Phase,
		FromBackendUUID: phaseInfo.FromBackendUUID,
		ToBackendUUID:   phaseInfo.ToBackendUUID,
	}, nil
}

// SetObjectStoreBackendToS3 sets the object store to use S3 with the provided
// credentials. This is used to update the object store information when the
// object store is set to use S3 as the backend.
func (s *State) SetObjectStoreBackendToS3(ctx context.Context, uuid string, credential domainobjectstore.S3Credentials) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	type s3Credentials struct {
		UUID         string `db:"object_store_backend_uuid"`
		Endpoint     string `db:"endpoint"`
		AccessKey    string `db:"static_key"`
		SecretKey    string `db:"static_secret"`
		SessionToken string `db:"session_token"`
	}
	type uuidIdent struct {
		UUID string `db:"uuid"`
	}

	s3Creds := s3Credentials{
		UUID:         uuid,
		Endpoint:     credential.Endpoint,
		AccessKey:    credential.AccessKey,
		SecretKey:    credential.SecretKey,
		SessionToken: credential.SessionToken,
	}
	backendUUID := uuidIdent{UUID: uuid}

	// Force the active backend to be marked as dying, this will ensure that
	// we can ensure that there is only one active backend at a time, and that
	// we can monitor the process of draining from on to another.

	setBackendDyingStmt, err := s.Prepare(`
UPDATE object_store_backend
SET life_id = 1
WHERE life_id = 0`)
	if err != nil {
		return errors.Errorf("preparing update object store backend statement: %w", err)
	}

	insertBackendInfoStmt, err := s.Prepare(`
INSERT INTO object_store_backend (uuid, life_id, type_id)
VALUES ($uuidIdent.uuid, 0, 1)`, backendUUID)
	if err != nil {
		return errors.Errorf("preparing insert object store backend statement: %w", err)
	}

	s3InsertStmt, err := s.Prepare(`
INSERT INTO object_store_backend_s3_credential (object_store_backend_uuid, endpoint, static_key, static_secret, session_token)
VALUES ($s3Credentials.*)
`, s3Creds)
	if err != nil {
		return errors.Errorf("preparing insert object store information statement: %w", err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, setBackendDyingStmt).Get(&outcome); err != nil {
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
		if err := tx.Query(ctx, insertBackendInfoStmt, backendUUID).Get(&outcome); err != nil {
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

// MarkObjectStoreBackendAsDrained marks the object store backend as drained.
// This is used to mark the object store backend as drained after the draining
// process has completed. If the s3 backend has been drained, then this will
// remove the credentials.
func (s *State) MarkObjectStoreBackendAsDrained(ctx context.Context, uuid string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	type backendType struct {
		UUID   string `db:"uuid"`
		TypeID int    `db:"type_id"`
	}

	backend := backendType{UUID: uuid}

	selectStmt, err := s.Prepare(`
SELECT type_id AS &backendType.type_id
FROM object_store_backend
WHERE uuid = $backendType.uuid`, backend)
	if err != nil {
		return errors.Errorf("preparing select object store backend statement: %w", err)
	}

	deleteStmt, err := s.Prepare(`
DELETE FROM object_store_backend_s3_credential
WHERE object_store_backend_uuid = $backendType.uuid`, backend)
	if err != nil {
		return errors.Errorf("preparing delete object store backend credentials statement: %w", err)
	}

	updateStmt, err := s.Prepare(`
UPDATE object_store_backend
SET life_id = 2
WHERE uuid = $backendType.uuid AND life_id = 1`, backend)
	if err != nil {
		return errors.Errorf("preparing update object store backend statement: %w", err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, selectStmt, backend).Get(&backend); errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrBackendNotFound
		} else if err != nil {
			return errors.Errorf("selecting object store backend: %w", err)
		}

		// If the backend is S3, then we want to remove the credentials for the
		// backend, as the draining process has completed, and we want to ensure
		// that the credentials are not left in the database.
		if backend.TypeID == 1 {
			if err := tx.Query(ctx, deleteStmt, backend).Run(); err != nil {
				return errors.Errorf("deleting object store backend credentials: %w", err)
			}
		}

		var outcome sqlair.Outcome
		if err := tx.Query(ctx, updateStmt, backend).Get(&outcome); err != nil {
			return errors.Errorf("updating object store backend: %w", err)
		}
		if rows, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("updating object store backend: %w", err)
		} else if rows == 0 {
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
