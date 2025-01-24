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
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	coreresourcestore "github.com/juju/juju/core/resource/store"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	internaldatabase "github.com/juju/juju/internal/database"
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

		// delete resources from application_resource.
		err = st.safeDeleteResourceUUIDs(ctx, tx, deleteFromApplicationResourceStmt, resUUIDs)
		if err != nil {
			return errors.Capture(err)
		}

		// delete resources from resource.
		return st.safeDeleteResourceUUIDs(ctx, tx, deleteFromResourceStmt, resUUIDs)
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

// GetResourceUUIDByApplicationAndResourceName returns the ID of the application
// resource specified by natural key of application and resource name.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationNotFound] is returned if the application is
//     not found.
//   - [resourceerrors.ResourceNotFound] if no resource with name exists for
//     given application.
func (st *State) GetResourceUUIDByApplicationAndResourceName(
	ctx context.Context,
	appName string,
	resName string,
) (coreresource.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	// Define the resource identity based on the provided application ID and
	// name.
	names := resourceAndAppName{
		ApplicationName: appName,
		ResourceName:    resName,
	}
	uuid := localUUID{}

	// Prepare the SQL statement to retrieve the resource UUID.
	stmt, err := st.Prepare(`
SELECT r.uuid AS &localUUID.uuid
FROM   resource AS r
JOIN   application_resource ar ON r.uuid = ar.resource_uuid
JOIN   application a           ON ar.application_uuid = a.uuid
WHERE  r.charm_resource_name = $resourceAndAppName.resource_name 
AND    a.name = $resourceAndAppName.application_name
`, names, uuid)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Execute the SQL transaction.
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, names).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			if exists, err := st.checkApplicationNameExists(ctx, tx, appName); err != nil {
				return errors.Errorf("checking application with name %s exists: %w", appName, err)
			} else if !exists {
				return resourceerrors.ApplicationNotFound
			}
			return resourceerrors.ResourceNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return coreresource.UUID(uuid.UUID), nil
}

// ListResources returns the list of resource for the given application.
func (st *State) ListResources(
	ctx context.Context,
	applicationID application.ID,
) (coreresource.ApplicationResources, error) {
	db, err := st.DB()
	if err != nil {
		return coreresource.ApplicationResources{}, errors.Capture(err)
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
		return coreresource.ApplicationResources{}, errors.Capture(err)
	}

	// Prepare the statement to get units related to a resource.
	getResourceUnitsQuery := `
SELECT &unitResource.*	
FROM unit_resource
WHERE resource_uuid = $resourceIdentity.uuid`
	getUnitStmt, err := st.Prepare(getResourceUnitsQuery, appID, unitResource{})
	if err != nil {
		return coreresource.ApplicationResources{}, errors.Capture(err)
	}

	getApplicationUnitsStmt, err := st.Prepare(`
SELECT &unitUUIDAndName.*
FROM unit 
WHERE application_uuid = $resourceIdentity.application_uuid
	`, appID, unitUUIDAndName{})
	if err != nil {
		return coreresource.ApplicationResources{}, errors.Capture(err)
	}

	var result coreresource.ApplicationResources
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) (err error) {

		// Map to hold unit-specific resources
		resByUnit := map[coreunit.UUID]coreresource.UnitResources{}
		// Query all units for the given application
		var applicationUnits []unitUUIDAndName
		err = tx.Query(ctx, getApplicationUnitsStmt, appID).GetAll(&applicationUnits)
		// it is ok to not have any units.
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		for _, unit := range applicationUnits {
			resByUnit[coreunit.UUID(unit.UUID)] = coreresource.UnitResources{
				Name:      coreunit.Name(unit.Name),
				Resources: nil,
			}
		}

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

			if res.State == resource.StatePotential.String() {
				// Add the charm resource
				charmRes, err := res.toCharmResource()
				if err != nil {
					return errors.Capture(err)
				}
				result.RepositoryResources = append(result.RepositoryResources, charmRes)
				continue
			}

			resId := resourceIdentity{UUID: res.UUID, ApplicationUUID: res.ApplicationUUID}

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

			// Sort by unit to generate unit resources.
			for _, unit := range units {
				unitRes, ok := resByUnit[coreunit.UUID(unit.UnitUUID)]
				if !ok {
					return errors.Errorf("unexpected unit %q linked to resource %q.", unit.UnitUUID, unitRes.Name)
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
		units := slices.SortedFunc(maps.Values(resByUnit), func(r1, r2 coreresource.UnitResources) int {
			return strings.Compare(r1.Name.String(), r2.Name.String())
		})
		result.UnitResources = append(result.UnitResources, units...)
		return nil
	})

	// Return the list of application resources along with unit resources.
	return result, errors.Capture(err)
}

// GetResourcesByApplicationID returns the list of resource for the given application.
// Returns an error if the operation fails at any point in the process.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ApplicationNotFound] if the application ID is not an
//     existing one.
//
// If the application exists but doesn't have any resources, no error are
// returned, the result just contains an empty list.
func (st *State) GetResourcesByApplicationID(
	ctx context.Context,
	applicationID application.ID,
) ([]coreresource.Resource, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Prepare the application ID to query resources by application.
	appID := resourceIdentity{
		ApplicationUUID: applicationID.String(),
	}

	// Prepare the statement to get resources for the given application.
	getResourcesQuery := `
SELECT &resourceView.* 
FROM v_resource
WHERE application_uuid = $resourceIdentity.application_uuid
AND state = 'available'`
	getResourcesStmt, err := st.Prepare(getResourcesQuery, appID, resourceView{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var resources []resourceView
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) (err error) {
		// Query to get all resources for the given application.
		err = tx.Query(ctx, getResourcesStmt, appID).GetAll(&resources)
		if errors.Is(err, sqlair.ErrNoRows) {
			if exists, err := st.checkApplicationIDExists(ctx, tx, applicationID); err != nil {
				return errors.Errorf("checking if application with id %q exists: %w", applicationID, err)
			} else if !exists {
				return errors.Errorf("no application with id %q: %w", applicationID, resourceerrors.ApplicationNotFound)
			}
			return nil // nothing found
		}
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	// Convert each resourceView to a resource
	var result []coreresource.Resource
	for _, res := range resources {
		r, err := res.toResource()
		if err != nil {
			return nil, errors.Capture(err)
		}
		// Add each resource.
		result = append(result, r)
	}

	return result, errors.Capture(err)
}

// GetResource returns the identified resource.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] if no such resource exists.
func (st *State) GetResource(ctx context.Context,
	resourceUUID coreresource.UUID) (coreresource.Resource, error) {
	db, err := st.DB()
	if err != nil {
		return coreresource.Resource{}, errors.Capture(err)
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
		return coreresource.Resource{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, resourceParam).Get(&resourceOutput)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resourceerrors.ResourceNotFound
		}

		return errors.Capture(err)
	})
	if err != nil {
		return coreresource.Resource{}, errors.Capture(err)
	}

	return resourceOutput.toResource()
}

// RecordStoredResource records a stored resource along with who retrieved it.
//
// If recording a stored blob for a resource that already has a blob associated
// with it, this association is removed and the hash of this blob returned in
// droppedHash. If there was no blob associated, droppedHash is empty.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.StoredResourceNotFound] if the stored resource at the
//     storageID cannot be found.
func (st *State) RecordStoredResource(
	ctx context.Context,
	args resource.RecordStoredResourceArgs,
) (droppedHash string, err error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		switch args.ResourceType {
		case charmresource.TypeFile:
			droppedHash, err = st.replaceFileResource(ctx, tx, args.ResourceUUID, args.StorageID, args.Size, args.SHA384)
			if err != nil {
				return errors.Errorf("inserting stored file resource information: %w", err)
			}
		case charmresource.TypeContainerImage:
			droppedHash, err = st.replaceImageResource(ctx, tx, args.ResourceUUID, args.StorageID, args.Size, args.SHA384)
			if err != nil {
				return errors.Errorf("inserting stored container image resource information: %w", err)
			}
		default:
			return errors.Errorf("unknown resource type: %q", args.ResourceType.String())
		}

		err := st.updateOriginAndRevision(ctx, tx, args.ResourceUUID, args.Origin, args.Revision)
		if err != nil {
			return errors.Errorf("updating resource revision and origin: %w", err)
		}

		if args.RetrievedBy != "" {
			err := st.upsertRetrievedBy(ctx, tx, args.ResourceUUID, args.RetrievedBy, args.RetrievedByType)
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
		return "", err
	}
	return droppedHash, nil
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

	var resKind charmresource.Type
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var errQuery error
		resKind, errQuery = st.getResourceType(ctx, tx, resourceUUID)
		return errors.Capture(errQuery)
	})
	if err != nil {
		return 0, errors.Capture(err)
	}

	return resKind, nil
}

func (st *State) getResourceType(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
) (charmresource.Type, error) {
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

	err = tx.Query(ctx, getResourceType, resKind).Get(&resKind)
	if errors.Is(err, sqlair.ErrNoRows) {
		return 0, resourceerrors.ResourceNotFound
	} else if err != nil {
		return 0, errors.Capture(err)
	}

	kind, err := charmresource.ParseType(resKind.Name)
	if err != nil {
		return 0, errors.Errorf("parsing resource kind: %w", err)
	}
	return kind, nil
}

// replaceFileResource checks that the storage ID corresponds to stored object
// store metadata and then records that the resource is stored at the provided
// storage ID.
//
// If recording a stored file for a resource that already has a file associated
// with it, this association is removed and the hash of this file returned in
// droppedHash. If there was no file associated, droppedHash is empty.
func (st *State) replaceFileResource(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	storageID coreresourcestore.ID,
	size int64,
	sha384 string,
) (droppedHash string, err error) {
	// Get the object store UUID of the stored resource blob.
	uuid, err := storageID.ObjectStoreUUID()
	if err != nil {
		return "", errors.Errorf("cannot get object store UUID: %w", err)
	}

	// Check the resource blob is stored in the object store.
	storedResource := storedFileResource{
		ResourceUUID:    resourceUUID.String(),
		ObjectStoreUUID: uuid.String(),
		Size:            size,
		SHA384:          sha384,
	}
	checkObjectStoreMetadataStmt, err := st.Prepare(`
SELECT uuid AS &storedFileResource.store_uuid
FROM   object_store_metadata
WHERE  uuid = $storedFileResource.store_uuid
`, storedResource)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, checkObjectStoreMetadataStmt, storedResource).Get(&storedResource)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("checking object store for resource %s: %w", resourceUUID, resourceerrors.StoredResourceNotFound)
	} else if err != nil {
		return "", errors.Errorf("checking object store for resource %s: %w", resourceUUID, err)
	}

	// Check if there is already a stored file for this resource.
	var existingStoredResource storedFileResource
	checkResourceFileStoreStmt, err := st.Prepare(`
SELECT &storedFileResource.*
FROM   resource_file_store
WHERE  resource_uuid = $storedFileResource.resource_uuid
`, existingStoredResource)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, checkResourceFileStoreStmt, storedResource).Get(&existingStoredResource)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("checking if resource %s already stored: %w", resourceUUID, err)
	} else if err == nil {
		// If a row was found, then a file for that resource is
		// already stored, remove it and set its storeID as droppedHash.
		if existingStoredResource == storedResource {
			// Unless it is the same resource blob.
			return "", nil
		}

		removeExistingStoredResource, err := st.Prepare(`
DELETE FROM   resource_file_store
WHERE         store_uuid = $storedFileResource.store_uuid
`, existingStoredResource)
		if err != nil {
			return "", errors.Capture(err)
		}

		err = tx.Query(ctx, removeExistingStoredResource, existingStoredResource).Run()
		if err != nil {
			return "", errors.Errorf("delinking old file for resource: %w", err)
		}

		droppedHash = existingStoredResource.SHA384
	}

	// Record where the resource is stored.
	insertStoredResourceStmt, err := st.Prepare(`
INSERT INTO resource_file_store (*)
VALUES      ($storedFileResource.*)
`, storedResource)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, insertStoredResourceStmt, storedResource).Run()
	if err != nil {
		return "", errors.Errorf("resource %s: %w", resourceUUID, err)
	}

	return droppedHash, nil
}

// replaceImageResource checks that the storage ID corresponds to stored
// container image store metadata and then records that the resource is stored
// at the provided storage ID.
//
// If recording a stored image for a resource that already has an image
// associated with it, this association is removed and the hash of this image
// returned in droppedHash. If there was no image associated, droppedHash is
// empty.
func (st *State) replaceImageResource(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	storageID coreresourcestore.ID,
	size int64,
	hash string,
) (droppedHash string, err error) {
	// Get the container image metadata storage key.
	storageKey, err := storageID.ContainerImageMetadataStoreID()
	if err != nil {
		return "", errors.Errorf("cannot get container image metadata id: %w", err)
	}

	// Check the resource is stored in the container image metadata store.
	storedResource := storedContainerImageResource{
		ResourceUUID: resourceUUID.String(),
		StorageKey:   storageKey,
		Size:         size,
		Hash:         hash,
	}
	checkContainerImageStoreStmt, err := st.Prepare(`
SELECT storage_key AS &storedContainerImageResource.store_storage_key
FROM   resource_container_image_metadata_store
WHERE  storage_key = $storedContainerImageResource.store_storage_key
`, storedResource)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, checkContainerImageStoreStmt, storedResource).Get(&storedResource)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("checking container image metadata store for resource %s: %w", resourceUUID, resourceerrors.StoredResourceNotFound)
	} else if err != nil {
		return "", errors.Errorf("checking container image metadata store for resource %s: %w", resourceUUID, err)
	}

	// Check if there is already a stored container image for this resource.
	var existingStoredResource storedContainerImageResource
	checkResourceImageStoreStmt, err := st.Prepare(`
SELECT &storedContainerImageResource.*
FROM   resource_image_store
WHERE  resource_uuid = $storedContainerImageResource.resource_uuid
`, existingStoredResource)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, checkResourceImageStoreStmt, storedResource).Get(&existingStoredResource)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("checking if resource %s already stored: %w", resourceUUID, err)
	} else if err == nil {
		// If a row was found, then a container image for that resource is
		// already stored, remove it and set its storeID as droppedHash.
		if existingStoredResource == storedResource {
			// Unless it is the same resource blob.
			return "", nil
		}

		removeExistingStoredResource, err := st.Prepare(`
DELETE FROM   resource_image_store
WHERE         store_storage_key = $storedContainerImageResource.store_storage_key
`, existingStoredResource)
		if err != nil {
			return "", errors.Capture(err)
		}

		err = tx.Query(ctx, removeExistingStoredResource, existingStoredResource).Run()
		if err != nil {
			return "", errors.Errorf("delinking old file for resource: %w", err)
		}

		droppedHash = existingStoredResource.Hash
	}

	// Record where the resource is stored.
	insertStoredResourceStmt, err := st.Prepare(`
INSERT INTO resource_image_store (*)
VALUES ($storedContainerImageResource.*)
`, storedResource)
	if err != nil {
		return "", errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, insertStoredResourceStmt, storedResource).Get(&outcome)
	if err != nil {
		return "", errors.Errorf("resource %s: %w", resourceUUID, err)
	}

	return droppedHash, nil
}

// upsertRetrievedBy updates the retrieved by table to record who retrieved the
// currently stored resource in the retrieved_by table, and if not, adds the
// given retrieved by name and type. If there is already a "retrieved by" value
// set for the resource, it is replaced.
func (st *State) upsertRetrievedBy(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	retrievedBy string,
	retrievedByType coreresource.RetrievedByType,
) error {
	// Upsert retrieved by.
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
ON CONFLICT(resource_uuid) DO UPDATE SET retrieved_by_type_id=excluded.retrieved_by_type_id, name=excluded.name
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

// updateOriginAndRevision sets the resource origin and revision.
func (st *State) updateOriginAndRevision(
	ctx context.Context,
	tx *sqlair.TX,
	resourceUUID coreresource.UUID,
	origin charmresource.Origin,
	revision int,
) error {
	originAndRevision := resourceOriginAndRevision{
		UUID:     resourceUUID.String(),
		Origin:   origin.String(),
		Revision: revision,
	}
	updateOriginAndRevisionStmt, err := st.Prepare(`
UPDATE resource
SET    revision = $resourceOriginAndRevision.revision,
       origin_type_id = (
    SELECT id
    FROM resource_origin_type
    WHERE name = $resourceOriginAndRevision.origin_name
)
WHERE  uuid = $resourceOriginAndRevision.uuid
`, originAndRevision)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, updateOriginAndRevisionStmt, originAndRevision).Get(&outcome)
	if err != nil {
		return errors.Errorf("updating resource origin and revision: %w", err)
	}

	rows, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	} else if rows != 1 {
		return errors.Errorf("updating resource origin and revision: expected 1 row affected, got %d", rows)
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
SELECT ur.resource_uuid AS &localUUID.uuid
FROM   unit_resource ur
JOIN   resource r ON ur.resource_uuid = r.uuid
WHERE  ur.unit_uuid = $unitResource.unit_uuid
AND    (r.charm_uuid, r.charm_resource_name) IN (
    SELECT charm_uuid, charm_resource_name
    FROM   resource 
    WHERE  uuid = $unitResource.resource_uuid
    AND    state_id = 0 -- Only check available resources, not potential.
)`, unitRes, localUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	// Check if the unit already had a resource set for this charm resource.
	var matchingUUIDs []localUUID
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
WHERE         resource_uuid = $localUUID.uuid 
AND           unit_uuid = $unitResource.unit_uuid
`, unitRes, localUUID{})
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

// SetRepositoryResources updates the "potential" resources as the last
// revision from charm repository. The current data for this
// application/resource  combination with "potential" state will be overwritten.
// If the resource doesn't exist, a log is generated.
//
// "Potential" resources should be created at the creation of the application
// for repository charm, with undefined `revision` and `last_polled` fields.
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
SELECT &resourceIdentity.*
FROM v_application_resource
WHERE  application_uuid = $resourceIdentity.application_uuid
AND state = 'potential'
AND name IN ($resourceNames[:])
`
	fetchUUIDsStmt, err := st.Prepare(fetchUUIDsQuery, fetchResIdentity, resourceNames{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to update resources.
	type updatePotentialResource struct {
		UUID       string    `db:"uuid"`
		LastPolled time.Time `db:"last_polled"`
		Revision   int       `db:"revision"`
		CharmUUID  string    `db:"charm_uuid"`
	}
	updateLastPolledQuery := `
UPDATE resource 
SET last_polled=$updatePotentialResource.last_polled,
    revision=$updatePotentialResource.revision,
    charm_uuid=$updatePotentialResource.charm_uuid
WHERE uuid = $updatePotentialResource.uuid
`
	updateLastPolledStmt, err := st.Prepare(updateLastPolledQuery, updatePotentialResource{})
	if err != nil {
		return errors.Capture(err)
	}

	revisionByName := make(map[string]int, len(config.Info))
	names := make([]string, 0, len(config.Info))
	for _, info := range config.Info {
		names = append(names, info.Name)
		revisionByName[info.Name] = info.Revision
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
			st.logger.Errorf(context.TODO(), "Resource not found for application %s (%s), missing: %s",
				appNameAndID.Name, config.ApplicationID, set.NewStrings(names...).Difference(foundResources).Values())
		}

		// Update last polled resources.
		for _, res := range resIdentities {
			err := tx.Query(ctx, updateLastPolledStmt, updatePotentialResource{
				UUID:       res.UUID,
				CharmUUID:  config.CharmID.String(),
				LastPolled: config.LastPolled,
				Revision:   revisionByName[res.Name],
			}).Run()
			if err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	return errors.Capture(err)
}

// AddResourcesBeforeApplication adds the details of which resource
// revisions to use before the application exists in the model. The
// charm and resource metadata must exist.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.CharmResourceNotFound] if the charm or charm resource
//     do not exist.
func (st *State) AddResourcesBeforeApplication(
	ctx context.Context,
	args resource.AddResourcesBeforeApplicationArgs,
) ([]coreresource.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Prepare SQL statement to insert the resource.
	insertStmt, err := st.Prepare(`
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision, 
       origin_type_id, state_id, created_at)
SELECT $addPendingResource.uuid,
       $addPendingResource.charm_uuid,
       $addPendingResource.charm_resource_name,
       $addPendingResource.revision,
       rot.id,
       rs.id,
       $addPendingResource.created_at
FROM   resource_origin_type rot,
       resource_state rs
WHERE  rot.name = $addPendingResource.origin_type_name
AND    rs.name = $addPendingResource.state_name`, addPendingResource{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Prepare SQL statement to link resource with application name.
	linkStmt, err := st.Prepare(`
INSERT INTO pending_application_resource (application_name, resource_uuid)
VALUES ($linkResourceApplication.*)`, linkResourceApplication{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var resourceUUIDs []coreresource.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		charmUUID, err := st.getCharmUUID(ctx, tx, args.CharmLocator)
		if err != nil {
			return err
		}

		var resources []addPendingResource
		resources, resourceUUIDs, err = st.buildResourcesToAdd(charmUUID, args.ResourceDetails)
		if err != nil {
			return err
		}

		// Insert resources
		for _, res := range resources {
			// Insert the resource.
			err = tx.Query(ctx, insertStmt, res).Run()
			if internaldatabase.IsErrConstraintForeignKey(err) {
				return errors.New("charm or charm resource does not exist").Add(resourceerrors.CharmResourceNotFound)
			} else if err != nil {
				return errors.Errorf("inserting resource %q: %w", res.Name, err)
			}

			// Link the resource to the application name.
			if err = tx.Query(ctx, linkStmt, linkResourceApplication{
				ResourceUUID:    res.UUID,
				ApplicationName: args.ApplicationName,
			}).Run(); err != nil {
				return errors.Errorf(
					"linking resource %q to application %q: %w",
					res.Name, args.ApplicationName, err)
			}
		}
		return nil
	})
	return resourceUUIDs, errors.Capture(err)
}

// getCharmUUID returns the charm UUID based on the charmLocator.
func (st *State) getCharmUUID(ctx context.Context, tx *sqlair.TX, locator charm.CharmLocator) (string, error) {
	// charmLocator is used to get the UUID of a charm.
	type charmLocator struct {
		ReferenceName string `db:"reference_name"`
		Revision      int    `db:"revision"`
		Source        string `db:"source"`
	}
	charmLoc := charmLocator{
		ReferenceName: locator.Name,
		Revision:      locator.Revision,
		Source:        string(locator.Source),
	}
	var charmUUID localUUID

	locatorQuery := `
SELECT     v.uuid AS &localUUID.*
FROM       v_charm_locator AS v
LEFT JOIN  charm_source AS cs ON v.source_id = cs.id
WHERE      v.reference_name = $charmLocator.reference_name
AND        v.revision = $charmLocator.revision
AND        cs.name = $charmLocator.source;
	`
	locatorStmt, err := st.Prepare(locatorQuery, localUUID{}, charmLocator{})
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, locatorStmt, &charmLoc).Get(&charmUUID); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", resourceerrors.CharmResourceNotFound
		}
		return "", errors.Errorf("getting charm ID: %w", err)
	}

	return charmUUID.UUID, nil
}

// buildResourcesToAdd creates resources to add based on provided app and charm
// resources.
// Returns a slice of addPendingResource, a slice of the created resource
// UUIDs, and an error if any issues occur during creation.
func (st *State) buildResourcesToAdd(
	charmUUID string,
	appResources []resource.AddResourceDetails,
) ([]addPendingResource, []coreresource.UUID, error) {
	resources := make([]addPendingResource, len(appResources))
	result := make([]coreresource.UUID, len(appResources))
	now := st.clock.Now()
	for i, r := range appResources {
		uuid, err := coreresource.NewUUID()
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		result[i] = uuid
		resources[i] = addPendingResource{
			UUID:      uuid.String(),
			CharmUUID: charmUUID,
			Name:      r.Name,
			Revision:  r.Revision,
			Origin:    r.Origin.String(),
			State:     coreresource.StateAvailable.String(),
			CreatedAt: now,
		}
	}
	return resources, result, nil
}

// UpdateResourceRevisionAndDeletePriorVersion updates the revision of resource
// to a new version. Increments charm modified version for the application to
// trigger use of the new resource revision by the application. Any stored
// resource will be deleted from the DB. The droppedHash will be returned to aid
// in removing from the object store.
func (st *State) UpdateResourceRevisionAndDeletePriorVersion(
	ctx context.Context,
	args resource.UpdateResourceRevisionArgs,
	resourceType charmresource.Type,
) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var droppedHash string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Setup to delete the blob in the service if one exists.
		switch resourceType {
		case charmresource.TypeFile:
			droppedHash, err = st.deleteFileResource(ctx, tx, args.ResourceUUID)
			if err != nil && !errors.Is(err, resourceerrors.StoredResourceNotFound) {
				return errors.Errorf("deleting stored file resource information: %w", err)
			}
		case charmresource.TypeContainerImage:
			droppedHash, err = st.deleteImageResource(ctx, tx, args.ResourceUUID)
			if err != nil && !errors.Is(err, resourceerrors.StoredResourceNotFound) {
				return errors.Errorf("deleting stored image resource information: %w", err)
			}
		default:
			return errors.Errorf("unknown resource type: %q", resourceType.String())
		}

		err = st.updateOriginAndRevision(
			ctx,
			tx,
			args.ResourceUUID,
			charmresource.OriginStore,
			args.Revision)
		if err != nil {
			return errors.Errorf("updating resource revision and origin: %w", err)
		}
		err = st.incrementCharmModifiedVersion(ctx, tx, args.ResourceUUID)
		if err != nil {
			return errors.Errorf(
				"incrementing charm modified version for application of resource %s: %w",
				args.ResourceUUID, err)
		}
		return nil
	})
	return droppedHash, errors.Capture(err)
}

// deleteFileResource deletes the resource_file_store row for the given
// resource UUID and returns the hash from it.
func (st *State) deleteFileResource(
	ctx context.Context,
	tx *sqlair.TX,
	resUUID coreresource.UUID,
) (string, error) {
	uuidToDelete := localUUID{UUID: resUUID.String()}
	dropped := hash{}

	queryStoredHash, err := st.Prepare(`
SELECT &hash.*
FROM   resource_file_store
WHERE  resource_uuid = $localUUID.uuid
`, dropped, uuidToDelete)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, queryStoredHash, uuidToDelete).Get(&dropped)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", resourceerrors.StoredResourceNotFound
	} else if err != nil {
		return "", errors.Errorf("removing stored file resource %s: %w", resUUID, err)
	}

	removeExistingStoredResource, err := st.Prepare(`
DELETE FROM   resource_file_store
WHERE         resource_uuid = $localUUID.uuid
`, uuidToDelete)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, removeExistingStoredResource, uuidToDelete).Run()
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", resourceerrors.StoredResourceNotFound
	} else if err != nil {
		return "", errors.Errorf("removing stored file resource %s: %w", resUUID, err)
	}

	return dropped.Hash, nil
}

// deleteImageResource deletes the resource_image_store row for the given
// resource UUID and returns the hash from it.
func (st *State) deleteImageResource(
	ctx context.Context,
	tx *sqlair.TX,
	resUUID coreresource.UUID,
) (string, error) {
	uuidToDelete := localUUID{UUID: resUUID.String()}
	dropped := hash{}

	queryStoredHash, err := st.Prepare(`
SELECT sha384 AS &hash.*
FROM   resource_image_store
WHERE  resource_uuid = $localUUID.uuid
`, dropped, uuidToDelete)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, queryStoredHash, uuidToDelete).Get(&dropped)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", resourceerrors.StoredResourceNotFound
	} else if err != nil {
		return "", errors.Errorf("removing stored image resource %s: %w", resUUID, err)
	}

	removeExistingStoredResource, err := st.Prepare(`
DELETE FROM   resource_image_store
WHERE         resource_uuid = $localUUID.uuid
`, uuidToDelete)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, removeExistingStoredResource, uuidToDelete).Run()
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", resourceerrors.StoredResourceNotFound
	} else if err != nil {
		return "", errors.Errorf("removing stored image resource %s: %w", resUUID, err)
	}

	return dropped.Hash, nil
}

// DeleteResourcesAddedBeforeApplication removes all resources for the
// given resource UUIDs. These resource UUIDs must have been returned
// by AddResourcesBeforeApplication.
func (st *State) DeleteResourcesAddedBeforeApplication(ctx context.Context, resources []coreresource.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	resUUIDs := make(uuids, 0, len(resources))
	for _, res := range resources {
		resUUIDs = append(resUUIDs, res.String())
	}

	// SQL statement to delete resources from pending_application_resource.
	deleteFromPendingApplicationResourceStmt, err := st.Prepare(`
DELETE FROM pending_application_resource
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

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.safeDeleteResourceUUIDs(ctx, tx, deleteFromPendingApplicationResourceStmt, resUUIDs); err != nil {
			return errors.Capture(err)
		}
		return st.safeDeleteResourceUUIDs(ctx, tx, deleteFromResourceStmt, resUUIDs)
	})
	return errors.Capture(err)
}

func (st *State) safeDeleteResourceUUIDs(ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, resUUIDs uuids) error {
	var outcome sqlair.Outcome
	err := tx.Query(ctx, stmt, resUUIDs).Get(&outcome)
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

// ImportResources sets resources imported in migration. It first builds all the
// resources to insert from the arguments, then inserts them at the end so as to
// wait as long as possible before turning into a write transaction.
//
// The following error types can be expected to be returned:
//   - [resourceerrors.ResourceNotFound] if the resource metadata cannot be
//     found on the charm.
//   - [resourceerrors.ApplicationNotFound] if the application name of an
//     application resource cannot be found in the database.
//   - [resourceerrors.UnitNotFound] if the unit name of a unit resource cannot
//     be found in the database.
func (st *State) ImportResources(ctx context.Context, args resource.ImportResourcesArgs) error {
	if len(args) == 0 {
		return nil
	}

	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		typeIDs, err := st.getTypeIDs(ctx, tx)
		if err != nil {
			return errors.Errorf("getting type IDs from database: %w", err)
		}

		resourcesToSet := &resourcesToSet{}
		for _, arg := range args {
			toSet, err := st.getResourcesToSetForApplication(ctx, tx, typeIDs, arg)
			if err != nil {
				return errors.Errorf("setting resources for application %s: %w", arg.ApplicationName, err)
			}
			resourcesToSet.append(toSet)
		}

		err = st.insertResources(ctx, tx, resourcesToSet)
		if err != nil {
			return errors.Errorf("inserting resources: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// typeIDs holds metadata about the IDs used in the database for certain types.
type typeIDs struct {
	// originToID maps the names of resource origins to the integer used to
	// represent them in the database.
	originToID map[string]int
	// stateAvailableID is the ID of the available state in the database.
	stateAvailableID int
	// statePotential is the ID of the potential state in the database.
	statePotentialID int
}

// getTypeIDs fetches the metadata about the IDs for certain types.
func (st *State) getTypeIDs(ctx context.Context, tx *sqlair.TX) (typeIDs, error) {
	origins, err := st.getOriginIDs(ctx, tx)
	if err != nil {
		return typeIDs{}, errors.Errorf("getting origin ids: %w", err)
	}

	states, err := st.getStateIDs(ctx, tx)
	if err != nil {
		return typeIDs{}, errors.Errorf("getting state ids: %w", err)
	}

	stateAvailableID, ok := states[resource.StateAvailable]
	if !ok {
		return typeIDs{}, errors.Errorf("state %s not found in database", resource.StateAvailable)
	}

	statePotentialID, ok := states[resource.StatePotential]
	if !ok {
		return typeIDs{}, errors.Errorf("state %s not found in database", resource.StatePotential)
	}

	ids := typeIDs{
		originToID:       origins,
		stateAvailableID: stateAvailableID,
		statePotentialID: statePotentialID,
	}
	return ids, nil
}

// resourcesToSet holds the resource structures to insert into the database.
// This allows all resources to be inserted in one go at the end of a
// transaction.
type resourcesToSet struct {
	resources                      []setResource
	applicationResources           []applicationResource
	unitResources                  []unitResource
	kubernetesApplicationResources []kubernetesApplicationResource
}

// append appends all slices in the another resourcesToSet struct to this one.
func (toSet *resourcesToSet) append(otherToSet resourcesToSet) {
	toSet.resources = append(
		toSet.resources, otherToSet.resources...,
	)
	toSet.applicationResources = append(
		toSet.applicationResources, otherToSet.applicationResources...,
	)
	toSet.unitResources = append(
		toSet.unitResources, otherToSet.unitResources...,
	)
	toSet.kubernetesApplicationResources = append(
		toSet.kubernetesApplicationResources, otherToSet.kubernetesApplicationResources...,
	)
}

// getResourcesToSetForApplication gets all the resources to set for a
// particular application from the arguments, and checks that their charm
// resources exist.
func (st *State) getResourcesToSetForApplication(
	ctx context.Context,
	tx *sqlair.TX,
	typeIDs typeIDs,
	args resource.ImportResourcesArg,
) (toSet resourcesToSet, err error) {
	appID, charmID, err := st.getApplicationAndCharmUUID(ctx, tx, args.ApplicationName)
	if err != nil {
		return toSet, errors.Errorf("getting ID for application %q: %w", args.ApplicationName, err)
	}

	// Get resources to set.
	toSet, resourceNameToUUID, err := st.getResourcesToSet(
		ctx, tx, typeIDs, charmID, appID, args.Resources,
	)
	if err != nil {
		return toSet, errors.Capture(err)
	}

	// If the charm is not local, add a repository resource placeholders and
	// link them to the application. The placeholders are filled in by the charm
	// revision update worker which populates them with potential resource
	// upgrades. The charm revision update worker does not do anything for local charms, so these are not needed.
	if isLocal, err := st.isLocalCharm(ctx, tx, charmID); err != nil {
		return toSet, errors.Errorf("checking if charm %s is local: %w", charmID, err)
	} else if !isLocal {
		repoResources, repoAppResources, err := st.getPotentialResourcePlaceholdersToSet(typeIDs, charmID, appID, args.Resources)
		if err != nil {
			return toSet, errors.Capture(err)
		}
		// Append the repository resources to the regular resources.
		toSet.resources = append(toSet.resources, repoResources...)
		toSet.applicationResources = append(toSet.applicationResources, repoAppResources...)
	}

	// Get unit resources to set.
	unitResourcesToSet, err := st.getUnitResourcesToSet(
		ctx, tx, args.UnitResources, resourceNameToUUID,
	)
	if err != nil {
		return toSet, errors.Errorf("getting unit resources for application %q: %w", args.ApplicationName, err)
	}
	toSet.unitResources = append(toSet.unitResources, unitResourcesToSet...)

	return toSet, nil
}

// getResourcesToSet gets the resources, application resource, and kubernetes
// resources to set for the given arguments, checking that the charm resources
// exist for each one.
func (st *State) getResourcesToSet(
	ctx context.Context,
	tx *sqlair.TX,
	typeIDs typeIDs,
	charmID corecharm.ID,
	appID application.ID,
	resources []resource.ImportResourceInfo,
) (toSet resourcesToSet, resourceNameToUUID map[string]coreresource.UUID, err error) {
	resourceNameToUUID = make(map[string]coreresource.UUID)
	for _, res := range resources {
		// Check that the charm resource exists and get its kind before we
		// attempt to set it.
		kind, err := st.getCharmResourceKind(ctx, tx, charmID, res.Name)
		if err != nil {
			return toSet, nil, errors.Errorf("checking resource %s exists on charm: %w", res.Name, err)
		}

		// Add resource to set.
		resourceToSet, resourceUUID, err := st.getResourceToSet(typeIDs, charmID, res)
		if err != nil {
			return toSet, nil, errors.Capture(err)
		}
		toSet.resources = append(toSet.resources, resourceToSet)
		resourceNameToUUID[res.Name] = resourceUUID

		// Add application resource to set.
		toSet.applicationResources = append(toSet.applicationResources, applicationResource{
			ResourceUUID:    resourceUUID.String(),
			ApplicationUUID: appID.String(),
		})

		if kind != charmresource.TypeContainerImage {
			continue
		}
		// Add kubernetes application resource for container image resources.
		// Assume that the application is already using the container image.
		toSet.kubernetesApplicationResources = append(toSet.kubernetesApplicationResources, kubernetesApplicationResource{
			ResourceUUID: resourceUUID.String(),
			AddedAt:      res.Timestamp,
		})
	}
	return toSet, resourceNameToUUID, nil
}

func (st *State) getResourceToSet(typeIDs typeIDs, charmID corecharm.ID, res resource.ImportResourceInfo) (setResource, coreresource.UUID, error) {
	originID, ok := typeIDs.originToID[res.Origin.String()]
	if !ok {
		return setResource{}, "", errors.Errorf("origin %s not found in database: %w",
			res.Origin, resourceerrors.OriginNotValid)
	}
	resourceUUID, err := coreresource.NewUUID()
	if err != nil {
		return setResource{}, "", errors.Capture(err)
	}
	return setResource{
		UUID:         resourceUUID.String(),
		CharmUUID:    charmID.String(),
		Name:         res.Name,
		Revision:     &res.Revision,
		OriginTypeId: originID,
		StateID:      typeIDs.stateAvailableID,
		CreatedAt:    res.Timestamp,
	}, resourceUUID, nil
}

// getPotentialResourcePlaceholdersToSet returns a repository resource
// placeholder to set in the resources table. The resource will have state
// potential but will only act as a placeholder to be updated in the future, it
// does not contain revision data.
func (st *State) getPotentialResourcePlaceholdersToSet(
	typeIDs typeIDs,
	charmID corecharm.ID,
	appID application.ID,
	resources []resource.ImportResourceInfo,
) ([]setResource, []applicationResource, error) {
	// All repository resources have origin store.
	storeOriginID, ok := typeIDs.originToID[charmresource.OriginStore.String()]
	if !ok {
		return nil, nil, errors.Errorf("origin %s not found in database",
			charmresource.OriginStore)
	}
	var (
		repoResourcesToSet    []setResource
		repoAppResourcesToSet []applicationResource
	)
	for _, res := range resources {
		resourceUUID, err := coreresource.NewUUID()
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		repoResourcesToSet = append(repoResourcesToSet, setResource{
			UUID:         resourceUUID.String(),
			CharmUUID:    charmID.String(),
			Name:         res.Name,
			Revision:     nil,
			OriginTypeId: storeOriginID,
			StateID:      typeIDs.statePotentialID,
		})
		repoAppResourcesToSet = append(repoAppResourcesToSet, applicationResource{
			ResourceUUID:    resourceUUID.String(),
			ApplicationUUID: appID.String(),
		})
	}
	return repoResourcesToSet, repoAppResourcesToSet, nil
}

// getUnitResourcesToSet gets the unit resources to set for the given arguments,
// checking that the unit exists for each unit resources.
func (st *State) getUnitResourcesToSet(
	ctx context.Context,
	tx *sqlair.TX,
	unitResources []resource.ImportUnitResourceInfo,
	resourceNameToUUID map[string]coreresource.UUID,
) ([]unitResource, error) {
	var unitResourcesToSet []unitResource
	for _, res := range unitResources {
		unitUUID, err := st.getUnitUUID(ctx, tx, res.UnitName)
		if err != nil {
			return nil, errors.Errorf("getting uuid of unit %q: %w", res.UnitName, err)
		}

		resUUID, ok := resourceNameToUUID[res.ResourceName]
		if !ok {
			return nil, errors.Errorf("unit resource for unknown resource: %q", res.ResourceName)
		}

		unitResourcesToSet = append(unitResourcesToSet, unitResource{
			ResourceUUID: resUUID.String(),
			UnitUUID:     unitUUID.String(),
			AddedAt:      res.Timestamp,
		})
	}
	return unitResourcesToSet, nil
}

// insertResources inserts resources, application resources, kubernetes
// resources and unit resources using bulk inserts.
func (st *State) insertResources(
	ctx context.Context,
	tx *sqlair.TX,
	toSet *resourcesToSet,
) error {
	// Bulk insert the resources.
	if len(toSet.resources) > 0 {
		insertStmt, err := st.Prepare(`
INSERT INTO resource (*) VALUES ($setResource.*)
`, setResource{})
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, insertStmt, toSet.resources).Run()
		if err != nil {
			return errors.Errorf("inserting resources: %w", err)
		}
	}

	// Bulk insert the application-resource links.
	if len(toSet.applicationResources) > 0 {
		insertApplicationResourceStmt, err := st.Prepare(`
INSERT INTO application_resource (*) VALUES ($applicationResource.*)
`, applicationResource{})
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, insertApplicationResourceStmt, toSet.applicationResources).Run()
		if err != nil {
			return errors.Errorf("linking resources to applications: %w", err)
		}
	}

	// Bulk insert the kubernetes-application-resource links.
	if len(toSet.kubernetesApplicationResources) > 0 {
		insertK8sResourceStmt, err := st.Prepare(`
INSERT INTO kubernetes_application_resource (*) VALUES ($kubernetesApplicationResource.*)
`, kubernetesApplicationResource{})
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, insertK8sResourceStmt, toSet.kubernetesApplicationResources).Run()
		if err != nil {
			return errors.Errorf("linking resources to kubernetes applications: %w", err)
		}
	}

	// Bulk insert the unit-resource links.
	if len(toSet.unitResources) > 0 {
		insertUnitResourceStmt, err := st.Prepare(`
INSERT INTO unit_resource (*) VALUES ($unitResource.*)
`, unitResource{})
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, insertUnitResourceStmt, toSet.unitResources).Run()
		if err != nil {
			return errors.Errorf("linking resources to units: %w", err)
		}
	}

	return nil
}

// isLocalCharm returns true if the charm uuid belongs to a local charm.
func (st *State) isLocalCharm(
	ctx context.Context,
	tx *sqlair.TX,
	charmID corecharm.ID,
) (bool, error) {
	uuid := charmUUID{
		UUID: charmID.String(),
	}
	getCharmSourceStmt, err := st.Prepare(`
SELECT cs.name AS &charmUUID.uuid
FROM   charm c
JOIN   charm_source cs ON c.source_id = cs.id
WHERE  uuid = $charmUUID.uuid
AND    source_name = 'local'
`, uuid)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, getCharmSourceStmt, uuid).Get(&uuid)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// getCharmResourceKind fetches the kind of charm resource and returns
// [resourceerrors.ResourceNotFound] if it cannot be found.
func (st *State) getCharmResourceKind(
	ctx context.Context,
	tx *sqlair.TX,
	charmID corecharm.ID,
	resourceName string,
) (charmresource.Type, error) {
	charmRes := charmResource{
		CharmUUID:    charmID.String(),
		ResourceName: resourceName,
	}
	checkCharmResourceExistsStmt, err := st.Prepare(`
SELECT &charmResource.kind
FROM   v_charm_resource
WHERE  name       = $charmResource.name
AND    charm_uuid = $charmResource.charm_uuid
`, charmRes)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = tx.Query(ctx, checkCharmResourceExistsStmt, charmRes).Get(&charmRes)
	if errors.Is(err, sqlair.ErrNoRows) {
		return 0, resourceerrors.ResourceNotFound
	} else if err != nil {
		return 0, errors.Capture(err)
	}

	kind, err := charmresource.ParseType(charmRes.Kind)
	if err != nil {
		return 0, errors.Errorf("parsing resource type %q: %w", charmRes.Kind, err)
	}
	return kind, nil
}

// getAppplicationAndCharmUUID returns gets the application ID and charm UUID
// for the given application name, returning [resourcerrors.ApplicationNotFound]
// if it cannot be found.
func (st *State) getApplicationAndCharmUUID(
	ctx context.Context,
	tx *sqlair.TX,
	applicationName string,
) (application.ID, corecharm.ID, error) {
	app := getApplicationAndCharmID{Name: applicationName}
	queryApplicationStmt, err := st.Prepare(`
SELECT (charm_uuid, uuid) AS (&getApplicationAndCharmID.*)
FROM   application
WHERE  name = $getApplicationAndCharmID.name
`, app)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	err = tx.Query(ctx, queryApplicationStmt, app).Get(&app)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return "", "", errors.Capture(err)
		}
		return "", "", errors.Errorf("%w: %s", resourceerrors.ApplicationNotFound, applicationName)
	}
	return app.ApplicationID, app.CharmID, nil

}

// getOriginIDs returns the database IDs for the origin types.
func (st *State) getOriginIDs(ctx context.Context, tx *sqlair.TX) (map[string]int, error) {
	type origin struct {
		Name string `db:"name"`
		ID   int    `db:"id"`
	}

	selectOriginStmt, err := st.Prepare(`
SELECT &origin.*
FROM   resource_origin_type
`, origin{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var origins []origin
	err = tx.Query(ctx, selectOriginStmt).GetAll(&origins)
	if err != nil {
		return nil, errors.Capture(err)
	}

	m := make(map[string]int)
	for _, o := range origins {
		m[o.Name] = o.ID
	}
	return m, nil
}

// getStateIDs returns the database IDs for the state types.
func (st *State) getStateIDs(ctx context.Context, tx *sqlair.TX) (map[resource.StateType]int, error) {
	type state struct {
		Name resource.StateType `db:"name"`
		ID   int                `db:"id"`
	}

	selectStateStmt, err := st.Prepare(`
SELECT &state.*
FROM   resource_state
`, state{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var states []state
	err = tx.Query(ctx, selectStateStmt).GetAll(&states)
	if err != nil {
		return nil, errors.Capture(err)
	}

	m := make(map[resource.StateType]int)
	for _, s := range states {
		m[s.Name] = s.ID
	}
	return m, nil
}

// getUnitUUID gets the UUID of the unit with the given name, its returns
// [resourceerrors.UnitNotFound] if the unit cannot be found.
func (st *State) getUnitUUID(ctx context.Context, tx *sqlair.TX, name string) (coreunit.UUID, error) {
	unit := unitUUIDAndName{Name: name}
	getUnitStmt, err := st.Prepare(`
SELECT &unitUUIDAndName.uuid 
FROM   unit 
WHERE  name = $unitUUIDAndName.name
`, unit)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, getUnitStmt, unit).Get(&unit)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q: %w", name, resourceerrors.UnitNotFound)
	} else if err != nil {
		return "", errors.Errorf("querying unit %q: %w", name, err)
	}
	return coreunit.UUID(unit.UUID), nil
}

// checkApplicationIDExists checks if an application exists in the database by its UUID.
func (st *State) checkApplicationIDExists(ctx context.Context, tx *sqlair.TX, appID application.ID) (bool, error) {
	application := applicationNameAndID{ApplicationID: appID}
	checkApplicationExistsStmt, err := st.Prepare(`
SELECT &applicationNameAndID.*
FROM   application
WHERE  uuid = $applicationNameAndID.uuid
`, application)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkApplicationExistsStmt, application).Get(&application)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

func (st *State) checkApplicationNameExists(ctx context.Context, tx *sqlair.TX, name string) (bool, error) {
	id := applicationNameAndID{
		Name: name,
	}
	checkApplicationExistsStmt, err := st.Prepare(`
SELECT uuid AS &applicationNameAndID.uuid
FROM   application
WHERE  name = $applicationNameAndID.name
`, id)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkApplicationExistsStmt, id).Get(&id)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}
