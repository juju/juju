// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
)

// RemoveSpace removes a space from the system, optionally forcing removal,
// or simulating it via dry run.
// It returns any violations preventing the space's removal along with an
// error if the operation fails.
func (st *State) RemoveSpace(ctx context.Context,
	spaceName network.SpaceName,
	force, dryRun bool) (domainnetwork.RemoveSpaceViolations, error) {
	var violations domainnetwork.RemoveSpaceViolations
	db, err := st.DB()
	if err != nil {
		return violations, errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		violations, err = st.checkRemoveSpace(ctx, tx, spaceName)
		if err != nil {
			return errors.Errorf("checking for violations: %w", err)
		}
		// We remove the space only if we are not in dry run, have no violation
		//  or are in force mode.
		if dryRun || !violations.IsEmpty() && !force {
			return nil
		}

		return errors.Capture(st.deleteSpace(ctx, tx, spaceName))
	}); err != nil {
		return violations, errors.Capture(err)
	}

	return violations, nil
}

// checkRemoveSpace checks for violations that may prevent the removal of a
// specified space in the system.
func (st *State) checkRemoveSpace(ctx context.Context,
	tx *sqlair.TX,
	spaceName network.SpaceName) (domainnetwork.RemoveSpaceViolations, error) {
	var err error
	result := domainnetwork.RemoveSpaceViolations{}

	if result.HasModelConstraint, err = st.hasModelSpaceConstraint(ctx, tx, spaceName); err != nil {
		return result, errors.Errorf("checking model constraint: %w", err)
	}
	if result.ApplicationConstraints, err = st.getApplicationConstraintsForSpace(ctx, tx, spaceName); err != nil {
		return result, errors.Errorf("checking application constraints: %w", err)
	}
	if result.ApplicationBindings, err = st.getApplicationBoundToSpace(ctx, tx, spaceName); err != nil {
		return result, errors.Errorf("checking application bindings: %w", err)
	}
	return result, nil
}

// deleteSpace removes the specified space from the database within a transaction context.
// It also removes all dependencies:
// - if the model has a space constraint on it, the constraint will be removed
// - if any application have a space constraint on it, they will be removed
// - if any application is bound to the space, it will be set to the alpha space
// - if any application endpoint is bound to space, binding will be removed
// - if any application exposed endpoint is bound to the space, it will be set to the alpha space
// - Any subnets belonging to the space will be moved to alpha space.
// - Any provider id for the space will be removed
func (st *State) deleteSpace(ctx context.Context,
	tx *sqlair.TX,
	spaceName network.SpaceName) error {
	// First, get the space UUID from the name
	toDelete, err := st.getSpaceUUIDFromName(ctx, tx, spaceName)
	if err != nil {
		return errors.Errorf("getting space UUID: %w", err)
	}

	if toDelete.UUID == network.AlphaSpaceId {
		return errors.Errorf("cannot remove the alpha space")
	}

	// Remove space constraints for applications and models
	if err := st.removeSpaceConstraints(ctx, tx, toDelete); err != nil {
		return errors.Errorf("removing model constraints: %w", err)
	}

	// Update application default bindings to use the alpha space
	if err := st.resetSpaceReference(ctx, tx, "application", toDelete); err != nil {
		return errors.Errorf("updating application bindings: %w", err)
	}

	// Update application exposed endpoints to use the alpha space
	if err := st.resetSpaceReference(ctx, tx, "application_exposed_endpoint_space", toDelete); err != nil {
		return errors.Errorf("updating application exposed endpoints: %w", err)
	}

	// Update application endpoints to null space constraint
	if err := st.removeSpaceReference(ctx, tx, "application_endpoint", toDelete); err != nil {
		return errors.Errorf("updating application endpoints: %w", err)
	}

	// Update application extra endpoints to null space constraint
	if err := st.removeSpaceReference(ctx, tx, "application_extra_endpoint", toDelete); err != nil {
		return errors.Errorf("updating application extra endpoints: %w", err)
	}

	// Move subnets to the alpha space
	if err := st.resetSpaceReference(ctx, tx, "subnet", toDelete); err != nil {
		return errors.Errorf("updating subnet space: %w", err)
	}

	// Remove provider ID for the space
	if err := st.removeProviderSpace(ctx, tx, toDelete); err != nil {
		return errors.Errorf("deleting provider space: %w", err)
	}

	// Remove the space itself
	if err := st.removeSpaceRecord(ctx, tx, toDelete); err != nil {
		return errors.Errorf("deleting space: %w", err)
	}

	return nil
}

// getSpaceUUIDFromName retrieves the UUID of a space given its name.
func (st *State) getSpaceUUIDFromName(ctx context.Context, tx *sqlair.TX, spaceName network.SpaceName) (space, error) {
	toDelete := space{Name: spaceName}
	stmt, err := st.Prepare(`
SELECT &space.uuid
FROM space
WHERE name = $space.name`, toDelete)
	if err != nil {
		return toDelete, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, toDelete).Get(&toDelete)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return toDelete, networkerrors.SpaceNotFound
		}
		return toDelete, errors.Capture(err)
	}

	return toDelete, nil
}

// removeSpaceConstraints removes all constraints associated with the given space.
func (st *State) removeSpaceConstraints(ctx context.Context, tx *sqlair.TX, spaceToDelete space) error {
	stmt, err := st.Prepare(`
DELETE FROM constraint_space
WHERE space = $space.name`, spaceToDelete)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, spaceToDelete).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// resetSpaceReference updates references of the specified space to the alpha
// space ID in the given database table.
func (st *State) resetSpaceReference(ctx context.Context, tx *sqlair.TX, table string, spaceToDelete space) error {
	alphaUUID := entityUUID{UUID: network.AlphaSpaceId.String()}
	stmt, err := st.Prepare(fmt.Sprintf(`
UPDATE %q
SET space_uuid = $entityUUID.uuid
WHERE space_uuid = $space.uuid`, table), spaceToDelete, alphaUUID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, spaceToDelete, alphaUUID).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// removeSpaceReference nullifies the space_uuid fields in the specified table
// where matched to the input space UUID.
func (st *State) removeSpaceReference(ctx context.Context, tx *sqlair.TX, table string, spaceToDelete space) error {
	stmt, err := st.Prepare(fmt.Sprintf(`
UPDATE %q
SET space_uuid = NULL
WHERE space_uuid = $space.uuid`, table), spaceToDelete)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, spaceToDelete).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// removeProviderSpace removes the provider ID for the given space.
func (st *State) removeProviderSpace(ctx context.Context, tx *sqlair.TX, spaceToDelete space) error {
	stmt, err := st.Prepare(`
DELETE FROM provider_space
WHERE space_uuid = $space.uuid;`, spaceToDelete)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, spaceToDelete).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// removeSpaceRecord removes the space record itself.
func (st *State) removeSpaceRecord(ctx context.Context, tx *sqlair.TX, spaceToDelete space) error {
	stmt, err := st.Prepare(`
DELETE FROM space
WHERE uuid = $space.uuid;`, spaceToDelete)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, spaceToDelete).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// hasModelSpaceConstraint checks if a model space constraint exists for the
// given spaceName in the specified transaction.
func (st *State) hasModelSpaceConstraint(ctx context.Context, tx *sqlair.TX, spaceName network.SpaceName) (bool,
	error) {
	type space name
	current := space{Name: spaceName.String()}
	stmt, err := st.Prepare(`
SELECT constraint_uuid AS &entityUUID.uuid
FROM v_model_constraint_space
WHERE space = $space.name`, current, entityUUID{})
	if err != nil {
		return false, errors.Capture(err)
	}

	var constraint entityUUID
	err = tx.Query(ctx, stmt, current).Get(&constraint)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	}

	return err == nil, err
}

// getApplicationConstraintsForSpace fetches application names with a constraint
// associated with the specified space name.
func (st *State) getApplicationConstraintsForSpace(ctx context.Context, tx *sqlair.TX, spaceName network.SpaceName) ([]string, error) {
	type space name
	type application name
	current := space{Name: spaceName.String()}

	stmt, err := st.Prepare(`
SELECT a.name AS &application.name
FROM constraint_space AS cs
JOIN application_constraint AS ac ON cs.constraint_uuid = ac.constraint_uuid
JOIN application AS a ON a.uuid = ac.application_uuid
WHERE cs.space = $space.name`, current, application{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var applications []application
	if err := tx.Query(ctx, stmt, current).GetAll(&applications); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return transform.Slice(applications, func(f application) string {
		return f.Name
	}), nil
}

// getApplicationBoundToSpace retrieves the names of applications bound to a
// specific space as their default space or through any endpoint or exposed
// endpoint
func (st *State) getApplicationBoundToSpace(ctx context.Context, tx *sqlair.TX, spaceName network.SpaceName) ([]string, error) {
	type space name
	type application name
	current := space{Name: spaceName.String()}

	stmt, err := st.Prepare(`
WITH
bindings AS (
    SELECT uuid AS application_uuid, space_uuid FROM application
    UNION ALL
    SELECT application_uuid, space_uuid FROM application_endpoint
    UNION ALL
    SELECT application_uuid, space_uuid FROM application_extra_endpoint
    UNION ALL
    SELECT application_uuid, space_uuid FROM application_exposed_endpoint_space
)
SELECT DISTINCT a.name AS &application.name
FROM bindings AS b
JOIN application AS a ON b.application_uuid = a.uuid
JOIN space AS s ON b.space_uuid = s.uuid
WHERE s.name = $space.name`, current, application{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var applications []application
	if err := tx.Query(ctx, stmt, current).GetAll(&applications); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return transform.Slice(applications, func(f application) string {
		return f.Name
	}), nil
}
