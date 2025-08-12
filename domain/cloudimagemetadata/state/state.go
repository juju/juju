// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/cloudimagemetadata"
	cloudmetadataerrors "github.com/juju/juju/domain/cloudimagemetadata/errors"
	dberrors "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

const (
	// ExpirationDelay is the maximum time a metadata can live in the cache before being removed.
	ExpirationDelay = 5 * time.Minute
)

var (
	// architectureIDsByName maps architecture names to their corresponding IDs.
	//
	// It is a hardcoded version of architecture table in controller schema.
	architectureIDsByName = map[string]int{
		"amd64":   0,
		"arm64":   1,
		"ppc64el": 2,
		"s390x":   3,
		"riscv64": 4,
	}
)

// State encapsulates the state management, logging, and architecture data.
type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// SupportedArchitectures retrieves the set of supported architecture names.
func (s *State) SupportedArchitectures(context.Context) set.Strings {
	return set.NewStrings(slices.Collect(maps.Keys(architectureIDsByName))...)
}

// NewState creates a new State instance using the provided database transaction factory and logger.
func NewState(factory database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	state := &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
		clock:     clock,
	}
	return state
}

// getArchitectureID returns the ID for a given architecture name and a boolean indicating if the name was found.
func getArchitectureID(name string) (int, bool) {
	id, ok := architectureIDsByName[name]
	return id, ok
}

// SaveMetadata stores the provided list of cloud image metadata into the database.
//
// Returns any errors occurred during db transaction.
//
// It also fires a cleanup for old images, if any.
//
// [cloudimagemetadata.Metadata] are considered unique among a signature, composed of these fields:
//
//   - Stream
//   - Region
//   - Version
//   - Arch
//   - VirtType
//   - RootStorageType
//   - Source
//
// Above behaviors applies for duplicated inserted metadata:
//   - If a metadata has the same signature than an existing one in the database, the imageID will be
//     updated with the new value in the existing metadata
//   - If a several metadata have the same signature in the list, it will cause a unique constraint error from
//     the database. It is likely a programmatic error.
func (s *State) SaveMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) error {
	// Cleaning the database before insert/update is a reasonable way to remove outdated data without
	// requiring an housekeeping goroutine. Since a metadata data doesn't live long in the db and
	// it shouldn't be a lot of custom metadata, it shouldn't slow too much the insertion.
	// Expired metadata are ignored when retrieving metadata, so it doesn't have any impact to keep
	// some of them longer than their time to live.
	if err := s.tryCleanUpExpiredMetadata(ctx); err != nil {
		s.logger.Warningf(ctx, "cannot cleanup expired metadata: %s", err)
	}
	s.logger.Debugf(ctx, "saving %d metadata", len(metadata))

	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return InsertMetadata(ctx, db, metadata, s.clock.Now())
}

// InsertMetadata inserts or updates metadata for cloud images in the database.
func InsertMetadata(ctx context.Context, db domain.TxnRunner, metadata []cloudimagemetadata.Metadata, createdAt time.Time) error {
	// Prepare inputs
	values := make([]inputMetadata, 0, len(metadata))
	for _, m := range metadata {
		// Convert architecture name to a db id
		archId, ok := getArchitectureID(m.Arch)
		if !ok {
			// If we get this error, something went wrong in the service layer, which should have
			// validated it.
			return errors.Errorf("unknown architecture %q", m.Arch)
		}

		// Update creation time if not defined.
		if !m.CreationTime.IsZero() {
			createdAt = m.CreationTime
		}

		mUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Errorf("failed to generate metadata uuid: %w", err)
		}
		values = append(values, inputMetadata{
			UUID:            mUUID.String(),
			CreatedAt:       createdAt,
			Source:          m.Source,
			Stream:          m.Stream,
			Region:          m.Region,
			Version:         m.Version,
			VirtType:        m.VirtType,
			ArchitectureID:  archId,
			RootStorageType: m.RootStorageType,
			RootStorageSize: m.RootStorageSize,
			Priority:        m.Priority,
			ImageID:         m.ImageID,
		})
	}

	// Prepare statement to check if a metadata is in the DB, and retrieve the UUID
	checkExistMetadataStmt, err := sqlair.Prepare(`
SELECT &metadataUUID.uuid FROM cloud_image_metadata
WHERE stream = $inputMetadata.stream
AND region = $inputMetadata.region
AND version = $inputMetadata.version
AND architecture_id = $inputMetadata.architecture_id
AND virt_type = $inputMetadata.virt_type
AND root_storage_type = $inputMetadata.root_storage_type
AND source = $inputMetadata.source
`, metadataUUID{}, inputMetadata{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare a statement to update only the image from a specific UUID
	updateMetadataStmt, err := sqlair.Prepare(`
UPDATE cloud_image_metadata 
SET image_id = $inputMetadata.image_id
WHERE cloud_image_metadata.uuid = $inputMetadata.uuid;`, inputMetadata{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to insert a batch of image in db. In case of conflict on unique constraint,
	// it will update the image id instead
	insertMetadataStmt, err := sqlair.Prepare(`
INSERT INTO cloud_image_metadata (*)
VALUES ($inputMetadata.*)`, inputMetadata{})
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		toInsert := make([]inputMetadata, 0, len(values))
		for _, m := range values {
			var out metadataUUID
			err := tx.Query(ctx, checkExistMetadataStmt, m).Get(&out)
			if dberrors.IsErrNotFound(err) {
				// Not found => insert
				toInsert = append(toInsert, m)
			} else if err == nil {
				// found => Update
				m.UUID = out.UUID
				err = tx.Query(ctx, updateMetadataStmt, m).Run()
				if err != nil {
					return errors.Capture(err)
				}
			} else {
				return errors.Capture(err)
			}
		}

		// Insert images metadata
		if len(toInsert) == 0 {
			return nil
		}
		if err := tx.Query(ctx, insertMetadataStmt, toInsert).Run(); dberrors.IsErrConstraintUnique(err) {
			return cloudmetadataerrors.ImageMetadataAlreadyExists
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	}))

}

// DeleteMetadataWithImageID deletes all metadata associated with the given image ID from the database.
func (s *State) DeleteMetadataWithImageID(ctx context.Context, imageID string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	deleteMetadataStmt, err := sqlair.Prepare(`
DELETE FROM cloud_image_metadata 
WHERE image_id = $metadataImageID.image_id`, metadataImageID{})
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, deleteMetadataStmt, metadataImageID{imageID}).Run()
		return err
	}))

}

// FindMetadata retrieves cloud image metadata from the database based on specified filter criteria.
// It constructs a dynamic SQL query using the supplied criteria and executes it to fetch matching records.
// Returns [cloudmetadataerrors.NotFound] if none are found with this criteria.
func (s *State) FindMetadata(ctx context.Context, criteria cloudimagemetadata.MetadataFilter) ([]cloudimagemetadata.Metadata, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	filter := metadataFilter{
		Region:          criteria.Region,
		Versions:        criteria.Versions,
		Arches:          criteria.Arches,
		Stream:          criteria.Stream,
		VirtType:        criteria.VirtType,
		RootStorageType: criteria.RootStorageType,
	}

	// Injection of the expiration time
	expirationTime := ttl{
		ExpiresAt: s.clock.Now().Add(-ExpirationDelay),
	}
	customSource := inputMetadata{
		Source: cloudimagemetadata.CustomSource,
	}
	inputArgs := []any{expirationTime, customSource}

	// clauses will collect all required clauses to build the final WHERE ... AND ... clause
	clauses := []string{
		// Ignores expired metadata in case of non custom source.
		`source = $inputMetadata.source OR cloud_image_metadata.created_at >=  $ttl.expires_at`,
	}

	// declareEqualsClause is an helper function to add a sqlair clause with the format cloud_image_metadata.<field> = $metadataFilter.<field>,
	// only if the corresponding field isn't empty
	hasFilter := false
	declareEqualsClause := func(field, value string) {
		if value != "" {
			hasFilter = true
			clauses = append(clauses, fmt.Sprintf(`cloud_image_metadata.%s = $metadataFilter.%s`, field, field))
		}
	}
	declareEqualsClause("region", filter.Region)
	declareEqualsClause("stream", filter.Stream)
	declareEqualsClause("virt_type", filter.VirtType)
	declareEqualsClause("root_storage_type", filter.RootStorageType)

	if hasFilter {
		inputArgs = append(inputArgs, filter)
	}

	// Handle  IN clauses, which is are bit different than equals clauses.
	if len(filter.Versions) > 0 {
		clauses = append(clauses, `cloud_image_metadata.version IN ($versions[:])`)
		inputArgs = append(inputArgs, filter.Versions)
	}
	if len(filter.Arches) > 0 {
		clauses = append(clauses, `arch.architecture_name IN ($arches[:])`)
		inputArgs = append(inputArgs, filter.Arches)
	}

	findMetadataQuery := fmt.Sprintf(`
WITH arch(id, architecture_name) AS (
	SELECT id, name FROM architecture
)
SELECT &outputMetadata.* 
FROM cloud_image_metadata
JOIN arch ON cloud_image_metadata.architecture_id = arch.id
WHERE %s;`, strings.Join(clauses, ` AND `))

	findMetadataStmt, err := sqlair.Prepare(findMetadataQuery, append([]any{outputMetadata{}}, inputArgs...)...)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var metadata []outputMetadata
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, findMetadataStmt, inputArgs...).GetAll(&metadata)
		if dberrors.IsErrNotFound(err) {
			return cloudmetadataerrors.NotFound
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]cloudimagemetadata.Metadata, len(metadata))
	for i, m := range metadata {
		result[i] = cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          m.Stream,
				Region:          m.Region,
				Version:         m.Version,
				Arch:            m.ArchitectureName,
				VirtType:        m.VirtType,
				RootStorageType: m.RootStorageType,
				RootStorageSize: m.RootStorageSize,
				Source:          m.Source,
			},
			Priority:     m.Priority,
			ImageID:      m.ImageID,
			CreationTime: m.CreatedAt,
		}
	}
	return result, nil
}

// AllCloudImageMetadata retrieves all cloud image metadata from the database without applying any filter criteria.
// Returns a slice of [cloudimagemetadata.Metadata] and any error encountered during the retrieval process.
func (s *State) AllCloudImageMetadata(ctx context.Context) ([]cloudimagemetadata.Metadata, error) {
	result, err := s.FindMetadata(ctx, cloudimagemetadata.MetadataFilter{})
	if errors.Is(err, cloudmetadataerrors.NotFound) {
		return nil, nil
	}
	return result, errors.Capture(err)
}

// tryCleanUpExpiredMetadata removes metadata records from the cloud_image_metadata table that are older than [ExpirationDelay].
//
// Custom images doesn't expire.
func (s *State) tryCleanUpExpiredMetadata(ctx context.Context) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// Injection of the expiration time
	expirationTime := ttl{
		ExpiresAt: s.clock.Now().Add(-ExpirationDelay),
	}
	customSource := inputMetadata{
		Source: cloudimagemetadata.CustomSource,
	}

	deleteMetadataStmt, err := sqlair.Prepare(`
DELETE FROM cloud_image_metadata 
WHERE source != $inputMetadata.source AND  
created_at < $ttl.expires_at;`, expirationTime, customSource)
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, deleteMetadataStmt, expirationTime, customSource).Run()
		return err
	}))

}
