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

// buildResourceInserts creates resources to add based on provided app and charm
// resources.
// Returns a slice of resourceToAdd and an error if any issues occur during
// creation.
func (st *State) buildResourcesToAdd(
	charmUUID string,
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
			UUID:      uuid.String(),
			CharmUUID: charmUUID,
			Name:      r.Name,
			Revision:  r.Revision,
			Origin:    r.Origin.String(),
			State:     coreresource.StateAvailable.String(),
			CreatedAt: now,
		})
	}
	return resources, nil
}

// insertResources constructs a transaction to insert resources into the
// database. It returns a function which, when executed, inserts resources and
// links them to applications.
func (st *State) insertResources(ctx context.Context, tx *sqlair.TX, appDetails applicationDetails, appResources []application.AddApplicationResourceArg) error {

	resources, err := st.buildResourcesToAdd(appDetails.CharmID, appResources)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare SQL statement to insert the resource.
	insertStmt, err := st.Prepare(`
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
AND    rs.name = $resourceToAdd.state_name`, resourceToAdd{})
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
	appUUID := appDetails.UUID.String()
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
