// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// GetMachineSpaceConstraints returns the positive and negative
// space constraints for the machine with the input UUID.
func (st *State) GetMachineSpaceConstraints(
	ctx context.Context, machineUUID string,
) ([]internal.SpaceName, []internal.SpaceName, error) {
	db, err := st.DB()
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	mUUID := entityUUID{UUID: machineUUID}

	qry := `
SELECT (space, exclude) AS (&spaceConstraint.*),
       s.uuid AS &spaceConstraint.uuid
FROM   constraint_space cs
       JOIN machine_constraint m ON cs.constraint_uuid = m.constraint_uuid
	   JOIN space s ON cs.space = s.name	
WHERE  m.machine_uuid = $entityUUID.uuid`

	stmt, err := st.Prepare(qry, mUUID, spaceConstraint{})
	if err != nil {
		return nil, nil, errors.Errorf("preparing machine space constraints statement: %w", err)
	}

	var cons []spaceConstraint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, mUUID).GetAll(&cons); err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying machine space constraints: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	var pos, neg []internal.SpaceName
	for _, con := range cons {
		sn := internal.SpaceName{
			UUID: con.SpaceUUID,
			Name: con.SpaceName,
		}

		if con.Exclude {
			neg = append(neg, sn)
			continue
		}
		pos = append(pos, sn)
	}

	return pos, neg, nil
}

// GetMachineAppBindings returns the bound spaces for applications
// with units assigned to the machine with the input UUID.
func (st *State) GetMachineAppBindings(ctx context.Context, machineUUID string) ([]internal.SpaceName, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	mUUID := entityUUID{UUID: machineUUID}

	qry := `
WITH all_bound AS (
    SELECT application_uuid, space_uuid
    FROM   application_endpoint
	UNION  
    SELECT application_uuid, space_uuid
    FROM   application_extra_endpoint
)
SELECT DISTINCT 
       s.uuid AS &spaceConstraint.uuid,
       s.name AS &spaceConstraint.space
FROM   machine m
	   JOIN unit u ON m.net_node_uuid = u.net_node_uuid
       JOIN all_bound b ON u.application_uuid = b.application_uuid
	   JOIN space s ON b.space_uuid = s.uuid
WHERE  m.uuid = $entityUUID.uuid
AND    s.name IS NOT NULL`

	stmt, err := st.Prepare(qry, mUUID, spaceConstraint{})
	if err != nil {
		return nil, errors.Errorf("preparing machine app bindings statement: %w", err)
	}

	var cons []spaceConstraint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, mUUID).GetAll(&cons); err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying machine app bindings: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	bound := make([]internal.SpaceName, len(cons))
	for i, con := range cons {
		bound[i] = internal.SpaceName{
			UUID: con.SpaceUUID,
			Name: con.SpaceName,
		}
	}
	return bound, nil
}

// NICsInSpaces returns the link-layer devices on the machine with the
// input net node UUID, indexed by the spaces that they are in.
func (st *State) NICsInSpaces(ctx context.Context, netNode string) (map[string][]network.NetInterface, error) {
	return nil, errors.Errorf("implement me")
}

// GetContainerNetworkingMethod returns the model's configured value
// for container-networking-method.
func (st *State) GetContainerNetworkingMethod(ctx context.Context) (string, error) {
	return "", errors.Errorf("implement me")
}
