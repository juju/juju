// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/application"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	apperrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

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
			return apperrors.ResourceNotFound
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
		for _, unitRes := range slices.SortedFunc(maps.Values(resByUnit), func(r1, r2 resource.UnitResources) int {
			return strings.Compare(r1.ID.String(), r2.ID.String())
		}) {
			result.UnitResources = append(result.UnitResources, unitRes)
		}
		return nil
	})

	// Return the list of application resources along with unit resources.
	return result, errors.Capture(err)
}

// GetResource returns the identified resource.
// Returns a [apperrors.ResourceNotFound] if no such resource exists.
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
			return apperrors.ResourceNotFound
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
	res resource.Resource,
	increment resource.IncrementCharmModifiedVersionType,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	resourceIdentity := resourceIdentity{
		UUID:            res.UUID.String(),
		ApplicationUUID: res.ApplicationID.String(),
		Name:            res.Name,
	}

	resourceOriginType := resourceOriginType{Name: res.Origin.String()}

	// If the revision is -1, set it to NULL in the database.
	var revision sql.Null[int64]
	if res.Revision != -1 {
		revision.V = int64(res.Revision)
		revision.Valid = true
	}

	setRes := setResource{
		UUID:              res.UUID.String(),
		CharmResourceName: res.Name,
		Revision:          revision,
		CreatedAt:         res.Timestamp,
		// TODO(aflynn): Set StateID to 0 for now until its purpose is better
		// understood.
		StateID: 0,
	}

	resourceRetrievedByType := resourceRetrievedByType{Name: string(res.RetrievedByType)}

	resourceRetrievedBy := resourceRetrievedBy{
		ResourceUUID: res.UUID.String(),
		Name:         res.RetrievedBy,
	}

	charmUUID := charmUUID{}
	getCharmUUIDStmt, err := st.Prepare(`
SELECT &charmUUID.charm_uuid
FROM   application a 
JOIN   charm c ON a.charm_uuid = c.uuid
WHERE  a.uuid = $resourceIdentity.application_uuid
`, charmUUID, resourceIdentity)
	if err != nil {
		return errors.Capture(err)
	}

	checkCharmResourceExistsStmt, err := st.Prepare(`
SELECT &resourceIdentity.name
FROM   charm c 
JOIN   charm_resource cr ON c.uuid = cr.charm_uuid
WHERE  c.uuid = $charmUUID.charm_uuid
AND    cr.name = $resourceIdentity.name
`, charmUUID, resourceIdentity)
	if err != nil {
		return errors.Capture(err)
	}

	getOriginIDStmt, err := st.Prepare(`
SELECT &resourceOriginType.id
FROM   resource_origin_type
WHERE  name = $resourceOriginType.name
`, resourceOriginType)
	if err != nil {
		return errors.Capture(err)
	}

	insertResourceStmt, err := st.Prepare(`
INSERT INTO resource (*)
VALUES               ($setResource.*)
`, setRes)
	if err != nil {
		return errors.Capture(err)
	}

	getRetrievedByIDStmt, err := st.Prepare(`
SELECT &resourceRetrievedByType.id
FROM resource_added_by_type 
WHERE name = $resourceRetrievedByType.name
`, resourceRetrievedByType)
	if err != nil {
		return errors.Capture(err)
	}

	insertRetrievedByStmt, err := st.Prepare(`
INSERT INTO resource_added_by (*)
VALUES      ($resourceRetrievedBy.*)
`, resourceRetrievedBy)
	if err != nil {
		return errors.Capture(err)
	}

	insertApplicationResourceStmt, err := st.Prepare(`
INSERT INTO application_resource (resource_uuid, application_uuid)
VALUES      ($resourceIdentity.uuid, $resourceIdentity.application_uuid)
`, resourceIdentity)
	if err != nil {
		return errors.Capture(err)
	}

	var incrementCharmModifiedVersionStmt *sqlair.Statement
	if increment {
		incrementCharmModifiedVersionStmt, err = st.Prepare(`
UPDATE application
SET charm_modified_version = IFNULL(charm_modified_version,0) + 1
WHERE uuid = $resourceIdentity.application_uuid
`, resourceIdentity)
		if err != nil {
			return errors.Capture(err)
		}
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get charm UUID by joining on the application table.
		err = tx.Query(ctx, getCharmUUIDStmt, resourceIdentity).Get(&charmUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting charm UUID for application %q: %w", res.ApplicationID, apperrors.ApplicationNotFound)
		} else if err != nil {
			return errors.Errorf("getting charm UUID for application %q: %w", res.ApplicationID, err)
		}
		setRes.CharmUUID = charmUUID.UUID

		// Check that the charm has metadata for the resource being added.
		err = tx.Query(ctx, checkCharmResourceExistsStmt, charmUUID, resourceIdentity).Get(&resourceIdentity)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking resource exists on charm: %w", apperrors.CharmResourceNotFound)
		} else if err != nil {
			return errors.Errorf("checking resource exists on charm: %w", err)
		}

		// Get the database ID of the resource origin type.
		err = tx.Query(ctx, getOriginIDStmt, resourceOriginType).Get(&resourceOriginType)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting id for resource origin type: %w: %q", apperrors.UnknownResourceOriginType, resourceOriginType.Name)
		} else if err != nil {
			return errors.Errorf("getting id for resource origin type: %w", err)
		}
		setRes.OriginTypeID = resourceOriginType.ID

		// Get the database ID of the resource added by type.
		err = tx.Query(ctx, getRetrievedByIDStmt, resourceRetrievedByType).Get(&resourceRetrievedByType)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting id for resource added by type: %w: %q", apperrors.UnknownResourceRetrievedByType, resourceRetrievedByType.Name)
		} else if err != nil {
			return errors.Errorf("getting id for resource added by type: %w", err)
		}
		resourceRetrievedBy.RetrievedByTypeID = resourceRetrievedByType.ID

		// Insert the resource row into the resource table.
		err = tx.Query(ctx, insertResourceStmt, setRes).Run()
		if err != nil {
			return errors.Errorf("inserting resource: %w", err)
		}

		// Record the identity of the entity that added the resource.
		err = tx.Query(ctx, insertRetrievedByStmt, resourceRetrievedBy).Run()
		if err != nil {
			return errors.Errorf("inserting resource: %w", err)
		}

		// Link the application to the new resource.
		err = tx.Query(ctx, insertApplicationResourceStmt, resourceIdentity).Run()
		if err != nil {
			return errors.Errorf("inserting application resource: %w", err)
		}

		// Increment the charm modified version if necessary.
		if increment {
			err = tx.Query(ctx, incrementCharmModifiedVersionStmt, resourceIdentity).Run()
			if err != nil {
				return errors.Errorf("inserting resource: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// SetUnitResource sets the resource metadata for a specific unit.
// Returns [apperrors.UnitNotFound] if the unit id doesn't belong to an existing unit.
// Returns [apperrors.ResourceNotFound] if the resource id doesn't belong to an existing resource.
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
			return errors.Errorf("resource %s: %w", resourceUUID.UUID, apperrors.ResourceNotFound)
		}
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, checkValidUnitStmt, unitUUID).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("resource %s: %w", unitUUID.UnitUUID, apperrors.UnitNotFound)
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
				return apperrors.UnknownRetrievedByType
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
// Returns [apperrors.ApplicationNotFound] if the application id doesn't belong to a valid application.
func (st *State) SetRepositoryResources(
	ctx context.Context,
	config resource.SetRepositoryResourcesArgs,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare statement to check that the application exists.
	appDetails := applicationDetails{
		ApplicationID: config.ApplicationID,
	}
	getAppNameQuery := `
SELECT name as &applicationDetails.name 
FROM application 
WHERE uuid = $applicationDetails.uuid
`
	getAppNameStmt, err := st.Prepare(getAppNameQuery, appDetails)
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
		err := tx.Query(ctx, getAppNameStmt, appDetails).Get(&appDetails)
		if errors.Is(err, sqlair.ErrNoRows) {
			return apperrors.ApplicationNotFound
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
				appDetails.Name, config.ApplicationID, set.NewStrings(names...).Difference(foundResources).Values())
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
