// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	corenetwork "github.com/juju/juju/core/network"
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
func (st *State) NICsInSpaces(ctx context.Context, nodeUUID string) (map[string][]network.NetInterface, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	// This is esoteric enough to make re-use unlikely.
	type deviceInSpace struct {
		DeviceName string         `db:"device_name"`
		Type       string         `db:"type_name"`
		PortType   string         `db:"port_type_name"`
		MACAddress sql.NullString `db:"mac_address"`
		ParentName sql.NullString `db:"parent_name"`
		SpaceUUID  sql.NullString `db:"space_uuid"`
	}

	mUUID := entityUUID{UUID: nodeUUID}

	qry := `
SELECT DISTINCT 
       d.name AS &deviceInSpace.device_name, 
       t.name AS &deviceInSpace.type_name, 
       v.name AS &deviceInSpace.port_type_name,
       pd.name AS &deviceInSpace.parent_name, 
       (s.space_uuid, d.mac_address) AS (&deviceInSpace.*)
FROM   link_layer_device d
       JOIN link_layer_device_type t on d.device_type_id = t.id	
       JOIN virtual_port_type v on d.virtual_port_type_id = v.id
       LEFT JOIN ip_address a on d.uuid = a.device_uuid
       LEFT JOIN subnet s on a.subnet_uuid = s.uuid
       LEFT JOIN link_layer_device_parent p ON d.uuid = p.device_uuid
       LEFT JOIN link_layer_device pd ON p.parent_uuid = pd.uuid
WHERE  d.net_node_uuid = $entityUUID.uuid`

	stmt, err := st.Prepare(qry, mUUID, deviceInSpace{})
	if err != nil {
		return nil, errors.Errorf("preparing NICs in spaces statement: %w", err)
	}

	var nics []deviceInSpace
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, mUUID).GetAll(&nics); err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying NICs in spaces: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	nicsInSpaces := make(map[string][]network.NetInterface)
	for _, spaceNic := range nics {
		nic := network.NetInterface{
			Name:            spaceNic.DeviceName,
			Type:            corenetwork.LinkLayerDeviceType(spaceNic.Type),
			VirtualPortType: corenetwork.VirtualPortType(spaceNic.PortType),
		}

		if spaceNic.MACAddress.Valid {
			nic.MACAddress = &spaceNic.MACAddress.String
		}

		if spaceNic.ParentName.Valid {
			nic.ParentDeviceName = spaceNic.ParentName.String
		}

		// This ensures that NICs without addresses are not in any space.
		var spaceUUID string
		if spaceNic.SpaceUUID.Valid {
			spaceUUID = spaceNic.SpaceUUID.String
		}

		if spaceNics, ok := nicsInSpaces[spaceUUID]; !ok {
			nicsInSpaces[spaceUUID] = []network.NetInterface{nic}
		} else {
			nicsInSpaces[spaceUUID] = append(spaceNics, nic)
		}
	}
	return nicsInSpaces, nil
}

// GetContainerNetworkingMethod returns the model's configured value
// for container-networking-method.
func (st *State) GetContainerNetworkingMethod(ctx context.Context) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	type cVal struct {
		Value string `db:"value"`
	}
	var conf cVal

	stmt, err := st.Prepare(`SELECT &cVal.* FROM model_config WHERE "key" = 'container-networking-method'`, conf)
	if err != nil {
		return "", errors.Errorf("preparing model config statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).Get(&conf); err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying model config: %w", err)
			}
		}
		return nil
	})
	return conf.Value, errors.Capture(err)
}

// GetSubnetCIDRForDevice uses the device identified by the input node UUID
// and device name to locate the CIDR of the subnet that it is connected to,
// in the input space.
func (st *State) GetSubnetCIDRForDevice(ctx context.Context, nodeUUID, deviceName, spaceUUID string) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	nUUID := netNodeUUID{UUID: nodeUUID}
	dName := name{Name: deviceName}
	sUUID := entityUUID{UUID: spaceUUID}

	qry := `
SELECT &subnet.cidr
FROM   link_layer_device d
       JOIN ip_address a ON d.uuid = a.device_uuid
       JOIN subnet s ON a.subnet_uuid = s.uuid
WHERE  d.net_node_uuid = $netNodeUUID.net_node_uuid
AND    d.name = $name.name
AND    s.space_uuid = $entityUUID.uuid`

	stmt, err := st.Prepare(qry, nUUID, dName, sUUID, subnet{})
	if err != nil {
		return "", errors.Errorf("preparing subnet CIDR statement: %w", err)
	}

	var subnetCIDR subnet
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, nUUID, dName, sUUID).Get(&subnetCIDR); err != nil {
			if err != nil {
				return errors.Errorf("querying subnet CIDR: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return subnetCIDR.CIDR, nil
}
