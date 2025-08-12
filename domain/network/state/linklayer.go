// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// GetMachineNetNodeUUID returns the net node UUID for the input machine UUID.
// The following errors may be returned:
//   - [github.com/juju/juju/domain/machine/errors.MachineNotFound]
//     if such a machine does not exist.
func (st *State) GetMachineNetNodeUUID(ctx context.Context, machineUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	mUUID := entityUUID{UUID: machineUUID}
	var nUUID netNodeUUID

	stmt, err := st.Prepare("SELECT &netNodeUUID.* FROM machine WHERE uuid = $entityUUID.uuid", mUUID, nUUID)
	if err != nil {
		return "", errors.Errorf("preparing machine net node statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, mUUID).Get(&nUUID); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return machineerrors.MachineNotFound
			}
			return errors.Errorf("querying machine net node: %w", err)
		}
		return nil
	})
	return nUUID.UUID, errors.Capture(err)
}

// GetAllLinkLayerDevicesByNetNodeUUIDs retrieves all link-layer devices grouped
// by their associated NetNodeUUIDs.
// The function fetches link-layer devices, DNS domains, DNS addresses,
// and IP addresses, then maps them accordingly.
// Returns a map of NetNodeUUID to a slice of NetInterface and an error if
// any operation fails during execution.
func (st *State) GetAllLinkLayerDevicesByNetNodeUUIDs(ctx context.Context) (map[string][]network.NetInterface, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var llds []getLinkLayerDevice
	var dnsDomains []dnsSearchDomainRow
	var dnsAddresses []dnsAddressRow
	var ipAddresses []getIpAddress

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		llds, err = st.getAllLinkLayerDevices(ctx, tx)
		if err != nil {
			return errors.Errorf("fetching all link layer devices: %w", err)
		}
		dnsDomains, err = st.getAllDNSDomains(ctx, tx)
		if err != nil {
			return errors.Errorf("fetching all DNS search domains: %w", err)
		}
		dnsAddresses, err = st.getAllDNSAddresses(ctx, tx)
		if err != nil {
			return errors.Errorf("fetching all DNS addresses: %w", err)
		}
		ipAddresses, err = st.getAllAddresses(ctx, tx)
		if err != nil {
			return errors.Errorf("fetching all IP addresses: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Errorf("fetching all link layer devices: %w", err)
	}

	dnsDomainByDeviceUUID, _ := accumulateToMap(dnsDomains, func(in dnsSearchDomainRow) (string, string, error) {
		return in.DeviceUUID, in.SearchDomain, nil
	})
	dnsAddressesByDeviceUUID, _ := accumulateToMap(dnsAddresses, func(in dnsAddressRow) (string, string, error) {
		return in.DeviceUUID, in.Address, nil
	})
	ipAddressByDeviceUUID, _ := accumulateToMap(ipAddresses, func(f getIpAddress) (string, getIpAddress, error) {
		return f.DeviceUUID, f, nil
	})

	return accumulateToMap(llds, func(in getLinkLayerDevice) (string, network.NetInterface, error) {
		result, err := in.toNetInterface(dnsDomainByDeviceUUID[in.UUID],
			dnsAddressesByDeviceUUID[in.UUID],
			ipAddressByDeviceUUID[in.UUID])
		return in.NetNodeUUID, result, err
	})
}

// accumulateToMap transforms a slice of elements into a map of keys to slices
// of values using the provided transform function.
// If the transformation function results in an error, end the loop and return
// the error
func accumulateToMap[F any, K comparable, V any](from []F, transform func(F) (K, V, error)) (map[K][]V, error) {
	to := make(map[K][]V)
	for _, oneFrom := range from {
		k, v, err := transform(oneFrom)
		if err != nil {
			return nil, errors.Capture(err)
		}
		to[k] = append(to[k], v)
	}
	return to, nil
}

// getAllLinkLayerDevices fetches all link-layer devices from the database
// within the context of a transaction.
// It executes a SQL query to retrieve device fields, including UUID, name,
// provider details, and other attributes.
// The method returns a slice of getLinkLayerDevice and an error if the
// query preparation or execution fails.
func (st *State) getAllLinkLayerDevices(ctx context.Context, tx *sqlair.TX) ([]getLinkLayerDevice, error) {
	stmt, err := st.Prepare(`
SELECT 
	lld.uuid AS &getLinkLayerDevice.uuid,
	lld.net_node_uuid AS &getLinkLayerDevice.net_node_uuid,
	lld.name AS &getLinkLayerDevice.name,
	lldpn.name AS &getLinkLayerDevice.parent_name,
	plld.provider_id AS &getLinkLayerDevice.provider_id,
	lld.mtu AS &getLinkLayerDevice.mtu,
	lld.mac_address AS &getLinkLayerDevice.mac_address,
	lldt.name AS &getLinkLayerDevice.device_type,
	vpt.name AS &getLinkLayerDevice.virtual_port_type,
	lld.is_auto_start AS &getLinkLayerDevice.is_auto_start,
	lld.is_enabled AS &getLinkLayerDevice.is_enabled,
	lld.is_default_gateway AS &getLinkLayerDevice.is_default_gateway,
	lld.gateway_address AS &getLinkLayerDevice.gateway_address,
	lld.vlan_tag AS &getLinkLayerDevice.vlan_tag
FROM link_layer_device AS lld
JOIN link_layer_device_type AS lldt ON lld.device_type_id = lldt.id
JOIN virtual_port_type AS vpt ON lld.virtual_port_type_id = vpt.id
LEFT JOIN provider_link_layer_device AS plld ON lld.uuid = plld.device_uuid
LEFT JOIN link_layer_device_parent AS lldp ON lld.uuid = lldp.device_uuid
LEFT JOIN link_layer_device AS lldpn ON lldp.parent_uuid = lldpn.uuid
`, getLinkLayerDevice{})
	if err != nil {
		return nil, errors.Errorf("preparing link layer device select statement: %w", err)
	}

	var llds []getLinkLayerDevice
	err = tx.Query(ctx, stmt).GetAll(&llds)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("querying link layer devices: %w", err)
	}
	return llds, nil
}

// getAllDNSDomains retrieves all DNS search domain rows from the
// link_layer_device_dns_domain table within a transaction.
func (st *State) getAllDNSDomains(ctx context.Context, tx *sqlair.TX) ([]dnsSearchDomainRow, error) {
	stmt, err := st.Prepare(`
SELECT &dnsSearchDomainRow.* 
FROM link_layer_device_dns_domain
`, dnsSearchDomainRow{})
	if err != nil {
		return nil, errors.Errorf("preparing DNS search domain select statement: %w", err)
	}

	var domains []dnsSearchDomainRow
	err = tx.Query(ctx, stmt).GetAll(&domains)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("querying DNS search domains: %w", err)
	}
	return domains, nil
}

// getAllDNSAddresses retrieves all DNS address entries from the
// link_layer_device_dns_address table in the database.
func (st *State) getAllDNSAddresses(ctx context.Context, tx *sqlair.TX) ([]dnsAddressRow, error) {
	stmt, err := st.Prepare(`
SELECT &dnsAddressRow.* 
FROM link_layer_device_dns_address
`, dnsAddressRow{})
	if err != nil {
		return nil, errors.Errorf("preparing DNS address select statement: %w", err)
	}

	var addresses []dnsAddressRow
	err = tx.Query(ctx, stmt).GetAll(&addresses)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("querying DNS addresses: %w", err)
	}
	return addresses, nil
}

// getAllAddresses retrieves all IP addresses from the database using the
// provided context and transaction.
// Returns a slice of getIpAddress or an error if the operation fails.
func (st *State) getAllAddresses(ctx context.Context, tx *sqlair.TX) ([]getIpAddress, error) {
	stmt, err := st.Prepare(`
SELECT 
 ia.uuid AS &getIpAddress.uuid,
 ia.net_node_uuid AS &getIpAddress.net_node_uuid,
 pia.provider_id AS &getIpAddress.provider_id,
 ps.provider_id AS &getIpAddress.provider_subnet_id,
 ia.device_uuid AS &getIpAddress.device_uuid,
 ia.address_value AS &getIpAddress.address_value,
 s.name AS &getIpAddress.space,
 iat.name AS &getIpAddress.type,
 iact.name AS &getIpAddress.config_type,
 iao.name AS &getIpAddress.origin,
 ias.name AS &getIpAddress.scope,
 ia.is_secondary AS &getIpAddress.is_secondary,
 ia.is_shadow AS &getIpAddress.is_shadow
FROM ip_address AS ia
JOIN ip_address_type AS iat ON ia.type_id = iat.id
JOIN ip_address_config_type AS iact ON ia.config_type_id = iact.id
JOIN ip_address_origin AS iao ON ia.origin_id = iao.id
JOIN ip_address_scope AS ias ON ia.scope_id = ias.id
LEFT JOIN provider_ip_address AS pia ON ia.uuid = pia.address_uuid
LEFT JOIN provider_subnet as ps ON ia.subnet_uuid = ps.subnet_uuid
LEFT JOIN subnet as sub ON ia.subnet_uuid = sub.uuid
LEFT JOIN space as s ON sub.space_uuid = s.uuid
`, getIpAddress{})
	if err != nil {
		return nil, errors.Errorf("preparing IP address select statement: %w", err)
	}

	var addresses []getIpAddress
	err = tx.Query(ctx, stmt).GetAll(&addresses)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("querying IP addresses: %w", err)
	}
	return addresses, nil
}

func (st *State) getCurrentDevices(ctx context.Context, tx *sqlair.TX, nodeUUID string) (map[string]string, error) {
	nUUID := entityUUID{UUID: nodeUUID}

	qry := "SELECT &linkLayerDeviceName.* FROM link_layer_device WHERE net_node_uuid = $entityUUID.uuid"
	stmt, err := st.Prepare(qry, nUUID, linkLayerDeviceName{})
	if err != nil {
		return nil, errors.Errorf("preparing current devices statement: %w", err)
	}

	var devs []linkLayerDeviceName
	if err := tx.Query(ctx, stmt, nUUID).GetAll(&devs); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Errorf("running current devices query: %w", err)
	}

	return transform.SliceToMap(devs, func(d linkLayerDeviceName) (string, string) {
		return d.Name, d.UUID
	}), nil
}

func (st *State) upsertLinkLayerDevices(ctx context.Context, tx *sqlair.TX, devs []linkLayerDeviceDML) error {
	dml := `
INSERT INTO link_layer_device (*) VALUES ($linkLayerDeviceDML.*)
ON CONFLICT (uuid) DO UPDATE SET
    device_type_id = EXCLUDED.device_type_id,
	mac_address = EXCLUDED.mac_address,
    mtu = EXCLUDED.mtu,
    gateway_address = EXCLUDED.gateway_address,
    is_default_gateway = EXCLUDED.is_default_gateway,
    is_auto_start = EXCLUDED.is_auto_start,
    is_enabled = EXCLUDED.is_enabled,
    virtual_port_type_id = EXCLUDED.virtual_port_type_id,
    vlan_tag = EXCLUDED.vlan_tag`

	stmt, err := st.Prepare(dml, devs[0])
	if err != nil {
		return errors.Errorf("preparing device insert statement: %w", err)
	}

	err = tx.Query(ctx, stmt, devs).Run()
	if err != nil {
		return errors.Errorf("running device insert statement: %w", err)
	}

	return nil
}
