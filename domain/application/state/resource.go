// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/internal/errors"
)

// buildResourceToAdd creates resources to add based on provided app and charm
// resources.
// Returns a slice of resourceToAdd and an error if any issues occur during
// creation.
func (st *State) buildResourceToAdd(
	appUUID, charmUUID string,
	appResources []application.AddApplicationResourceArg,
) ([]resourceToAdd, error) {
	resources := make([]resourceToAdd, 0, len(appResources))
	now := st.clock.Now()
	for _, r := range appResources {
		uuid, err := coreresource.NewUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}
		resources = append(resources, resourceToAdd{
			UUID:            uuid.String(),
			ApplicationUUID: appUUID,
			CharmUUID:       charmUUID,
			Name:            r.Name,
			Revision:        r.Revision,
			Origin:          r.Origin.String(),
			State:           coreresource.StateAvailable.String(),
			CreatedAt:       now,
		})
	}
	return resources, nil
}

// insertResources constructs a transaction to insert resources into the
// database. It returns a function which, when executed, inserts resources and
// links them to applications.
func (st *State) insertResources(ctx context.Context, tx *sqlair.TX, appDetails applicationDetails, appResources []application.AddApplicationResourceArg) error {

	resources, err := st.buildResourceToAdd(appDetails.UUID.String(), appDetails.CharmID, appResources)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare SQL statement to get the origin of a specific resource
	getOriginIDStmt, err := st.Prepare(`
SELECT id AS &resourceToAdd.origin_type_id
FROM   resource_origin_type
WHERE  name = $resourceToAdd.origin_type_name`, resourceToAdd{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare SQL statement to get the state of a specific resource.
	getStateIDStmt, err := st.Prepare(`
SELECT id AS &resourceToAdd.state_id
FROM   resource_state
WHERE  name = $resourceToAdd.state_name`, resourceToAdd{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare SQL statement to insert the resource.
	insertStmt, err := st.Prepare(`
INSERT INTO resource (uuid, charm_uuid, charm_resource_name, revision, 
origin_type_id, state_id, created_at)
VALUES ($resourceToAdd.*)`, resourceToAdd{})
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare SQL statement to link resource with application.
	linkStmt, err := st.Prepare(`
INSERT INTO application_resource (application_uuid, resource_uuid)
VALUES ($resourceToAdd.application_uuid, $resourceToAdd.uuid)`, resourceToAdd{})
	if err != nil {
		return errors.Capture(err)
	}

	// Insert resources
	for _, res := range resources {
		// Retrieve state id.
		if err = tx.Query(ctx, getStateIDStmt, res).Get(&res); err != nil {
			return errors.Capture(err)
		}
		// Retrieve origin id.
		if err = tx.Query(ctx, getOriginIDStmt, res).Get(&res); err != nil {
			return errors.Capture(err)
		}
		// Insert the resource.
		if err = tx.Query(ctx, insertStmt, res).Run(); err != nil {
			return errors.Errorf("inserting resource %q: %w", res.Name, err)
		}
		// Link the resource to the application.
		if err = tx.Query(ctx, linkStmt, res).Run(); err != nil {
			return errors.Errorf("linking resource %q to application %q: %w", res.Name, res.ApplicationUUID, err)
		}
	}
	return nil
}
