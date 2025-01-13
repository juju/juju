// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// DeleteApplicationResources deletes all resources associated with a given
// application ID. It checks that resources are not linked to a file store,
// image store, or unit before deletion.
// The method uses several SQL statements to prepare and execute the deletion
// process within a transaction. If related records are found in any store,
// deletion is halted and an error is returned, preventing any deletion which
// can led to inconsistent state due to foreign key constraints.
func (st *State) DeleteApplicationResources(
	ctx context.Context,
	applicationID application.ID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	type uuids []string
	appIdentity := resourceIdentity{ApplicationUUID: applicationID.String()}

	// SQL statement to list all resources for an application.
	listAppResourcesStmt, err := st.Prepare(`
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM application_resource 
WHERE application_uuid = $resourceIdentity.application_uuid`, appIdentity)
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to check there is no related resources in resource_file_store.
	noFileStoreStmt, err := st.Prepare(`
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM resource_file_store
WHERE resource_uuid IN ($uuids[:])`, resourceIdentity{}, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to check there is no related resources in resource_image_store.
	noImageStoreStmt, err := st.Prepare(`
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM resource_image_store
WHERE resource_uuid IN ($uuids[:])`, resourceIdentity{}, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to check there is no related resources in unit_resource.
	noUnitResourceStmt, err := st.Prepare(`
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM unit_resource
WHERE resource_uuid IN ($uuids[:])`, resourceIdentity{}, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to delete resources from resource_retrieved_by.
	deleteFromRetrievedByStmt, err := st.Prepare(`
DELETE FROM resource_retrieved_by
WHERE resource_uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to delete resources from application_resource.
	deleteFromApplicationResourceStmt, err := st.Prepare(`
DELETE FROM application_resource
WHERE resource_uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// SQL statement to delete resources from resource.
	deleteFromResourceStmt, err := st.Prepare(`
DELETE FROM resource
WHERE uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) (err error) {
		// list all resources for an application.
		var resources []resourceIdentity
		err = tx.Query(ctx, listAppResourcesStmt, appIdentity).GetAll(&resources)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		resUUIDs := make(uuids, 0, len(resources))
		for _, res := range resources {
			resUUIDs = append(resUUIDs, res.UUID)
		}

		checkLink := func(message string, stmt *sqlair.Statement) error {
			var resources []resourceIdentity
			err := tx.Query(ctx, stmt, resUUIDs).GetAll(&resources)
			switch {
			case errors.Is(err, sqlair.ErrNoRows): // Happy path
				return nil
			case err != nil:
				return err
			}
			return errors.Errorf("%s: %w", message, resourceerrors.CleanUpStateNotValid)
		}

		// check there are no related resources in resource_file_store.
		if err = checkLink("resource linked to file store data", noFileStoreStmt); err != nil {
			return errors.Capture(err)
		}

		// check there are no related resources in resource_image_store.
		if err = checkLink("resource linked to image store data", noImageStoreStmt); err != nil {
			return errors.Capture(err)
		}

		// check there are no related resources in unit_resource.
		if err = checkLink("resource linked to unit", noUnitResourceStmt); err != nil {
			return errors.Capture(err)
		}

		// delete resources from resource_retrieved_by.
		if err = tx.Query(ctx, deleteFromRetrievedByStmt, resUUIDs).Run(); err != nil {
			return errors.Capture(err)
		}

		safedelete := func(stmt *sqlair.Statement) error {
			var outcome sqlair.Outcome
			err = tx.Query(ctx, stmt, resUUIDs).Get(&outcome)
			if err != nil {
				return errors.Capture(err)
			}
			num, err := outcome.Result().RowsAffected()
			if err != nil {
				return errors.Capture(err)
			}
			if num != int64(len(resUUIDs)) {
				return errors.Errorf("expected %d rows to be deleted, got %d", len(resUUIDs), num)
			}
			return nil
		}

		// delete resources from application_resource.
		err = safedelete(deleteFromApplicationResourceStmt)
		if err != nil {
			return errors.Capture(err)
		}

		// delete resources from resource.
		return safedelete(deleteFromResourceStmt)
	})
}

// DeleteUnitResources removes the association of a unit, identified by UUID,
// with any of its' application's resources. It initiates a transaction and
// executes an SQL statement to delete rows from the unit_resource table.
// Returns an error if the operation fails at any point in the process.
func (st *State) DeleteUnitResources(
	ctx context.Context,
	uuid coreunit.UUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unit := unitResource{UnitUUID: uuid.String()}
	stmt, err := st.Prepare(`DELETE FROM unit_resource WHERE unit_uuid = $unitResource.unit_uuid`, unit)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Capture(tx.Query(ctx, stmt, unit).Run())
	})
}

// GetApplicationResourceID returns the ID of the application resource
// specified by natural key of application and resource name.
func (st *State) GetApplicationResourceID(
	ctx context.Context,
	args resource.GetApplicationResourceIDArgs,
) (coreresource.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	// Define the resource identity based on the provided application ID and
	// name.
	resource := resourceIdentity{
		ApplicationUUID: args.ApplicationID.String(),
		Name:            args.Name,
	}

	// Prepare the SQL statement to retrieve the resource UUID.
	stmt, err := st.Prepare(`
SELECT uuid as &resourceIdentity.uuid 
FROM v_application_resource
WHERE name = $resourceIdentity.name 
AND application_uuid = $resourceIdentity.application_uuid
`, resource)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Execute the SQL transaction.
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, resource).Get(&resource)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resourceerrors.ResourceNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return coreresource.UUID(resource.UUID), nil
}

// ListResources returns the list of resource for the given application.
func (st *State) ListResources(
	ctx context.Context,
	applicationID application.ID,
) (resource.ApplicationResources, error) {
	db, err := st.DB()
	if err != nil {
		return resource.ApplicationResources{}, errors.Capture(err)
	}

	// Prepare the application ID to query resources by application.
	appID := resourceIdentity{
		ApplicationUUID: applicationID.String(),
	}

	// Prepare the statement to get resources for the given application.
	getResourcesQuery := `
SELECT &resourceView.* 
FROM v_resource
WHERE application_uuid = $resourceIdentity.application_uuid`
	getResourcesStmt, err := st.Prepare(getResourcesQuery, appID, resourceView{})
	if err != nil {
		return resource.ApplicationResources{}, errors.Capture(err)
	}

	// Prepare the statement to check if a resource has been polled.
	checkPolledQuery := `
SELECT &resourceIdentity.uuid 
FROM v_application_resource
WHERE application_uuid = $resourceIdentity.application_uuid
AND uuid = $resourceIdentity.uuid
AND last_polled IS NOT NULL`
	checkPolledStmt, err := st.Prepare(checkPolledQuery, appID)
	if err != nil {
		return resource.ApplicationResources{}, errors.Capture(err)
	}

	// Prepare the statement to get units related to a resource.
	getUnitsQuery := `
SELECT &unitResource.*
FROM unit_resource
WHERE unit_resource.resource_uuid = $resourceIdentity.uuid`
	getUnitStmt, err := st.Prepare(getUnitsQuery, appID, unitResource{})
	if err != nil {
		return resource.ApplicationResources{}, errors.Capture(err)
	}

	var result resource.ApplicationResources
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) (err error) {
		// Map to hold unit-specific resources
		resByUnit := map[coreunit.UUID]resource.UnitResources{}
		// resource found for the application
		var resources []resourceView

		// Query to get all resources for the given application.
		err = tx.Query(ctx, getResourcesStmt, appID).GetAll(&resources)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil // nothing found
		}
		if err != nil {
			return errors.Capture(err)
		}

		// Process each resource from the application to check polled state
		// and if they are associated with a unit.
		for _, res := range resources {
			resId := resourceIdentity{UUID: res.UUID, ApplicationUUID: res.ApplicationUUID}

			// Check to see if the resource has already been polled.
			err = tx.Query(ctx, checkPolledStmt, resId).Get(&resId)
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Capture(err)
			}
			hasBeenPolled := !errors.Is(err, sqlair.ErrNoRows)

			// Fetch units related to the resource.
			var units []unitResource
			err = tx.Query(ctx, getUnitStmt, resId).GetAll(&units)
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Capture(err)
			}

			r, err := res.toResource()
			if err != nil {
				return errors.Capture(err)
			}
			// Add each resource.
			result.Resources = append(result.Resources, r)

			// Add the charm resource or an empty one,
			// depending ons polled status.
			charmRes := charmresource.Resource{}
			if hasBeenPolled {
				charmRes, err = res.toCharmResource()
				if err != nil {
					return errors.Capture(err)
				}
			}
			result.RepositoryResources = append(result.RepositoryResources, charmRes)

			// Sort by unit to generate unit resources.
			for _, unit := range units {
				unitRes, ok := resByUnit[coreunit.UUID(unit.UnitUUID)]
				if !ok {
					unitRes = resource.UnitResources{ID: coreunit.UUID(unit.UnitUUID)}
				}
				ur, err := res.toResource()
				if err != nil {
					return errors.Capture(err)
				}
				unitRes.Resources = append(unitRes.Resources, ur)
				resByUnit[coreunit.UUID(unit.UnitUUID)] = unitRes
			}
		}
		// Collect and sort unit resources.
		units := slices.SortedFunc(maps.Values(resByUnit), func(r1, r2 resource.UnitResources) int {
			return strings.Compare(r1.ID.String(), r2.ID.String())
		})
		result.UnitResources = append(result.UnitResources, units...)

		return nil
	})

	// Return the list of application resources along with unit resources.
	return result, errors.Capture(err)
}

// GetResource returns the identified resource.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] if no such resource exists.
func (st *State) GetResource(ctx context.Context,
	resourceUUID coreresource.UUID) (resource.Resource, error) {
	db, err := st.DB()
	if err != nil {
		return resource.Resource{}, errors.Capture(err)
	}
	resourceParam := resourceIdentity{
		UUID: resourceUUID.String(),
	}
	resourceOutput := resourceView{}

	stmt, err := st.Prepare(`
SELECT &resourceView.*
FROM v_resource
WHERE uuid = $resourceIdentity.uuid`,
		resourceParam, resourceOutput)
	if err != nil {
		return resource.Resource{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, resourceParam).Get(&resourceOutput)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resourceerrors.ResourceNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return resource.Resource{}, errors.Capture(err)
	}

	return resourceOutput.toResource()
}

// RecordStoredResource records a stored resource along with who retrieved it.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.StoredResourceNotFound] if the stored resource at the
//     storageID cannot be found.
//   - [resourceerrors.ResourceAlreadyStored] if the resource is already
//     associated with a stored resource blob.
func (st *State) RecordStoredResource(
	ctx context.Context,
	args resource.RecordStoredResourceArgs,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		switch args.ResourceType {
		case charmresource.TypeFile:
			err = st.insertFileResource(ctx, tx, args.ResourceUUID, args.StorageID, args.Size, args.Fingerprint)
			if err != nil {
				return errors.Errorf("inserting stored file resource information: %w", err)
			}
		case charmresource.TypeContainerImage:
			err = st.insertImageResource(ctx, tx, args.ResourceUUID, args.StorageID)
			if err != nil {
				return errors.Errorf("inserting stored container image resource information: %w", err)
			}
		default:
			return errors.Errorf("unknown resource type: %q", args.ResourceType.String())
		}

		if args.RetrievedBy != "" {
			err := st.insertRetrievedBy(ctx, tx, args.ResourceUUID, args.RetrievedBy, args.RetrievedByType)
			if err != nil {
				return errors.Errorf("inserting retrieval by for resource %s: %w", args.ResourceUUID, err)
			}
		}

		if args.IncrementCharmModifiedVersion {
			err := st.incrementCharmModifiedVersion(ctx, tx, args.ResourceUUID)
			if err != nil {
				return errors.Errorf("incrementing charm modified version for application of resource %s: %w", args.ResourceUUID, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// GetResourceType finds the type of the given resource from the resource table.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] if the resource UUID cannot be
//     found.
func (st *State) GetResourceType(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (charmresource.Type, error) {
	db, err := st.DB()
	if err != nil {
		return 0, errors.Capture(err)
	}

	resKind := resourceKind{
		UUID: resourceUUID.String(),
	}
	getResourceType, err := st.Prepare(`
SELECT &resourceKind.kind_name 
FROM   v_resource
WHERE  uuid = $resourceKind.uuid
`, resKind)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, getResourceType, resKind).Get(&resKind)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resourceerrors.ResourceNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return 0, errors.Capture(err)
	}

	kind, err := charmresource.ParseType(resKind.Name)
	if err != nil {
		return 0, errors.Errorf("parsing resource kind: %w", err)
	}
	return kind, err
}

// insertFileResource checks that the storage ID corresponds to stored object
// store metadata and then records that the resource is stored at the provided
// storage ID.
func (st *State) insertFileResource(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	storageID coreresourcestore.ID,
	size int64,
	fingerprint charmresource.Fingerprint,
) error {
	// Get the object store UUID of the stored resource blob.
	uuid, err := storageID.ObjectStoreUUID()
	if err != nil {
		return errors.Errorf("cannot get object store UUID: %w", err)
	}

	// Check the resource blob is stored in the object store.
	storedResource := storedFileResource{
		ResourceUUID:    resourceUUID.String(),
		ObjectStoreUUID: uuid.String(),
		Size:            size,
		Fingerprint:     fingerprint.String(),
	}
	checkObjectStoreMetadataStmt, err := st.Prepare(`
SELECT uuid AS &storedFileResource.store_uuid
FROM   object_store_metadata
WHERE  uuid = $storedFileResource.store_uuid
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkObjectStoreMetadataStmt, storedResource).Get(&storedResource)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking object store for resource %s: %w", resourceUUID, resourceerrors.StoredResourceNotFound)
	} else if err != nil {
		return errors.Errorf("checking object store for resource %s: %w", resourceUUID, err)
	}

	// Check if the resource has already been stored.
	checkResourceFileStoreStmt, err := st.Prepare(`
SELECT &storedFileResource.*
FROM   resource_file_store
WHERE  resource_uuid = $storedFileResource.resource_uuid
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkResourceFileStoreStmt, storedResource).Get(&storedResource)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if resource %s already stored: %w", resourceUUID, err)
	} else if err == nil {
		// If a row was found, return that the resource is already stored.
		return resourceerrors.ResourceAlreadyStored
	}

	// Record where the resource is stored.
	insertStoredResourceStmt, err := st.Prepare(`
INSERT INTO resource_file_store (*)
VALUES      ($storedFileResource.*)
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, insertStoredResourceStmt, storedResource).Get(&outcome)
	if err != nil {
		return errors.Errorf("resource %s: %w", resourceUUID, err)
	}

	return nil
}

// insertImageResource checks that the storage ID corresponds to stored
// container image store metadata and then records that the resource is stored
// at the provided storage ID.
func (st *State) insertImageResource(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	storageID coreresourcestore.ID,
) error {
	// Get the container image metadata storage key.
	storageKey, err := storageID.ContainerImageMetadataStoreID()
	if err != nil {
		return errors.Errorf("cannot get container image metadata id: %w", err)
	}

	// Check the resource is stored in the container image metadata store.
	storedResource := storedContainerImageResource{
		ResourceUUID: resourceUUID.String(),
		StorageKey:   storageKey,
	}
	checkContainerImageStoreStmt, err := st.Prepare(`
SELECT storage_key AS &storedContainerImageResource.store_storage_key
FROM   resource_container_image_metadata_store
WHERE  storage_key = $storedContainerImageResource.store_storage_key
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkContainerImageStoreStmt, storedResource).Get(&storedResource)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking container image metadata store for resource %s: %w", resourceUUID, resourceerrors.StoredResourceNotFound)
	} else if err != nil {
		return errors.Errorf("checking container image metadata store for resource %s: %w", resourceUUID, err)
	}

	// Check if the resource has already been stored.
	checkResourceImageStoreStmt, err := st.Prepare(`
SELECT &storedContainerImageResource.*
FROM   resource_image_store
WHERE  resource_uuid = $storedContainerImageResource.resource_uuid
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkResourceImageStoreStmt, storedResource).Get(&storedResource)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if resource %s already stored: %w", resourceUUID, err)
	} else if err == nil {
		// If a row was found, return that the resource is already stored.
		return resourceerrors.ResourceAlreadyStored
	}

	// Record where the resource is stored.
	insertStoredResourceStmt, err := st.Prepare(`
INSERT INTO resource_image_store (*)
VALUES ($storedContainerImageResource.*)
`, storedResource)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, insertStoredResourceStmt, storedResource).Get(&outcome)
	if err != nil {
		return errors.Errorf("resource %s: %w", resourceUUID, err)
	}

	return nil
}

// insertRetrievedBy updates the retrieved by table to record who retrieved the currently stored resource.
// in the retrieved_by table, and if not, adds the given retrieved by name and
// type.
func (st *State) insertRetrievedBy(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	retrievedBy string,
	retrievedByType resource.RetrievedByType,
) error {
	// Verify if the resource has already been retrieved.
	resID := resourceIdentity{UUID: resourceUUID.String()}
	checkAlreadyRetrievedQuery := `
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM   resource_retrieved_by
WHERE  resource_uuid = $resourceIdentity.uuid`
	checkAlreadyRetrievedStmt, err := st.Prepare(checkAlreadyRetrievedQuery, resID)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkAlreadyRetrievedStmt, resID).Get(&resID)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	} else if err == nil {
		// If the resource has already been retrieved, the return an error.
		return resourceerrors.ResourceAlreadyStored
	}

	// Insert retrieved by.
	type setRetrievedBy struct {
		ResourceUUID    string `db:"resource_uuid"`
		RetrievedByType string `db:"retrieved_by_type"`
		Name            string `db:"name"`
	}
	retrievedByParam := setRetrievedBy{
		ResourceUUID:    resourceUUID.String(),
		Name:            retrievedBy,
		RetrievedByType: retrievedByType.String(),
	}
	insertRetrievedByStmt, err := st.Prepare(`
INSERT INTO resource_retrieved_by (resource_uuid, retrieved_by_type_id, name)
SELECT      $setRetrievedBy.resource_uuid, rrbt.id, $setRetrievedBy.name
FROM        resource_retrieved_by_type rrbt
WHERE       rrbt.name = $setRetrievedBy.retrieved_by_type
`, retrievedByParam)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, insertRetrievedByStmt, retrievedByParam).Get(&outcome)
	if err != nil {
		return errors.Capture(err)
	}

	rows, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	} else if rows != 1 {
		return errors.Errorf("updating charm modified version: expected 1 row affected, got %d", rows)
	}

	return nil
}

// incrementCharmModifiedVersion increments the charm modified version on the
// application associated with a resource.
func (st *State) incrementCharmModifiedVersion(ctx context.Context, tx *sqlair.TX, resourceUUID coreresource.UUID) error {
	resID := resourceIdentity{UUID: resourceUUID.String()}
	updateCharmModifiedVersionStmt, err := st.Prepare(`
UPDATE application
SET    charm_modified_version = IFNULL(charm_modified_version ,0) + 1
WHERE  uuid IN (
    SELECT application_uuid
    FROM   application_resource
    WHERE  resource_uuid = $resourceIdentity.uuid
)
`, resID)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, updateCharmModifiedVersionStmt, resID).Get(&outcome)
	if err != nil {
		return errors.Errorf("updating charm modified version: %w", err)
	}

	rows, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	} else if rows != 1 {
		return errors.Errorf("updating charm modified version: expected 1 row affected, got %d", rows)
	}

	return nil
}

// SetApplicationResource marks an existing resource as in use by a kubernetes
// application.
//
// Existing links between the application and resources with the same charm uuid
// and resource name as the resource being set are left in the table to be
// removed later on resource cleanup.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] is returned if the resource cannot be
//     found.
func (st *State) SetApplicationResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check if the unit/resource is not already there.
	k8sAppResource := kubernetesApplicationResource{
		ResourceUUID: resourceUUID.String(),
		AddedAt:      st.clock.Now(),
	}
	checkK8sAppResourceAlreadyExistsStmt, err := st.Prepare(`
SELECT &kubernetesApplicationResource.*
FROM   kubernetes_application_resource
WHERE  kubernetes_application_resource.resource_uuid = $kubernetesApplicationResource.resource_uuid
`, k8sAppResource)
	if err != nil {
		return errors.Capture(err)
	}

	checkResourceExistsStmt, err := st.Prepare(`
SELECT uuid AS &kubernetesApplicationResource.resource_uuid
FROM   resource
WHERE  uuid = $kubernetesApplicationResource.resource_uuid
`, k8sAppResource)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to insert a new link between unit and resource.
	insertK8sAppResourceStmt, err := st.Prepare(`
INSERT INTO kubernetes_application_resource (resource_uuid, added_at)
VALUES      ($kubernetesApplicationResource.*)
`, k8sAppResource)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check unit resource is not already inserted.
		err := tx.Query(ctx, checkK8sAppResourceAlreadyExistsStmt, k8sAppResource).Get(&k8sAppResource)
		if err == nil {
			// If the kubernetes application resource already exists, do nothing
			// and return.
			return nil
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		// Check resource exists.
		err = tx.Query(ctx, checkResourceExistsStmt, k8sAppResource).Get(&k8sAppResource)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("resource %s: %w", resourceUUID, resourceerrors.ResourceNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		// Update kubernetes application resource table.
		var outcome sqlair.Outcome
		err = tx.Query(ctx, insertK8sAppResourceStmt, k8sAppResource).Get(&outcome)
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})

	return err
}

// SetUnitResource links a unit and a resource. If the unit is already linked to
// a resource with the same charm uuid and resource name as the resource being
// set, this resource is unset from the unit.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.UnitNotFound] if the unit id doesn't belong to an
//     existing unit.
//   - [resourceerrors.ResourceNotFound] if the resource id doesn't belong
//     to an existing resource.
func (st *State) SetUnitResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	unitUUID coreunit.UUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check if the unit/resource link is already there.
	unitResourceInput := unitResource{
		ResourceUUID: resourceUUID.String(),
		UnitUUID:     unitUUID.String(),
		AddedAt:      st.clock.Now(),
	}
	checkUnitResourceStmt, err := st.Prepare(`
SELECT &unitResource.*
FROM   unit_resource 
WHERE  unit_resource.resource_uuid = $unitResource.resource_uuid 
AND    unit_resource.unit_uuid = $unitResource.unit_uuid`, unitResourceInput)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check if the unit already has a resource set for this charm resource.
	checkResourceExistsStmt, err := st.Prepare(`
SELECT uuid AS &unitResource.resource_uuid
FROM   resource
WHERE  uuid = $unitResource.resource_uuid
`, unitResourceInput)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check that UnitUUID is valid UUID.
	checkValidUnitStmt, err := st.Prepare(`
SELECT uuid AS &unitResource.unit_uuid 
FROM   unit 
WHERE  uuid = $unitResource.unit_uuid`, unitResourceInput)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to insert a new link between unit and resource.
	insertUnitResourceQuery := `
INSERT INTO unit_resource (unit_uuid, resource_uuid, added_at)
VALUES      ($unitResource.*)`
	insertUnitResourceStmt, err := st.Prepare(insertUnitResourceQuery, unitResourceInput)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check unit resource is not already inserted.
		err := tx.Query(ctx, checkUnitResourceStmt, unitResourceInput).Get(&unitResourceInput)
		if err == nil {
			// If the unit to resource link is already there, return.
			return nil
		} else if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		// Check resource exists.
		err = tx.Query(ctx, checkResourceExistsStmt, unitResourceInput).Get(&unitResourceInput)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("resource %s: %w", resourceUUID, resourceerrors.ResourceNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		// Check unit exists.
		err = tx.Query(ctx, checkValidUnitStmt, unitResourceInput).Get(&unitResourceInput)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %s: %w", unitUUID, resourceerrors.UnitNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		// Unset any existing resources with the same charm resource as the
		// resource being set in the unit resource table.
		err = st.unsetUnitResourcesWithSameCharmResource(ctx, tx, resourceUUID, unitUUID)
		if err != nil {
			return errors.Errorf(
				"removing previously set unit resources for resource %s: %w", resourceUUID, err,
			)
		}

		// Update unit resource table.
		err = tx.Query(ctx, insertUnitResourceStmt, unitResourceInput).Run()
		return errors.Capture(err)
	})

	return err
}

// unsetUnitResourcesForCharmResource removes all unit resources that use a
// charm resource.
func (st *State) unsetUnitResourcesWithSameCharmResource(
	ctx context.Context, tx *sqlair.TX, uuid coreresource.UUID, unitUUID coreunit.UUID) error {
	unitRes := unitResource{ResourceUUID: uuid.String(), UnitUUID: unitUUID.String()}

	// Check if there is a resource on the unit that is using the same charm
	// resource as the resource we are trying to set. This will be an old
	// application resource of the units' which needs to be unset.
	checkForResourcesStmt, err := st.Prepare(`
SELECT ur.resource_uuid AS &resourceUUID.uuid
FROM   unit_resource ur
JOIN   resource r ON ur.resource_uuid = r.uuid
WHERE  ur.unit_uuid = $unitResource.unit_uuid
AND    (r.charm_uuid, r.charm_resource_name) IN (
    SELECT charm_uuid, charm_resource_name
    FROM   resource 
    WHERE  uuid = $unitResource.resource_uuid
    AND    state_id = 0 -- Only check available resources, not potential.
)`, unitRes, resourceUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	// Check if the unit already had a resource set for this charm resource.
	var matchingUUIDs []resourceUUID
	err = tx.Query(ctx, checkForResourcesStmt, unitRes).GetAll(&matchingUUIDs)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Nothing to do.
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}

	// There should be at most one resource with a matching charm resource
	// entry for this unit. There must be 1 here because of there were none
	// we would have had ErrNoRows.
	if len(matchingUUIDs) != 1 {
		return errors.Errorf("unit already has the charm resource set more than once")
	}

	// Unset the old unit resource pointing to the charm resource.
	unsetResourceStmt, err := st.Prepare(`
DELETE FROM   unit_resource
WHERE         resource_uuid = $resourceUUID.uuid 
AND           unit_uuid = $unitResource.unit_uuid
`, unitRes, resourceUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, unsetResourceStmt, unitRes, matchingUUIDs[0]).Get(&outcome)
	if err != nil {
		return errors.Capture(err)
	}

	num, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	} else if num != int64(len(matchingUUIDs)) {
		return errors.Errorf("expected %d rows to be deleted, got %d", len(matchingUUIDs), num)
	}

	return nil
}

// SetRepositoryResources sets the "polled" resources for the
// application to the provided values. The current data for this
// application/resource combination will be overwritten.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationNotFound] if the application id doesn't belong
//     to a valid application.
func (st *State) SetRepositoryResources(
	ctx context.Context,
	config resource.SetRepositoryResourcesArgs,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check that the application exists.
	appNameAndID := applicationNameAndID{
		ApplicationID: config.ApplicationID,
	}
	getAppNameQuery := `
SELECT name as &applicationNameAndID.name 
FROM application 
WHERE uuid = $applicationNameAndID.uuid
`
	getAppNameStmt, err := st.Prepare(getAppNameQuery, appNameAndID)
	if err != nil {
		return errors.Capture(err)
	}

	type resourceNames []string
	// Prepare statement to get impacted resources UUID.
	fetchResIdentity := resourceIdentity{ApplicationUUID: config.ApplicationID.String()}
	fetchUUIDsQuery := `
SELECT uuid as &resourceIdentity.uuid
FROM v_application_resource
WHERE  application_uuid = $resourceIdentity.application_uuid
AND name IN ($resourceNames[:])
`
	fetchUUIDsStmt, err := st.Prepare(fetchUUIDsQuery, fetchResIdentity, resourceNames{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to update lastPolled value.
	type lastPolledResource struct {
		UUID       string    `db:"uuid"`
		LastPolled time.Time `db:"last_polled"`
	}
	updateLastPolledQuery := `
UPDATE resource 
SET last_polled=$lastPolledResource.last_polled
WHERE uuid = $lastPolledResource.uuid
`
	updateLastPolledStmt, err := st.Prepare(updateLastPolledQuery, lastPolledResource{})
	if err != nil {
		return errors.Capture(err)
	}

	names := make([]string, 0, len(config.Info))
	for _, info := range config.Info {
		names = append(names, info.Name)
	}
	var resIdentities []resourceIdentity
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check application exists.
		err := tx.Query(ctx, getAppNameStmt, appNameAndID).Get(&appNameAndID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resourceerrors.ApplicationNotFound
		}
		if err != nil {
			return errors.Capture(err)
		}

		// Fetch resources UUID.
		err = tx.Query(ctx, fetchUUIDsStmt, resourceNames(names), fetchResIdentity).GetAll(&resIdentities)
		if !errors.Is(err, sqlair.ErrNoRows) && err != nil {
			return errors.Capture(err)
		}

		if len(resIdentities) != len(names) {
			foundResources := set.NewStrings()
			for _, res := range resIdentities {
				foundResources.Add(res.Name)
			}
			st.logger.Errorf("Resource not found for application %s (%s), missing: %s",
				appNameAndID.Name, config.ApplicationID, set.NewStrings(names...).Difference(foundResources).Values())
		}

		// Update last polled resources.
		for _, res := range resIdentities {
			err := tx.Query(ctx, updateLastPolledStmt, lastPolledResource{
				UUID:       res.UUID,
				LastPolled: config.LastPolled,
			}).Run()
			if err != nil {
				return errors.Capture(err)
			}
		}

		return nil
	})
	return errors.Capture(err)
}
