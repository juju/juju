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
			return errors.Errorf("%s: %w", message, resourceerrors.InvalidCleanUpState)
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

			// Add each resource.
			result.Resources = append(result.Resources, res.toResource())

			// Add the charm resource or an empty one,
			// depending ons polled status.
			charmRes := charmresource.Resource{}
			if hasBeenPolled {
				charmRes = res.toCharmResource()
			}
			result.RepositoryResources = append(result.RepositoryResources, charmRes)

			// Sort by unit to generate unit resources.
			for _, unit := range units {
				unitRes, ok := resByUnit[coreunit.UUID(unit.UnitUUID)]
				if !ok {
					unitRes = resource.UnitResources{ID: coreunit.UUID(unit.UnitUUID)}
				}
				unitRes.Resources = append(unitRes.Resources, res.toResource())
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
// Returns a [resourceerrors.ResourceNotFound] if no such resource exists.
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

	return resourceOutput.toResource(), nil
}

// SetResource adds the resource to blob storage and updates the metadata.
func (st *State) SetResource(
	ctx context.Context,
	config resource.SetResourceArgs,
) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetUnitResource sets the resource metadata for a specific unit.
// Returns [resourceerrors.UnitNotFound] if the unit id doesn't belong to an existing unit.
// Returns [resourceerrors.ResourceNotFound] if the resource id doesn't belong to an existing resource.
func (st *State) SetUnitResource(
	ctx context.Context,
	config resource.SetUnitResourceArgs,
) (resource.SetUnitResourceResult, error) {
	db, err := st.DB()
	if err != nil {
		return resource.SetUnitResourceResult{}, errors.Capture(err)
	}

	// Prepare statement to check if the unit/resource is not already there.
	unitResourceInput := unitResource{
		ResourceUUID: config.ResourceUUID.String(),
		UnitUUID:     config.UnitUUID.String(),
		AddedAt:      st.clock.Now(),
	}
	checkUnitResourceQuery := `
SELECT &unitResource.* FROM unit_resource 
WHERE unit_resource.resource_uuid = $unitResource.resource_uuid 
AND unit_resource.unit_uuid = $unitResource.unit_uuid`
	checkUnitResourceStmt, err := st.Prepare(checkUnitResourceQuery, unitResourceInput)
	if err != nil {
		return resource.SetUnitResourceResult{}, errors.Capture(err)
	}

	// Prepare statement to check that UnitUUID is valid UUID.
	unitUUID := unitNameAndUUID{UnitUUID: config.UnitUUID}
	checkValidUnitQuery := `
SELECT &unitNameAndUUID.uuid 
FROM unit 
WHERE uuid = $unitNameAndUUID.uuid`
	checkValidUnitStmt, err := st.Prepare(checkValidUnitQuery, unitUUID)
	if err != nil {
		return resource.SetUnitResourceResult{}, errors.Capture(err)
	}

	// Prepare statement to check that resourceID is valid UUID.
	resourceUUID := resourceIdentity{UUID: config.ResourceUUID.String()}
	checkValidResourceQuery := `
SELECT &resourceIdentity.uuid
FROM resource
WHERE uuid = $resourceIdentity.uuid`
	checkValidResourceStmt, err := st.Prepare(checkValidResourceQuery, resourceUUID)
	if err != nil {
		return resource.SetUnitResourceResult{}, errors.Capture(err)
	}

	// Prepare statement to verify if the application resource is already
	// retrieved.
	checkAlreadyRetrievedQuery := `
SELECT resource_uuid AS &resourceIdentity.uuid 
FROM resource_retrieved_by
WHERE resource_uuid = $resourceIdentity.uuid`
	checkAlreadyRetrievedStmt, err := st.Prepare(checkAlreadyRetrievedQuery, resourceUUID)
	if err != nil {
		return resource.SetUnitResourceResult{}, errors.Capture(err)
	}

	// Prepare statements to update retrieved data if not already retrieved.
	type retrievedByType struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	type retrievedBy struct {
		ResourceUUID      string `db:"resource_uuid"`
		RetrievedByTypeID int    `db:"retrieved_by_type_id"`
		Name              string `db:"name"`
	}
	retrievedTypeParam := retrievedByType{Name: string(config.RetrievedByType)}
	retrievedByParam := retrievedBy{ResourceUUID: config.ResourceUUID.String(), Name: config.RetrievedBy}
	getRetrievedTypeQuery := `
	SELECT &retrievedByType.* 
	FROM resource_retrieved_by_type 
	WHERE name = $retrievedByType.name`
	getRetrievedByTypeStmt, err := st.Prepare(getRetrievedTypeQuery, retrievedTypeParam)
	if err != nil {
		return resource.SetUnitResourceResult{}, errors.Capture(err)
	}
	insertRetrievedByQuery := `
INSERT INTO resource_retrieved_by (resource_uuid, retrieved_by_type_id, name)
VALUES ($retrievedBy.*)`
	insertRetrievedByStmt, err := st.Prepare(insertRetrievedByQuery, retrievedByParam)
	if err != nil {
		return resource.SetUnitResourceResult{}, errors.Capture(err)
	}

	// Prepare statement to insert a new link between unit and resource.
	insertUnitResourceQuery := `
INSERT INTO unit_resource (unit_uuid, resource_uuid, added_at)
VALUES ($unitResource.*)`
	insertUnitResourceStmt, err := st.Prepare(insertUnitResourceQuery, unitResourceInput)
	if err != nil {
		return resource.SetUnitResourceResult{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check unit resource is not already inserted.
		err := tx.Query(ctx, checkUnitResourceStmt, unitResourceInput).Get(&unitResourceInput)
		if err == nil {
			return nil // nothing to do
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		// Check resource and unit exists.
		err = tx.Query(ctx, checkValidResourceStmt, resourceUUID).Get(&resourceUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("resource %s: %w", resourceUUID.UUID, resourceerrors.ResourceNotFound)
		}
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, checkValidUnitStmt, unitUUID).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("resource %s: %w", unitUUID.UnitUUID, resourceerrors.UnitNotFound)
		}
		if err != nil {
			return errors.Capture(err)
		}

		// Verify if the application is already retrieved.
		err = tx.Query(ctx, checkAlreadyRetrievedStmt, resourceUUID).Get(&resourceUUID)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}

		// Update retrieved by if it is not retrieved.
		if errors.Is(err, sqlair.ErrNoRows) {
			err = tx.Query(ctx, getRetrievedByTypeStmt, retrievedTypeParam).Get(&retrievedTypeParam)
			if errors.Is(err, sqlair.ErrNoRows) {
				return resourceerrors.UnknownRetrievedByType
			}
			if err != nil {
				return errors.Capture(err)
			}

			// Insert retrieved by.
			retrievedByParam.RetrievedByTypeID = retrievedTypeParam.ID
			err = tx.Query(ctx, insertRetrievedByStmt, retrievedByParam).Run()
			if err != nil {
				return errors.Capture(err)
			}
		}

		// update unit resource table.
		err = tx.Query(ctx, insertUnitResourceStmt, unitResourceInput).Run()
		return errors.Capture(err)
	})

	return resource.SetUnitResourceResult{
		UUID:      coreresource.UUID(unitResourceInput.ResourceUUID),
		Timestamp: unitResourceInput.AddedAt,
	}, err
}

// OpenApplicationResource returns the metadata for a resource.
func (st *State) OpenApplicationResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// OpenUnitResource returns the metadata for a resource. A unit
// resource is created to track the given unit and which resource
// its using.
func (st *State) OpenUnitResource(
	ctx context.Context,
	resourceUUID coreresource.UUID,
	unitID coreunit.UUID,
) (resource.Resource, error) {
	return resource.Resource{}, nil
}

// SetRepositoryResources sets the "polled" resources for the
// application to the provided values. The current data for this
// application/resource combination will be overwritten.
// Returns [resourceerrors.ApplicationNotFound] if the application id doesn't belong to a valid application.
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
