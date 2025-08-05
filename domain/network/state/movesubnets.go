// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// MoveSubnetsToSpace transfers a list of subnets to a specified network
// space. It verifies that existing machines will still satisfy their
// constraints and bindings. The check can be ignored if forced. In this
// case, failed constraints will be logged.
// Returns the details of moved subnets or an error if any issue occurs
// during the operation.
func (st *State) MoveSubnetsToSpace(
	ctx context.Context,
	subnetUUIDs []string,
	spaceName string,
	force bool,
) ([]domainnetwork.MovedSubnets, error) {
	if len(subnetUUIDs) == 0 {
		return nil, nil
	}

	space, err := st.GetSpaceByName(ctx, network.SpaceName(spaceName))
	if err != nil {
		return nil, errors.Errorf("getting space %q: %w", spaceName, err)
	}
	spaceUUID := space.ID.String()

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var movedSubnets []domainnetwork.MovedSubnets
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		subnetWithSpaces, err := st.getCurrentSubnetWithSpaces(ctx, tx, subnetUUIDs)
		if err != nil {
			return errors.Capture(err)
		}
		subnetToMove := st.filterAlreadyInSpace(subnetWithSpaces, spaceName)

		positiveFailures, err := st.validateSubnetsLeavingSpaces(ctx, tx, subnetToMove)
		if err != nil {
			return errors.Capture(err)
		}
		negativeFailures, err := st.validateSubnetsJoiningSpace(ctx, tx, subnetToMove, spaceUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if err := st.handleFailures(ctx, positiveFailures, negativeFailures, space.Name.String(), force); err != nil {
			return err
		}
		movedSubnets = transform.Slice(subnetToMove, func(uuid string) domainnetwork.MovedSubnets {
			return domainnetwork.MovedSubnets{
				UUID:      domainnetwork.SubnetUUID(uuid),
				FromSpace: network.SpaceName(subnetWithSpaces[uuid]),
			}
		})
		return errors.Capture(st.moveSubnets(ctx, tx, subnetToMove, spaceUUID))
	})

	return movedSubnets, errors.Capture(err)
}

// filterAlreadyInSpace filters out subnet UUIDs that are already associated
// with the specified network space.
// It returns a slice of subnet UUIDs that are not in the given space.
func (st *State) filterAlreadyInSpace(subnetWithSpaces map[string]string, spaceName string) []string {
	subnetToMove := make([]string, 0, len(subnetWithSpaces))
	for subnetUUID, space := range subnetWithSpaces {
		if space != spaceName {
			subnetToMove = append(subnetToMove, subnetUUID)
		}
	}
	return subnetToMove
}

// validateSubnetsLeavingSpaces verifies if subnets moved from their initial
// spaces will not cause violation of positive space constraints for any machines.
// It queries the database to identify machines whose space constraints
// conflict with the updated subnet-to-space mapping.
//
// Returns a list of positiveSpaceConstraintFailure instances or an error
// if the process fails.
func (st *State) validateSubnetsLeavingSpaces(
	ctx context.Context,
	tx *sqlair.TX,
	movingSubnets uuids,
) ([]positiveSpaceConstraintFailure, error) {

	query := `
WITH
app_bindings AS (
    SELECT application_uuid, space_uuid FROM application_endpoint
    UNION ALL
    SELECT application_uuid, space_uuid FROM application_extra_endpoint
    UNION ALL
    SELECT uuid AS application_uuid, space_uuid FROM application
),
machine_bindings AS (
    SELECT m.uuid AS machine_uuid, space_uuid
    FROM   machine AS m
        JOIN unit AS u ON m.net_node_uuid = u.net_node_uuid
        JOIN app_bindings AS b ON u.application_uuid = b.application_uuid
),
machine_constraints AS (
    SELECT m.machine_uuid, s.uuid AS space_uuid
    FROM   machine_constraint AS m
        JOIN constraint_space AS cs ON m.constraint_uuid = cs.constraint_uuid
        JOIN space AS s ON cs.space = s.name
    WHERE  cs.exclude IS FALSE
),
machine_space_reqs AS (
    SELECT machine_uuid, space_uuid FROM machine_bindings
    UNION ALL
    SELECT machine_uuid, space_uuid FROM machine_constraints
),
new_machine_spaces AS (
    SELECT DISTINCT m.uuid AS machine_uuid, s.space_uuid
    FROM   machine AS m
        JOIN ip_address AS ip ON m.net_node_uuid = ip.net_node_uuid
        JOIN subnet AS s ON ip.subnet_uuid = s.uuid
    WHERE  s.uuid NOT IN ($uuids[:])
),
failed_constraints AS (
    SELECT * from machine_space_reqs
    EXCEPT
    SELECT * from new_machine_spaces
)
SELECT 
    m.name AS &positiveSpaceConstraintFailure.machine_name,
    s.name AS &positiveSpaceConstraintFailure.space_name
FROM failed_constraints AS fc
    JOIN space AS s ON fc.space_uuid = s.uuid
    JOIN machine AS m ON fc.machine_uuid = m.uuid
`
	stmt, err := st.Prepare(query, movingSubnets, positiveSpaceConstraintFailure{})
	if err != nil {
		return nil, errors.Errorf("preparing failed constraint statement: %w,\nquery: %s", err, query)
	}
	var failedConstraints []positiveSpaceConstraintFailure
	err = tx.Query(ctx, stmt, movingSubnets).GetAll(&failedConstraints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("checking constraints: %w", err)
	}

	return failedConstraints, nil
}

// validateSubnetsJoiningSpace verifies negative constraints when subnets are moved to a
// specific "space" for machines in the system.
// It returns a slice of negativeSpaceConstraintFailure for failing
// machines and any error encountered during execution.
func (st *State) validateSubnetsJoiningSpace(
	ctx context.Context,
	tx *sqlair.TX,
	movingSubnets uuids,
	spaceUUID string,
) ([]negativeSpaceConstraintFailure, error) {

	type spaceTo entityUUID
	destSpace := spaceTo{UUID: spaceUUID}

	query := `
WITH 
bound_machines AS (    
    SELECT DISTINCT mc.machine_uuid
    FROM machine_constraint AS mc
        JOIN constraint_space AS cs ON mc.constraint_uuid = cs.constraint_uuid
        JOIN space AS s ON cs.space = s.name
    WHERE s.uuid = $spaceTo.uuid
    AND cs.exclude IS TRUE
) 
-- Get all addresses from machines with constraints in any of moved subnet UUID
SELECT 
    m.name AS &negativeSpaceConstraintFailure.machine_name, 
    a.address_value AS &negativeSpaceConstraintFailure.address_value
FROM bound_machines
    JOIN machine AS m ON bound_machines.machine_uuid = m.uuid
    JOIN ip_address AS a ON m.net_node_uuid = a.net_node_uuid
WHERE a.subnet_uuid IN ($uuids[:])
`
	stmt, err := st.Prepare(query, destSpace, movingSubnets, negativeSpaceConstraintFailure{})
	if err != nil {
		return nil, errors.Errorf("preparing failed constraint statement: %w,\nquery: %s", err, query)
	}
	var failedConstraints []negativeSpaceConstraintFailure
	err = tx.Query(ctx, stmt, destSpace, movingSubnets).GetAll(&failedConstraints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("checking constraints: %w", err)
	}

	return failedConstraints, nil
}

// handleFailures processes and formats positive and negative space constraint
// failures. Those errors are either logged or returned depending on the `force`
// input parameter.
// Group failures by machine name, format the messages, sort them consistently,
// and log or return them based on conditions.
func (st *State) handleFailures(ctx context.Context,
	positive []positiveSpaceConstraintFailure,
	negative []negativeSpaceConstraintFailure,
	spaceName string,
	force bool) error {
	if len(positive) == 0 && len(negative) == 0 {
		return nil
	}

	// Group positive failures by machine name
	positiveByMachine, _ := accumulateToMap(positive, func(f positiveSpaceConstraintFailure) (string, string, error) {
		return f.MachineName, f.SpaceName, nil
	})

	// Group negative failures by machine name
	negativeByMachine, _ := accumulateToMap(negative, func(f negativeSpaceConstraintFailure) (string, string, error) {
		return f.MachineName, f.Address, nil
	})

	// Format positive failures
	positiveMessages := transform.MapToSlice(positiveByMachine, func(machine string, spaces []string) []string {
		return []string{
			fmt.Sprintf("machine %q is missing addresses in space(s) %q", machine, strings.Join(spaces, ", ")),
		}
	})

	// Format negative failures
	negativeMessages := transform.MapToSlice(negativeByMachine, func(machine string, addresses []string) []string {
		return []string{
			fmt.Sprintf("machine %q would have %d addresses in excluded space %q (%s)",
				machine, len(addresses), spaceName, strings.Join(addresses, ", ")),
		}
	})

	messages := append(positiveMessages, negativeMessages...)

	if !force {
		// Sort error messages for consistent output
		sort.Strings(messages)
		return errors.Errorf("%s", strings.Join(messages, "\n"))
	}

	// If forced, log everything
	for _, message := range messages {
		st.logger.Warningf(ctx, "MoveSubnetsToSpace: %s", message)
	}
	return nil
}

// getCurrentSubnetWithSpaces retrieves a mapping of subnet UUIDs to their
// associated space names from the database.
// It queries the database using the provided transaction and a list of
// subnet UUIDs, returning the result as a map.
func (st *State) getCurrentSubnetWithSpaces(ctx context.Context, tx *sqlair.TX, subnetUUIDs uuids) (map[string]string, error) {
	type subnet struct {
		UUID  string `db:"uuid"`
		Space string `db:"space_name"`
	}

	getSpaceStmt, err := st.Prepare(`
SELECT subnet.uuid AS &subnet.uuid, space.name AS &subnet.space_name
FROM subnet
JOIN space ON space.uuid = subnet.space_uuid
WHERE subnet.uuid IN ($uuids[:])`, subnetUUIDs, subnet{})
	if err != nil {
		return nil, errors.Errorf("preparing get space statement: %w", err)
	}
	var movedSubnets []subnet
	if err := tx.Query(ctx, getSpaceStmt, subnetUUIDs).GetAll(&movedSubnets); err != nil {
		return nil, errors.Errorf("getting actual subnets spaces: %w", err)
	}
	return transform.SliceToMap(movedSubnets, func(s subnet) (string, string) {
		return s.UUID, s.Space
	}), nil
}

// moveSubnets moves a set of subnets to a specified network space within a
// transactional context.
func (st *State) moveSubnets(ctx context.Context, tx *sqlair.TX,
	subnetToMove uuids,
	spaceUUID string,
) error {
	type spaceTo entityUUID
	space := spaceTo{UUID: spaceUUID}

	upsertSubnetSpaceStmt, err := st.Prepare(`
UPDATE subnet
SET space_uuid = $spaceTo.uuid
WHERE uuid IN ($uuids[:])`, space, subnetToMove)
	if err != nil {
		return errors.Errorf("preparing upsert subnet space statement: %w", err)
	}
	var outcome sqlair.Outcome
	if err := tx.Query(ctx, upsertSubnetSpaceStmt, space, subnetToMove).Get(&outcome); err != nil {
		return errors.Errorf("updating subnets spaces: %w", err)
	}
	if count, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Errorf("checking expected subnet affected: %w", err)
	} else if count != int64(len(subnetToMove)) {
		return errors.Errorf("expected %d subnets to be updated, got %d", len(subnetToMove), count)
	}
	return nil
}
