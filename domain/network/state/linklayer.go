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
	db, err := st.DB()
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

// SetMachineNetConfig updates the network configuration for the machine with
// the input net node UUID.
//   - New devices and their addresses are insert with origin = "machine".
//   - Existing devices and addresses are updated (unchanged rows will not cause
//     triggers to fire).
//   - Existing addresses not in the input will be deleted if origin == "machine"
//   - Devices not observed will be deleted if they have no addresses.
func (st *State) SetMachineNetConfig(ctx context.Context, nodeUUID string, nics []network.NetInterface) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	nUUID := entityUUID{UUID: nodeUUID}

	// TODO (manadart 2025-04-29): This is temporary and serves to get us
	// operational with addresses on Dqlite.
	// We will set devices and addresses for any given machine *one time*
	// with subsequent updates being a no-op until we add the full
	// reconciliation logic.
	var devCount countResult
	devCountSql := "SELECT COUNT(*) AS &countResult.count FROM link_layer_device WHERE net_node_uuid = $entityUUID.uuid"
	devCountStmt, err := st.Prepare(devCountSql, nUUID, devCount)
	if err != nil {
		return errors.Errorf("preparing device count statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, devCountStmt, nUUID).Get(&devCount); err != nil {
			return errors.Errorf("running device count statement: %w", err)
		}

		// If we've done it for this machine before, bug out.
		if devCount.Count > 0 {
			return nil
		}

		// Otherwise, insert the data.
		newNics, dnsSearchDoms, dnsAddrs, nicNameToUUID, err := st.reconcileNetConfigDevices(nodeUUID, nics)
		if err != nil {
			return errors.Errorf("reconciling incoming network devices: %w", err)
		}

		// retainedDeviceUUIDs represent the set of link-layer device UUIDs
		// that we know will be in the data once we complete this operation.
		// I.e. those inserted and updated.
		retainedDeviceUUIDs := transform.MapToSlice(nicNameToUUID, func(k, v string) []string { return []string{v} })

		if err = st.insertLinkLayerDevices(ctx, tx, newNics); err != nil {
			return errors.Errorf("inserting link layer devices: %w", err)
		}

		if err = st.updateDNSSearchDomains(ctx, tx, dnsSearchDoms, retainedDeviceUUIDs); err != nil {
			return errors.Errorf("updating DNS search domains: %w", err)
		}

		if err = st.updateDNSAddresses(ctx, tx, dnsAddrs, retainedDeviceUUIDs); err != nil {
			return errors.Errorf("updating DNS addresses: %w", err)
		}

		addrsToInsert, err := st.reconcileNetConfigAddresses(nodeUUID, nics, nicNameToUUID)
		if err != nil {
			return errors.Errorf("reconciling incoming ip addresses: %w", err)
		}

		if len(addrsToInsert) == 0 {
			return nil
		}

		if err = st.insertIPAddresses(ctx, tx, addrsToInsert); err != nil {
			return errors.Errorf("inserting IP addresses: %w", err)
		}

		return nil
	})

	return errors.Capture(err)
}

// GetAllLinkLayerDevicesByNetNodeUUIDs retrieves all link-layer devices grouped
// by their associated NetNodeUUIDs.
// The function fetches link-layer devices, DNS domains, DNS addresses,
// and IP addresses, then maps them accordingly.
// Returns a map of NetNodeUUID to a slice of NetInterface and an error if
// any operation fails during execution.
func (st *State) GetAllLinkLayerDevicesByNetNodeUUIDs(ctx context.Context) (map[string][]network.NetInterface, error) {
	db, err := st.DB()
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
		result, err := dmlToNetInterface(in,
			dnsDomainByDeviceUUID[in.UUID],
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
 ia.subnet_uuid AS &getIpAddress.subnet_uuid,
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

func (st *State) reconcileNetConfigDevices(
	nodeUUID string, nics []network.NetInterface,
) ([]linkLayerDeviceDML, []dnsSearchDomainRow, []dnsAddressRow, map[string]string, error) {
	// TODO (manadart 2025-04-30): This will have to return more types for
	// provider ID entries etc.

	// The idea here will be to set the UUIDs that we know from querying
	// existing devices, then generate new ones for devices we don't have yet.
	// Interfaces that we do not observe will be deleted along with their
	// addresses if they have an origin of "machine".
	nameToUUID := make(map[string]string, len(nics))
	for _, n := range nics {
		nicUUID, err := network.NewInterfaceUUID()
		if err != nil {
			return nil, nil, nil, nil, errors.Capture(err)
		}
		nameToUUID[n.Name] = nicUUID.String()
	}

	nicsDML := make([]linkLayerDeviceDML, len(nics))
	var (
		dnsSearchDMLs []dnsSearchDomainRow
		dnsAddrDMLs   []dnsAddressRow
	)

	for i, n := range nics {
		nicDML, dnsSearchDML, dnsAddrDML, err := netInterfaceToDML(n, nodeUUID, nameToUUID)
		if err != nil {
			return nil, nil, nil, nil, errors.Capture(err)
		}

		nicsDML[i] = nicDML
		dnsSearchDMLs = append(dnsSearchDMLs, dnsSearchDML...)
		dnsAddrDMLs = append(dnsAddrDMLs, dnsAddrDML...)
	}

	return nicsDML, dnsSearchDMLs, dnsAddrDMLs, nameToUUID, nil
}

func (st *State) insertLinkLayerDevices(ctx context.Context, tx *sqlair.TX, devs []linkLayerDeviceDML) error {
	stmt, err := st.Prepare(
		"INSERT INTO link_layer_device (*) VALUES ($linkLayerDeviceDML.*)", devs[0])
	if err != nil {
		return errors.Errorf("preparing device insert statement: %w", err)
	}

	err = tx.Query(ctx, stmt, devs).Run()
	if err != nil {
		return errors.Errorf("running device insert statement: %w", err)
	}

	return nil
}

// updateDNSSearchDomains replaces all the data in link_layer_device_dns_domain
// for devices represented in the incoming data.
// We are unworried by the wholesale replacement because:
// - Machine network config updates are infrequent.
// - We are not watching this data. We mostly care about changing addresses.
func (st *State) updateDNSSearchDomains(
	ctx context.Context, tx *sqlair.TX, rows []dnsSearchDomainRow, devs uuids,
) error {
	stmt, err := st.Prepare("DELETE FROM link_layer_device_dns_domain WHERE device_uuid IN ($uuids[:])", devs)
	if err != nil {
		return errors.Errorf("preparing DNS search domain delete statement: %w", err)
	}

	if err := tx.Query(ctx, stmt, devs).Run(); err != nil {
		return errors.Errorf("running DNS search domain delete statement: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	stmt, err = st.Prepare("INSERT INTO link_layer_device_dns_domain(*) VALUES ($dnsSearchDomainRow.*)", rows[0])
	if err != nil {
		return errors.Errorf("preparing DNS search domain insert statement: %w", err)
	}

	if err := tx.Query(ctx, stmt, rows).Run(); err != nil {
		return errors.Errorf("running DNS search domain insert statement: %w", err)
	}

	return nil
}

// updateDNSAddresses replaces all the data in the link_layer_device_dns_address
// for devices represented in the incoming data.
// See [updateDNSSearchDomains] for why delete-all+insert is OK.
func (st *State) updateDNSAddresses(ctx context.Context, tx *sqlair.TX, rows []dnsAddressRow, devs uuids) error {
	stmt, err := st.Prepare("DELETE FROM link_layer_device_dns_address WHERE device_uuid IN ($uuids[:])", devs)
	if err != nil {
		return errors.Errorf("preparing DNS address delete statement: %w", err)
	}

	if err := tx.Query(ctx, stmt, devs).Run(); err != nil {
		return errors.Errorf("running DNS address delete statement: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	stmt, err = st.Prepare("INSERT INTO link_layer_device_dns_address(*) VALUES ($dnsAddressRow.*)", rows[0])
	if err != nil {
		return errors.Errorf("preparing DNS address insert statement: %w", err)
	}

	if err := tx.Query(ctx, stmt, rows).Run(); err != nil {
		return errors.Errorf("running DNS address insert statement: %w", err)
	}

	return nil
}

func (st *State) reconcileNetConfigAddresses(
	nodeUUID string, nics []network.NetInterface, nicNameToUUID map[string]string,
) ([]ipAddressDML, error) {
	var addrsDML []ipAddressDML

	for _, n := range nics {
		// If we don't know this NIC, we can assume that is a deletion candidate.
		devUUID, ok := nicNameToUUID[n.Name]
		if !ok {
			continue
		}

		// As with the interfaces, this will really be formed by querying the
		// existing addresses in addition to creating UUIDs for new ones.
		addrToUUID := make(map[string]string, len(n.Addrs))
		for _, a := range n.Addrs {
			addrUUID, err := network.NewAddressUUID()
			if err != nil {
				return nil, errors.Capture(err)
			}
			addrToUUID[a.AddressValue] = addrUUID.String()
		}

		for _, a := range n.Addrs {
			addrDML, err := netAddrToDML(a, nodeUUID, devUUID, addrToUUID)
			if err != nil {
				return nil, errors.Capture(err)
			}

			addrsDML = append(addrsDML, addrDML)
		}
	}

	return addrsDML, nil
}

func (st *State) insertIPAddresses(ctx context.Context, tx *sqlair.TX, addrs []ipAddressDML) error {
	st.logger.Tracef(ctx, "inserting IP addresses %#v", addrs)

	stmt, err := st.Prepare(
		"INSERT INTO ip_address (*) VALUES ($ipAddressDML.*)", addrs[0])
	if err != nil {
		return errors.Errorf("preparing address insert statement: %w", err)
	}

	err = tx.Query(ctx, stmt, addrs).Run()
	if err != nil {
		return errors.Errorf("running address insert statement: %w", err)
	}

	return nil
}
