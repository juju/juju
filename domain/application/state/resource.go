// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"

	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	charmresource "github.com/juju/juju/domain/deployment/charm/resource"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

type resourceReconciliation struct {
	ApplicationUUID string `db:"application_uuid"`
	ResourceUUID    string `db:"resource_uuid"`
	OldResourceUUID string `db:"old_resource_uuid"`
	CharmUUID       string `db:"charm_uuid"`
	Name            string `db:"charm_resource_name"`
	KindID          int    `db:"kind_id"`
}

type charmResourceIdentity struct {
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"name"`
	KindID    int    `db:"kind_id"`
}

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

func (st *State) reconcileApplicationResourcesForCharm(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID string,
	charmUUID string,
	params application.SetCharmStateParams,
) error {
	input := resourceReconciliation{
		ApplicationUUID: appUUID,
		CharmUUID:       charmUUID,
	}
	targetStmt, err := st.Prepare(`
SELECT cr.name AS &charmResourceIdentity.name,
       cr.kind_id AS &charmResourceIdentity.kind_id
FROM   charm_resource AS cr
WHERE  cr.charm_uuid = $charmResourceIdentity.charm_uuid
`, charmResourceIdentity{})
	if err != nil {
		return errors.Capture(err)
	}
	currentStmt, err := st.Prepare(`
SELECT r.uuid AS &resourceReconciliation.resource_uuid,
       r.charm_uuid AS &resourceReconciliation.charm_uuid,
       r.charm_resource_name AS &resourceReconciliation.charm_resource_name,
       cr.kind_id AS &resourceReconciliation.kind_id
FROM   resource AS r
JOIN   application_resource AS ar ON ar.resource_uuid = r.uuid
JOIN   resource_state AS rs ON rs.id = r.state_id
JOIN   charm_resource AS cr
ON     cr.charm_uuid = r.charm_uuid
AND    cr.name = r.charm_resource_name
WHERE  ar.application_uuid = $resourceReconciliation.application_uuid
AND    rs.name = 'available'
`, resourceReconciliation{})
	if err != nil {
		return errors.Capture(err)
	}
	candidateStmt, err := st.Prepare(`
SELECT r.uuid AS &resourceReconciliation.resource_uuid,
       r.charm_uuid AS &resourceReconciliation.charm_uuid,
       r.charm_resource_name AS &resourceReconciliation.charm_resource_name,
       cr.kind_id AS &resourceReconciliation.kind_id
FROM   resource AS r
JOIN   resource_state AS rs ON rs.id = r.state_id
JOIN   charm_resource AS cr
ON     cr.charm_uuid = r.charm_uuid
AND    cr.name = r.charm_resource_name
JOIN   pending_application_resource AS par ON par.resource_uuid = r.uuid
JOIN   application AS a ON a.name = par.application_name
WHERE  r.uuid = $resourceReconciliation.resource_uuid
AND    a.uuid = $resourceReconciliation.application_uuid
AND    rs.name = 'available'
`, resourceReconciliation{})
	if err != nil {
		return errors.Capture(err)
	}

	var targetResources []charmResourceIdentity
	if err := tx.Query(ctx, targetStmt, charmResourceIdentity{CharmUUID: charmUUID}).GetAll(&targetResources); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting destination charm resources: %w", err)
	}
	targetByName := make(map[string]charmResourceIdentity, len(targetResources))
	for _, target := range targetResources {
		targetByName[target.Name] = target
	}

	var currentResources []resourceReconciliation
	if err := tx.Query(ctx, currentStmt, input).GetAll(&currentResources); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting current application resources: %w", err)
	}
	currentByName := make(map[string]resourceReconciliation, len(currentResources))
	for _, current := range currentResources {
		if _, exists := currentByName[current.Name]; exists {
			return errors.Errorf("application has multiple current resources named %q: %w",
				current.Name, applicationerrors.InvalidResourceArgs)
		}
		currentByName[current.Name] = current
	}

	candidates := make(map[string]resourceReconciliation, len(params.ResourceIDs))
	for name, resourceUUID := range params.ResourceIDs {
		target, ok := targetByName[name]
		if !ok {
			return errors.Errorf("resource %q is not defined by destination charm: %w",
				name, applicationerrors.InvalidResourceArgs)
		}
		candidateInput := resourceReconciliation{
			ApplicationUUID: appUUID,
			ResourceUUID:    resourceUUID,
		}
		var candidate resourceReconciliation
		err := tx.Query(ctx, candidateStmt, candidateInput).Get(&candidate)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("resource %q is not pending for the application: %w",
				name, applicationerrors.InvalidResourceArgs)
		} else if err != nil {
			return errors.Errorf("getting pending resource %q: %w", name, err)
		}
		if candidate.Name != name || candidate.CharmUUID != charmUUID || candidate.KindID != target.KindID {
			return errors.Errorf("resource %q does not match the destination charm: %w",
				name, applicationerrors.InvalidResourceArgs)
		}
		candidates[name] = candidate
	}

	for name := range targetByName {
		if _, ok := candidates[name]; ok {
			continue
		}
		if _, ok := currentByName[name]; !ok {
			return errors.Errorf("resource %q is not resolved for the destination charm: %w",
				name, applicationerrors.InvalidResourceArgs)
		}
	}

	updateLinkStmt, err := st.Prepare(`
UPDATE application_resource
SET    resource_uuid = $resourceReconciliation.resource_uuid
WHERE  application_uuid = $resourceReconciliation.application_uuid
AND    resource_uuid = $resourceReconciliation.old_resource_uuid
`, resourceReconciliation{})
	if err != nil {
		return errors.Capture(err)
	}
	insertLinkStmt, err := st.Prepare(`
INSERT INTO application_resource (application_uuid, resource_uuid)
VALUES ($resourceReconciliation.application_uuid, $resourceReconciliation.resource_uuid)
`, resourceReconciliation{})
	if err != nil {
		return errors.Capture(err)
	}
	deleteLinkStmt, err := st.Prepare(`
DELETE FROM application_resource
WHERE application_uuid = $resourceReconciliation.application_uuid
AND   resource_uuid = $resourceReconciliation.resource_uuid
`, resourceReconciliation{})
	if err != nil {
		return errors.Capture(err)
	}
	deletePendingStmt, err := st.Prepare(`
DELETE FROM pending_application_resource
WHERE resource_uuid = $resourceReconciliation.resource_uuid
`, resourceReconciliation{})
	if err != nil {
		return errors.Capture(err)
	}

	for name, candidate := range candidates {
		candidate.ApplicationUUID = appUUID
		if current, ok := currentByName[name]; ok {
			candidate.OldResourceUUID = current.ResourceUUID
			if err := tx.Query(ctx, updateLinkStmt, candidate).Run(); err != nil {
				return errors.Errorf("activating replacement resource %q: %w", name, err)
			}
		} else if err := tx.Query(ctx, insertLinkStmt, candidate).Run(); err != nil {
			return errors.Errorf("activating added resource %q: %w", name, err)
		}
		if err := tx.Query(ctx, deletePendingStmt, candidate).Run(); err != nil {
			return errors.Errorf("removing pending resource %q: %w", name, err)
		}
	}

	for name, current := range currentByName {
		if _, retained := targetByName[name]; retained {
			continue
		}
		current.ApplicationUUID = appUUID
		if err := tx.Query(ctx, deleteLinkStmt, current).Run(); err != nil {
			return errors.Errorf("detaching removed resource %q: %w", name, err)
		}
	}

	if err := st.replaceApplicationResourcesForCharm(
		ctx, tx, appUUID, charmUUID, params.ReplacementResourceUUIDs,
	); err != nil {
		return errors.Capture(err)
	}

	return st.reconcileRepositoryResourcesForCharm(
		ctx, tx, appUUID, charmUUID, targetByName, params.RepositoryResourceUUIDs,
	)
}

func (st *State) reconcileRepositoryResourcesForCharm(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID string,
	charmUUID string,
	targetResources map[string]charmResourceIdentity,
	replacementUUIDs map[string]string,
) error {
	input := resourceReconciliation{
		ApplicationUUID: appUUID,
		CharmUUID:       charmUUID,
	}
	currentStmt, err := st.Prepare(`
SELECT r.uuid AS &resourceReconciliation.resource_uuid,
       r.charm_uuid AS &resourceReconciliation.charm_uuid,
       r.charm_resource_name AS &resourceReconciliation.charm_resource_name
FROM   resource AS r
JOIN   application_resource AS ar ON ar.resource_uuid = r.uuid
JOIN   resource_state AS rs ON rs.id = r.state_id
WHERE  ar.application_uuid = $resourceReconciliation.application_uuid
AND    rs.name = 'potential'
`, resourceReconciliation{})
	if err != nil {
		return errors.Capture(err)
	}
	deleteLinkStmt, err := st.Prepare(`
DELETE FROM application_resource
WHERE application_uuid = $resourceReconciliation.application_uuid
AND   resource_uuid = $resourceReconciliation.resource_uuid
`, resourceReconciliation{})
	if err != nil {
		return errors.Capture(err)
	}
	insertResourceStmt, err := st.Prepare(insertResourceQuery, resourceToAdd{})
	if err != nil {
		return errors.Capture(err)
	}
	insertLinkStmt, err := st.Prepare(`
INSERT INTO application_resource (application_uuid, resource_uuid)
VALUES ($linkResourceApplication.*)
`, linkResourceApplication{})
	if err != nil {
		return errors.Capture(err)
	}
	type charmSource struct {
		UUID   string `db:"uuid"`
		Source string `db:"source"`
	}
	charmSourceStmt, err := st.Prepare(`
SELECT cs.name AS &charmSource.source
FROM   charm AS c
JOIN   charm_source AS cs ON cs.id = c.source_id
WHERE  c.uuid = $charmSource.uuid
`, charmSource{})
	if err != nil {
		return errors.Capture(err)
	}

	var currentResources []resourceReconciliation
	if err := tx.Query(ctx, currentStmt, input).GetAll(&currentResources); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting repository resources: %w", err)
	}
	currentByName := make(map[string]resourceReconciliation, len(currentResources))
	for _, current := range currentResources {
		if current.CharmUUID == charmUUID {
			currentByName[current.Name] = current
			continue
		}
		current.ApplicationUUID = appUUID
		if err := tx.Query(ctx, deleteLinkStmt, current).Run(); err != nil {
			return errors.Errorf("detaching repository resource %q: %w", current.Name, err)
		}
	}
	for name, current := range currentByName {
		if _, retained := targetResources[name]; retained {
			continue
		}
		current.ApplicationUUID = appUUID
		if err := tx.Query(ctx, deleteLinkStmt, current).Run(); err != nil {
			return errors.Errorf("detaching removed repository resource %q: %w", name, err)
		}
		delete(currentByName, name)
	}

	var source charmSource
	if err := tx.Query(ctx, charmSourceStmt, charmSource{UUID: charmUUID}).Get(&source); err != nil {
		return errors.Errorf("getting destination charm source: %w", err)
	}
	if source.Source != string(charm.CharmHubSource) {
		return nil
	}

	for name := range targetResources {
		if _, exists := currentByName[name]; exists {
			continue
		}
		resourceUUID, ok := replacementUUIDs[name]
		if !ok {
			return errors.Errorf("repository resource UUID not supplied for resource %q", name)
		}
		resourceToAdd := resourceToAdd{
			UUID:      resourceUUID,
			CharmUUID: charmUUID,
			Name:      name,
			Origin:    charmresource.OriginStore.String(),
			State:     coreresource.StatePotential.String(),
			CreatedAt: st.clock.Now().UTC(),
		}
		if err := tx.Query(ctx, insertResourceStmt, resourceToAdd).Run(); err != nil {
			return errors.Errorf("inserting repository resource %q: %w", name, err)
		}
		if err := tx.Query(ctx, insertLinkStmt, linkResourceApplication{
			ResourceUUID:    resourceUUID,
			ApplicationUUID: appUUID,
		}).Run(); err != nil {
			return errors.Errorf("linking repository resource %q: %w", name, err)
		}
	}

	return nil
}

// replaceApplicationResourcesForCharm gives each retained application resource
// an identity scoped to the destination charm. Stored content is shared with
// the immutable source resource; unit links remain on that source while units
// converge.
func (st *State) replaceApplicationResourcesForCharm(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID string,
	charmUUID string,
	replacementUUIDs map[string]string,
) error {
	type replacement struct {
		ApplicationUUID string    `db:"application_uuid"`
		OldUUID         string    `db:"old_uuid"`
		NewUUID         string    `db:"new_uuid"`
		CharmUUID       string    `db:"charm_uuid"`
		Name            string    `db:"charm_resource_name"`
		OldKindID       int       `db:"old_kind_id"`
		NewKindID       int       `db:"new_kind_id"`
		CreatedAt       time.Time `db:"created_at"`
	}

	input := replacement{
		ApplicationUUID: appUUID,
		CharmUUID:       charmUUID,
	}
	selectStmt, err := st.Prepare(`
SELECT r.uuid AS &replacement.old_uuid,
       r.charm_resource_name AS &replacement.charm_resource_name,
       old_cr.kind_id AS &replacement.old_kind_id,
       new_cr.kind_id AS &replacement.new_kind_id
FROM   resource AS r
JOIN   application_resource AS ar ON ar.resource_uuid = r.uuid
JOIN   resource_state AS rs ON rs.id = r.state_id
JOIN   charm_resource AS old_cr
ON     old_cr.charm_uuid = r.charm_uuid
AND    old_cr.name = r.charm_resource_name
JOIN   charm_resource AS new_cr
ON     new_cr.charm_uuid = $replacement.charm_uuid
AND    new_cr.name = r.charm_resource_name
WHERE  ar.application_uuid = $replacement.application_uuid
AND    rs.name = 'available'
AND    r.charm_uuid != $replacement.charm_uuid
`, replacement{})
	if err != nil {
		return errors.Capture(err)
	}

	insertResourceStmt, err := st.Prepare(`
INSERT INTO resource (
    uuid, charm_uuid, charm_resource_name, revision, origin_type_id,
    state_id, created_at, last_polled
)
SELECT $replacement.new_uuid,
       $replacement.charm_uuid,
       r.charm_resource_name,
       r.revision,
       r.origin_type_id,
       r.state_id,
       $replacement.created_at,
       r.last_polled
FROM   resource AS r
WHERE  r.uuid = $replacement.old_uuid
`, replacement{})
	if err != nil {
		return errors.Capture(err)
	}

	copyFileStoreStmt, err := st.Prepare(`
INSERT INTO resource_file_store (resource_uuid, store_uuid, size, sha384)
SELECT $replacement.new_uuid, rfs.store_uuid, rfs.size, rfs.sha384
FROM   resource_file_store AS rfs
WHERE  rfs.resource_uuid = $replacement.old_uuid
`, replacement{})
	if err != nil {
		return errors.Capture(err)
	}

	copyImageStoreStmt, err := st.Prepare(`
INSERT INTO resource_image_store (
    resource_uuid, store_storage_key, size, sha384
)
SELECT $replacement.new_uuid,
       ris.store_storage_key,
       ris.size,
       ris.sha384
FROM   resource_image_store AS ris
WHERE  ris.resource_uuid = $replacement.old_uuid
`, replacement{})
	if err != nil {
		return errors.Capture(err)
	}

	copyRetrievedByStmt, err := st.Prepare(`
INSERT INTO resource_retrieved_by (resource_uuid, retrieved_by_type_id, name)
SELECT $replacement.new_uuid, rrb.retrieved_by_type_id, rrb.name
FROM   resource_retrieved_by AS rrb
WHERE  rrb.resource_uuid = $replacement.old_uuid
`, replacement{})
	if err != nil {
		return errors.Capture(err)
	}

	replaceApplicationResourceStmt, err := st.Prepare(`
UPDATE application_resource
SET    resource_uuid = $replacement.new_uuid
WHERE  application_uuid = $replacement.application_uuid
AND    resource_uuid = $replacement.old_uuid
`, replacement{})
	if err != nil {
		return errors.Capture(err)
	}

	var replacements []replacement
	if err := tx.Query(ctx, selectStmt, input).GetAll(&replacements); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting resources to replace: %w", err)
	}

	for i := range replacements {
		if replacements[i].OldKindID != replacements[i].NewKindID {
			return errors.Errorf(
				"cannot reuse resource %q when its type changes", replacements[i].Name,
			)
		}
		newUUID, ok := replacementUUIDs[replacements[i].Name]
		if !ok {
			return errors.Errorf(
				"replacement UUID not supplied for resource %q", replacements[i].Name,
			)
		}
		replacements[i].ApplicationUUID = appUUID
		replacements[i].NewUUID = newUUID
		replacements[i].CharmUUID = charmUUID
		replacements[i].CreatedAt = st.clock.Now().UTC()

		if err := tx.Query(ctx, insertResourceStmt, replacements[i]).Run(); err != nil {
			return errors.Errorf("inserting replacement resource: %w", err)
		}
		if err := tx.Query(ctx, copyFileStoreStmt, replacements[i]).Run(); err != nil {
			return errors.Errorf("copying file resource storage link: %w", err)
		}
		if err := tx.Query(ctx, copyImageStoreStmt, replacements[i]).Run(); err != nil {
			return errors.Errorf("copying image resource storage link: %w", err)
		}
		if err := tx.Query(ctx, copyRetrievedByStmt, replacements[i]).Run(); err != nil {
			return errors.Errorf("copying resource retrieval metadata: %w", err)
		}
		if err := tx.Query(ctx, replaceApplicationResourceStmt, replacements[i]).Run(); err != nil {
			return errors.Errorf("selecting replacement resource: %w", err)
		}
	}

	return nil
}

// buildResourceInserts creates resources to add based on provided app and charm
// resources.
// Returns a slice of resourceToAdd and an error if any issues occur during
// creation.
func (st *State) buildResourcesToAdd(
	charmUUID string,
	charmSource charm.CharmSource,
	appResources []application.AddApplicationResourceArg,
) ([]resourceToAdd, error) {
	var resources []resourceToAdd
	now := st.clock.Now().UTC()
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
	appID        string
	charmUUID    string
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
	appUUID := args.appID
	for _, res := range resources {
		// Insert the resource.
		if err = tx.Query(ctx, insertStmt, res).Run(); database.IsErrConstraintForeignKey(err) {
			return errors.Errorf("inserting resource %q: resource not found in charm metadata", res.Name)
		} else if err != nil {
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
// application UUID is available. Duplicated those resources for charmhub
// sourced charms as potential.
func (st *State) resolvePendingResources(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID string,
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
		potentialResources[i].CreatedAt = st.clock.Now().UTC()
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
