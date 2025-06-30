// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"net"

	"github.com/canonical/sqlair"

	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// GetUnitAndK8sServiceAddresses returns the addresses of the specified unit.
// The addresses are taken from the union the net node UUIDs of the cloud service
// (if any) and the net node UUIDs of the unit, where each net node has an
// associated address.
// This approach allows us to get the addresses regardless of the substrate
// (k8s or machines).
//
// The following errors may be returned:
// - [uniterrors.UnitNotFound] if the unit does not exist
func (st *State) GetUnitAndK8sServiceAddresses(ctx context.Context, uuid coreunit.UUID) (corenetwork.SpaceAddresses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var address []spaceAddress
	ident := entityUUID{UUID: uuid.String()}
	queryUnitPublicAddressesStmt, err := st.Prepare(`
SELECT    &spaceAddress.*
FROM (
    SELECT s.net_node_uuid, u.uuid
    FROM unit u
    JOIN application AS a on a.uuid = u.application_uuid
    JOIN k8s_service AS s on s.application_uuid = a.uuid
    UNION
    SELECT net_node_uuid, uuid FROM unit
) AS n
JOIN      link_layer_device AS lld ON n.net_node_uuid = lld.net_node_uuid
JOIN      v_ip_address_with_names AS ipa ON lld.uuid = ipa.device_uuid
LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid
WHERE     n.uuid = $entityUUID.uuid
`, spaceAddress{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDead(ctx, tx, ident); err != nil {
			return errors.Capture(err)
		}
		err := tx.Query(ctx, queryUnitPublicAddressesStmt, ident).GetAll(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for unit %q (and it's services): %w", uuid, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return encodeIpAddresses(address)
}

// GetUnitAddresses returns the addresses of the specified unit.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
func (st *State) GetUnitAddresses(ctx context.Context, uuid coreunit.UUID) (corenetwork.SpaceAddresses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var address []spaceAddress
	ident := entityUUID{UUID: uuid.String()}
	queryUnitPublicAddressesStmt, err := st.Prepare(`
SELECT    &spaceAddress.*
FROM      unit u
JOIN      link_layer_device AS lld ON u.net_node_uuid = lld.net_node_uuid
JOIN      v_ip_address_with_names AS ipa ON lld.uuid = ipa.device_uuid
LEFT JOIN subnet AS sn ON ipa.subnet_uuid = sn.uuid
WHERE     u.uuid = $entityUUID.uuid
`, spaceAddress{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDead(ctx, tx, ident); err != nil {
			return errors.Capture(err)
		}
		err := tx.Query(ctx, queryUnitPublicAddressesStmt, ident).GetAll(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for unit %q: %w", uuid, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return encodeIpAddresses(address)
}

// GetControllerUnitUUIDByName returns the UUID for the named unit if it
// is a unit of the controller application.
//
// The following errors may be returned:
//   - [applicationerrors.UnitNotFound] if the unit does not exist or is not
//     a controller application unit.
func (st *State) GetControllerUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	query, err := st.Prepare(`
SELECT u.uuid AS &entityUUID.*
FROM   unit AS u
JOIN   application_controller AS ac ON u.application_uuid = ac.application_uuid
WHERE  u.name = $unitName.name
`, entityUUID{}, unitName{})
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	var uuid coreunit.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitName := unitName{Name: name}
		unitUUID := entityUUID{}
		err = tx.Query(ctx, query, unitName).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q not found", name).Add(applicationerrors.UnitNotFound)
		}
		uuid = coreunit.UUID(unitUUID.UUID)
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Errorf("querying unit name: %w", err)
	}

	return uuid, nil
}

// GetUnitUUIDByName returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid coreunit.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err = st.getUnitUUIDByName(ctx, tx, name)
		if err != nil {
			return errors.Errorf("querying unit name: %w", err)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying unit name: %w", err)
	}

	return uuid, nil
}

func (st *State) getUnitUUIDByName(
	ctx context.Context,
	tx *sqlair.TX,
	name coreunit.Name,
) (coreunit.UUID, error) {
	unitName := unitName{Name: name}

	query, err := st.Prepare(`
SELECT &entityUUID.*
FROM   unit
WHERE  name = $unitName.name
`, entityUUID{}, unitName)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	unitUUID := entityUUID{}
	err = tx.Query(ctx, query, unitName).Get(&unitUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q not found", name).Add(applicationerrors.UnitNotFound)
	}
	return coreunit.UUID(unitUUID.UUID), errors.Capture(err)
}

// checkUnitNotDead checks if the unit exists and is not dead. It's possible to
// access alive and dying units, but not dead ones:
// - If the unit is not found, [applicationerrors.UnitNotFound] is returned.
// - If the unit is dead, [applicationerrors.UnitIsDead] is returned.
func (st *State) checkUnitNotDead(ctx context.Context, tx *sqlair.TX, ident entityUUID) error {
	query := `
SELECT &lifeID.*
FROM unit
WHERE uuid = $entityUUID.uuid;
`
	stmt, err := st.Prepare(query, entityUUID{}, lifeID{})
	if err != nil {
		return errors.Errorf("preparing query for unit %q: %w", ident.UUID, err)
	}

	var result lifeID
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("checking unit %q exists: %w", ident.UUID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return applicationerrors.UnitIsDead
	default:
		return nil
	}
}

func encodeIpAddresses(addresses []spaceAddress) (corenetwork.SpaceAddresses, error) {
	res := make(corenetwork.SpaceAddresses, len(addresses))
	for i, addr := range addresses {
		encodedIP, err := encodeIpAddress(addr)
		if err != nil {
			return nil, errors.Capture(err)
		}
		res[i] = encodedIP
	}
	return res, nil
}

func encodeIpAddress(address spaceAddress) (corenetwork.SpaceAddress, error) {
	spaceUUID := corenetwork.AlphaSpaceId
	if address.SpaceUUID.Valid {
		spaceUUID = corenetwork.SpaceUUID(address.SpaceUUID.String)
	}
	// The saved address value is in the form 192.0.2.1/24,
	// parse the parts for the MachineAddress
	ipAddr, ipNet, err := net.ParseCIDR(address.Value)
	if err != nil {
		// Note: IP addresses from Kubernetes do not contain subnet
		// mask suffixes yet. Handle that scenario here. Eventually
		// an error should be returned instead.
		ipAddr = net.ParseIP(address.Value)
	}
	cidr := ipNet.String()
	// Prefer the subnet cidr if one exists.
	if address.SubnetCIDR.Valid {
		cidr = address.SubnetCIDR.String
	}
	return corenetwork.SpaceAddress{
		SpaceID: spaceUUID,
		Origin:  corenetwork.Origin(address.Origin),
		MachineAddress: corenetwork.MachineAddress{
			Value:      ipAddr.String(),
			CIDR:       cidr,
			Type:       corenetwork.AddressType(address.Type),
			Scope:      corenetwork.Scope(address.Scope),
			ConfigType: corenetwork.AddressConfigType(address.ConfigType),
		},
	}, nil
}
