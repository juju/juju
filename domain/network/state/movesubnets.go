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

// MoveSubnetsToSpace moves the specified subnets to a given network space,
// enforcing space constraints if applicable.
func (st *State) MoveSubnetsToSpace(
	ctx context.Context,
	subnetUUIDs []string,
	spaceName string,
	force bool,
) ([]domainnetwork.MovedSubnets, error) {
	if len(subnetUUIDs) == 0 {
		return nil, nil
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var positiveFailures []positiveSpaceConstraintFailure
	var negativeFailures []negativeSpaceConstraintFailure
	var movedSubnets []domainnetwork.MovedSubnets
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkSpaceName(ctx, tx, spaceName)
		if err != nil {
			return errors.Capture(err)
		}
		subnetToMove := uuids(subnetUUIDs)
		positiveFailures, err = st.validateSubnetsLeavingSpaces(ctx, tx, subnetToMove, spaceName)
		if err != nil {
			return errors.Capture(err)
		}
		negativeFailures, err = st.validateSubnetsJoiningSpace(ctx, tx, subnetToMove, spaceName)
		if err != nil {
			return errors.Capture(err)
		}
		if err := st.handleFailures(ctx, positiveFailures, negativeFailures, spaceName, force); err != nil {
			return err
		}
		movedSubnets, err = st.moveSubnets(ctx, tx, subnetToMove, spaceName)
		return errors.Capture(err)
	})

	return movedSubnets, errors.Capture(err)
}

// validateSubnetsLeavingSpaces verifies if subnets moved to a given space violate
// positive space constraints for any machines.
// It queries the database to identify machines whose space constraints
// conflict with the updated subnet-to-space mapping.
// Returns a list of positiveSpaceConstraintFailure instances or an error
// if the process fails.
func (st *State) validateSubnetsLeavingSpaces(
	ctx context.Context,
	tx *sqlair.TX,
	movedSubnets uuids,
	space string,
) ([]positiveSpaceConstraintFailure, error) {

	type spaceTo struct {
		Name string `db:"name"`
	}
	destSpace := spaceTo{Name: space}

	query := `
WITH to_spaces AS (
    -- Maps the moving subnets to the new space
	SELECT space.uuid as space_uuid, subnet.uuid as subnet_uuid
	FROM subnet
	JOIN space ON space.name = $spaceTo.name
	WHERE subnet.uuid IN ($uuids[:])
), from_spaces AS (
	-- Maps the moving subnets to their old spaces
    SELECT DISTINCT space_uuid FROM subnet WHERE uuid IN ($uuids[:])
), application_bindings AS (
	-- Application endpoints bound to the specified spaces
    SELECT application_uuid, space_uuid
    FROM application_endpoint
    WHERE space_uuid IN (SELECT space_uuid FROM from_spaces)
    UNION ALL
    -- Application extra endpoints bound to the specified spaces
    SELECT application_uuid, space_uuid
    FROM application_extra_endpoint
    WHERE space_uuid IN (SELECT space_uuid FROM from_spaces)
    UNION ALL
    -- Application with the space_uuid as default
    SELECT uuid as application_uuid, space_uuid
    FROM application
    WHERE space_uuid IN (SELECT space_uuid FROM from_spaces)
), space_nodes AS (
    -- nodes with units belonging to applications with bindings to the specified spaces
    SELECT DISTINCT u.net_node_uuid AS node_uuid, b.space_uuid
    FROM unit u
    JOIN application_bindings b ON u.application_uuid = b.application_uuid
    UNION
    -- Machines with positive constraints on the specified spaces
    SELECT DISTINCT m.net_node_uuid as node_uuid, s.uuid AS space_uuid
    FROM machine_constraint m
    JOIN constraint_space cs ON m.constraint_uuid = cs.constraint_uuid
    JOIN space s ON cs.space = s.name
    JOIN machine AS m ON m.uuid = m.machine_uuid
    WHERE s.uuid IN (SELECT space_uuid FROM from_spaces)
      AND cs.exclude IS FALSE
), bound_nodes AS (
    -- Select distinct machines to fetch addresses
    SELECT DISTINCT node_uuid FROM space_nodes
), space_addresses AS (
    -- Get all addresses with their destination space if the subnet is moved
    SELECT
        bn.node_uuid,
        coalesce(ts.space_uuid, s.space_uuid) as space_uuid
    FROM bound_nodes bn
    LEFT JOIN ip_address AS a ON bn.node_uuid = a.net_node_uuid
    LEFT JOIN to_spaces AS ts ON a.subnet_uuid = ts.subnet_uuid
    LEFT JOIN subnet AS s ON a.subnet_uuid = s.uuid
), failed_constraints AS (
    -- Filter out nodes which have an address for every bounded space
    SELECT *  FROM space_nodes
	EXCEPT
	SELECT *  FROM space_addresses
)
    SELECT 
		m.name AS &positiveSpaceConstraintFailure.machine_name,
		s.name AS &positiveSpaceConstraintFailure.space_name
FROM failed_constraints AS fc
JOIN space AS s ON fc.space_uuid = s.uuid
JOIN machine AS m ON fc.node_uuid = m.net_node_uuid
`
	stmt, err := st.Prepare(query, destSpace, movedSubnets, positiveSpaceConstraintFailure{})
	if err != nil {
		return nil, errors.Errorf("preparing failed constraint statement: %w,\nquery: %s", err, query)
	}
	var failedConstraints []positiveSpaceConstraintFailure
	err = tx.Query(ctx, stmt, destSpace, movedSubnets).GetAll(&failedConstraints)
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
	movedSubnets uuids,
	space string,
) ([]negativeSpaceConstraintFailure, error) {

	type spaceTo struct {
		Name string `db:"name"`
	}
	destSpace := spaceTo{Name: space}

	query := `
WITH bound_machines AS (    
	SELECT DISTINCT mc.machine_uuid
	FROM machine_constraint AS mc
	JOIN constraint_space AS cs ON mc.constraint_uuid = cs.constraint_uuid
	JOIN space AS s ON cs.space = s.name
	WHERE s.name = $spaceTo.name
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
	stmt, err := st.Prepare(query, destSpace, movedSubnets, negativeSpaceConstraintFailure{})
	if err != nil {
		return nil, errors.Errorf("preparing failed constraint statement: %w,\nquery: %s", err, query)
	}
	var failedConstraints []negativeSpaceConstraintFailure
	err = tx.Query(ctx, stmt, destSpace, movedSubnets).GetAll(&failedConstraints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("checking constraints: %w", err)
	}

	return failedConstraints, nil
}

// handleFailures processes and formats positive and negative space constraint
// failures for logging or error return.
// Group failures by machine name, format the messages, sort them consistently,
// and log or return them based on conditions.
func (st *State) handleFailures(ctx context.Context,
	positive []positiveSpaceConstraintFailure,
	negative []negativeSpaceConstraintFailure,
	space string,
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
				machine, len(addresses), space, strings.Join(addresses, ", ")),
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
		st.logger.Infof(ctx, message)
	}
	return nil
}

// moveSubnets moves a set of subnets to a specified network space within a
// transactional context.
// It returns the updated subnets with their original spaces or an error
// if the operation fails.
func (st *State) moveSubnets(ctx context.Context, tx *sqlair.TX,
	subnetToMove uuids,
	spaceName string,
) ([]domainnetwork.MovedSubnets, error) {

	type subnet struct {
		UUID  string `db:"uuid"`
		Space string `db:"space_name"`
	}
	type spaceTo struct {
		Name string `db:"name"`
	}
	space := spaceTo{Name: spaceName}

	getSpaceStmt, err := st.Prepare(`
SELECT subnet.uuid AS &subnet.uuid, space.name AS &subnet.space_name
FROM subnet
JOIN space ON space.uuid = subnet.space_uuid
WHERE subnet.uuid IN ($uuids[:])`, subnetToMove, subnet{})
	if err != nil {
		return nil, errors.Errorf("preparing get space statement: %w", err)
	}
	var movedSubnets []subnet
	if err := tx.Query(ctx, getSpaceStmt, subnetToMove).GetAll(&movedSubnets); err != nil {
		return nil, errors.Errorf("getting actual subnets spaces: %w", err)
	}

	upsertSubnetSpaceStmt, err := st.Prepare(`
UPDATE subnet
SET space_uuid = (SELECT uuid FROM space WHERE name = $spaceTo.name)
WHERE uuid IN ($uuids[:])`, space, subnetToMove)
	if err != nil {
		return nil, errors.Errorf("preparing upsert subnet space statement: %w", err)
	}
	var outcome sqlair.Outcome
	if err := tx.Query(ctx, upsertSubnetSpaceStmt, space, subnetToMove).Get(&outcome); err != nil {
		return nil, errors.Errorf("updating subnets spaces: %w", err)
	}
	if count, err := outcome.Result().RowsAffected(); err != nil {
		return nil, errors.Errorf("checking expected subnet affected: %w", err)
	} else if count != int64(len(subnetToMove)) {
		return nil, errors.Errorf("expected %d subnets to be updated, got %d", len(subnetToMove), count)
	}

	return transform.Slice(movedSubnets, func(f subnet) domainnetwork.MovedSubnets {
		return domainnetwork.MovedSubnets{
			UUID:      domainnetwork.SubnetUUID(f.UUID),
			FromSpace: network.SpaceName(f.Space),
		}
	}), nil
}
