// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// createApplicationResources handles resources when creating an application
// by updating resources added before the application via UUID, or by
// inserting new resources. Only one or the other scenario is allowed.
func (st *State) createApplicationResources(
	ctx context.Context,
	tx *sqlair.TX,
	args insertResourcesArgs,
	resourceUUIDs []coreresource.UUID,
) error {
	if len(resourceUUIDs) > 0 {
		return st.resolvePendingResources(ctx, tx, args.appID, args.charmSource, resourceUUIDs)
	}

	return st.insertResources(ctx, tx, args)
}

// buildResourceInserts creates resources to add based on provided app and charm
// resources.
// Returns a slice of resourceToAdd and an error if any issues occur during
// creation.
func (st *State) buildResourcesToAdd(
	charmUUID corecharm.ID,
	charmSource charm.CharmSource,
	appResources []application.AddApplicationResourceArg,
) ([]resourceToAdd, error) {
	var resources []resourceToAdd
	now := st.clock.Now()
	for _, r := range appResources {
		// Available resources are resources actually available for use to the
		// related application.
		uuid, err := coreresource.NewUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}
		resources = append(resources,
			resourceToAdd{
				UUID:      uuid.String(),
				CharmUUID: charmUUID,
				Name:      r.Name,
				Revision:  r.Revision,
				Origin:    r.Origin.String(),
				State:     coreresource.StateAvailable.String(),
				CreatedAt: now,
			})
		if charmSource != charm.CharmHubSource {
			continue
		}
		// Potential resources are possible updates from charm hub for resource
		// linked to the application.
		//
		// In the case of charm from charm hub, juju will regularly fetch the
		// repository to find out if there is newer revision for each resource.
		// Those resources are defined in the state as "potential" resources,
		// and we need to create them "empty" (with no revision) at the creation
		// of the application.
		//
		//   - They are updated by the CharmRevisionWorker, which check charmhub and
		//     updates the charmUUID, revision and polled_at field in the state.
		//   - They are used by resources facade to be compared with actual
		//     resources and provides information on potential updates.
		uuid, err = coreresource.NewUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}
		resources = append(resources,
			resourceToAdd{
				UUID:      uuid.String(),
				CharmUUID: charmUUID,
				Name:      r.Name,
				Revision:  nil, // No revision yet
				Origin:    charmresource.OriginStore.String(),
				State:     coreresource.StatePotential.String(),
				CreatedAt: now,
			})
	}
	return resources, nil
}

type insertResourcesArgs struct {
	appID        coreapplication.ID
	charmUUID    corecharm.ID
	charmSource  charm.CharmSource
	appResources []application.AddApplicationResourceArg
}

// insertResources constructs a transaction to insert resources into the
// database. It returns a function which, when executed, inserts resources and
// links them to applications.
func (st *State) insertResources(ctx context.Context, tx *sqlair.TX, args insertResourcesArgs) error {
	resources, err := st.buildResourcesToAdd(args.charmUUID, args.charmSource, args.appResources)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare SQL statement to insert the resource.
	insertStmt, err := st.Prepare(insertResourceQuery, resourceToAdd{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare SQL statement to link resource with application.
	linkStmt, err := st.Prepare(`
INSERT INTO application_resource (application_uuid, resource_uuid)
VALUES ($linkResourceApplication.*)`, linkResourceApplication{})
	if err != nil {
		return errors.Capture(err)
	}

	// Insert resources
	appUUID := args.appID.String()
	for _, res := range resources {
		// Insert the resource.
		if err = tx.Query(ctx, insertStmt, res).Run(); err != nil {
			return errors.Errorf("inserting resource %q: %w", res.Name, err)
		}
		// Link the resource to the application.
		if err = tx.Query(ctx, linkStmt, linkResourceApplication{
			ResourceUUID:    res.UUID,
			ApplicationUUID: appUUID,
		}).Run(); err != nil {
			return errors.Errorf("linking resource %q to application %q: %w", res.Name, appUUID, err)
		}
	}
	return nil
}

var insertResourceQuery = `
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision, 
       origin_type_id, state_id, created_at)
SELECT $resourceToAdd.uuid,
       $resourceToAdd.charm_uuid,
       $resourceToAdd.charm_resource_name,
       $resourceToAdd.revision,
       rot.id,
       rs.id,
       $resourceToAdd.created_at
FROM   resource_origin_type rot,
       resource_state rs
WHERE  rot.name = $resourceToAdd.origin_type_name
AND    rs.name = $resourceToAdd.state_name`

type uuids []string

// resolvePendingResources finds pending resources for the application and
// makes links them in the application resource link table now that an
// application ID is available. Duplicated those resources for charmhub
// sourced charms as potential.
func (st *State) resolvePendingResources(
	ctx context.Context,
	tx *sqlair.TX,
	appID coreapplication.ID,
	charmSource charm.CharmSource,
	resources []coreresource.UUID,
) error {
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

	// Delete the pending resource links.
	var outcome sqlair.Outcome
	err = tx.Query(ctx, deleteFromPendingApplicationResourceStmt, resUUIDs).Get(&outcome)
	if err != nil {
		return errors.Capture(errors.Errorf("deleting pending resources: %w", err))
	}
	num, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	}
	if num != int64(len(resUUIDs)) {
		return errors.Errorf("expected %d rows to be deleted, got %d", len(resUUIDs), num)
	}

	potentialResourceUUID, err := st.addPotentialFromResourceUUIDs(ctx, tx, charmSource, resUUIDs)
	if err != nil {
		return errors.Capture(err)
	}
	resUUIDs = append(resUUIDs, potentialResourceUUID...)

	// Prepare SQL statement to link resource with application.
	linkStmt, err := st.Prepare(`
INSERT INTO application_resource (application_uuid, resource_uuid)
VALUES ($linkResourceApplication.*)`, linkResourceApplication{})
	if err != nil {
		return errors.Capture(errors.Errorf("linking resources: %w", err))
	}

	// Insert resources
	appUUID := appID.String()
	for _, res := range resUUIDs {
		// Link the resource to the application.
		if err = tx.Query(ctx, linkStmt, linkResourceApplication{
			ResourceUUID:    res,
			ApplicationUUID: appUUID,
		}).Run(); err != nil {
			return errors.Errorf("linking resource %q to application %q: %w", res, appUUID, err)
		}
	}
	return nil
}

// addPotentialFromResourceUUIDs creates potential resources for each
// charmhub charm's resources from the resources created before the
// application. Returns a list of resource uuids to be added to the
// application resource link table.
func (st *State) addPotentialFromResourceUUIDs(ctx context.Context,
	tx *sqlair.TX,
	charmSource charm.CharmSource,
	resUUIDs uuids,
) (uuids, error) {

	// Only add potential resources for charmhub charms. See
	// comment in buildResourcesToAdd for more info.
	if charmSource != charm.CharmHubSource {
		return nil, nil
	}

	potentialUUIDs := make(uuids, len(resUUIDs))

	findStoreResourcesStmt, err := st.Prepare(`
SELECT r.uuid AS &resourceToAdd.uuid,
       r.charm_uuid AS &resourceToAdd.charm_uuid,
       r.charm_resource_name AS &resourceToAdd.charm_resource_name,
       rot.name AS &resourceToAdd.origin_type_name,
       rs.name  AS &resourceToAdd.state_name
FROM   resource AS r
JOIN   resource_origin_type AS rot ON r.origin_type_id = rot.id
JOIN   resource_state AS rs ON r.state_id = rs.id
WHERE  r.uuid IN ($uuids[:])`, uuids{}, resourceToAdd{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var potentialResources []resourceToAdd
	err = tx.Query(ctx, findStoreResourcesStmt, resUUIDs).GetAll(&potentialResources)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf("finding store pending resources for application %w", err)
	}

	// Update store resources to be potential store resources.
	for i := range potentialResources {
		newUUID, err := coreresource.NewUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}
		potentialUUIDs[i] = newUUID.String()
		potentialResources[i].UUID = newUUID.String()
		potentialResources[i].CreatedAt = time.Now()
		potentialResources[i].State = coreresource.StatePotential.String()
	}

	// Prepare SQL statement to insert the resource.
	insertStmt, err := st.Prepare(insertResourceQuery, resourceToAdd{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	for _, res := range potentialResources {
		// Insert the resource.
		if err = tx.Query(ctx, insertStmt, res).Run(); err != nil {
			return nil, errors.Errorf("inserting potential resource from pending %q: %w", res.Name, err)
		}
	}

	return potentialUUIDs, nil
}
