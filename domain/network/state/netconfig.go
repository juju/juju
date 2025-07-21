// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"net"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// SetMachineNetConfig updates the network configuration for the machine with
// the input net node UUID.
//   - New devices and their addresses are inserted with origin = "machine".
//   - Existing devices and addresses are updated (unchanged rows will not cause
//     triggers to fire).
//   - Existing addresses not in the input will be deleted
//     if origin == "machine"
//   - Devices not observed will be deleted if they have no addresses and no
//     provider ID.
//
// Addresses are associated with a subnet according to the following logic:
//   - If the address is determined to be in a subnet with a unique CIDR,
//     it is inserted with that subnet UUID.
//   - If the address cannot be unambiguously associated with a subnet,
//     i.e. if there are multiple subnets with the same CIDR, a new /32 (IPv4)
//     or /128 (IPv6) subnet is created for it and the address is inserted with
//     that subnet UUID. The instance-poller reconciliation will match it based
//     on provider subnet ID if it can; see [SetProviderNetConfig].
func (st *State) SetMachineNetConfig(ctx context.Context, nodeUUID string, nics []network.NetInterface) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		lookups, err := st.getNetConfigLookups(ctx, tx)
		if err != nil {
			return errors.Capture(err)
		}

		newNics, dnsDoms, dnsAddrs, parents, nicNameToUUID, err :=
			st.reconcileNetConfigDevices(ctx, tx, nodeUUID, nics, lookups)
		if err != nil {
			return errors.Errorf("reconciling incoming network devices: %w", err)
		}

		// retainedDeviceUUIDs represent the set of link-layer device UUIDs
		// that we know will be in the data once we complete this operation.
		// I.e. those inserted and updated.
		retainedDeviceUUIDs := transform.MapToSlice(nicNameToUUID, func(k, v string) []string { return []string{v} })

		if err := st.updateNetworkDevices(ctx, tx, newNics, dnsDoms, dnsAddrs, parents, retainedDeviceUUIDs); err != nil {
			return errors.Capture(err)
		}

		subs, err := st.getSubnetGroups(ctx, tx)
		if err != nil {
			return errors.Errorf("getting subnet groups: %w", err)
		}
		st.logger.Debugf(ctx, "matching with subnet groups: %#v", subs)

		addrsToInsert, newSubs, err :=
			st.reconcileNetConfigAddresses(ctx, tx, nodeUUID, nics, nicNameToUUID, subs, lookups)
		if err != nil {
			return errors.Errorf("reconciling incoming ip addresses: %w", err)
		}

		if len(addrsToInsert) != 0 {
			if err := st.insertSubnets(ctx, tx, newSubs); err != nil {
				return errors.Errorf("inserting subnets: %w", err)
			}

			if err = st.upsertIPAddresses(ctx, tx, addrsToInsert); err != nil {
				return errors.Errorf("inserting IP addresses: %w", err)
			}
		}

		if err := st.deleteAddressesNotObserved(ctx, tx, nodeUUID, addrsToInsert); err != nil {
			return errors.Errorf("deleting addresses: %w", err)
		}

		if err := st.deleteDevicesNotObserved(ctx, tx, nodeUUID, retainedDeviceUUIDs); err != nil {
			return errors.Errorf("deleting devices: %w", err)
		}

		return nil
	})

	return errors.Capture(err)
}

func (st *State) reconcileNetConfigDevices(
	ctx context.Context, tx *sqlair.TX, nodeUUID string, nics []network.NetInterface, lookups netConfigLookups,
) ([]linkLayerDeviceDML, []dnsSearchDomainRow, []dnsAddressRow, []linkLayerDeviceParent, map[string]string, error) {
	// Determine all the known UUIDs for incoming devices,
	// and generate new UUIDs for the others.
	existing, err := st.getCurrentDevices(ctx, tx, nodeUUID)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.Capture(err)
	}

	nameToUUID := make(map[string]string, len(nics))
	for _, n := range nics {
		if existingUUID, ok := existing[n.Name]; ok {
			nameToUUID[n.Name] = existingUUID
			continue
		}

		nicUUID, err := network.NewInterfaceUUID()
		if err != nil {
			return nil, nil, nil, nil, nil, errors.Capture(err)
		}
		nameToUUID[n.Name] = nicUUID.String()
	}

	nicsDML := make([]linkLayerDeviceDML, len(nics))
	var (
		dnsSearchDMLs []dnsSearchDomainRow
		dnsAddrDMLs   []dnsAddressRow
	)

	for i, n := range nics {
		nicDML, dnsSearchDML, dnsAddrDML, err := netInterfaceToDML(n, nodeUUID, nameToUUID, lookups)
		if err != nil {
			return nil, nil, nil, nil, nil, errors.Capture(err)
		}

		nicsDML[i] = nicDML
		dnsSearchDMLs = append(dnsSearchDMLs, dnsSearchDML...)
		dnsAddrDMLs = append(dnsAddrDMLs, dnsAddrDML...)
	}

	// Use the nameToUUID map to populate device parents.
	var parentDMLs []linkLayerDeviceParent
	for _, n := range nics {
		if n.ParentDeviceName == "" {
			continue
		}

		if parentUUID, ok := nameToUUID[n.ParentDeviceName]; ok {
			parentDMLs = append(parentDMLs, linkLayerDeviceParent{
				DeviceUUID: nameToUUID[n.Name],
				ParentUUID: parentUUID,
			})
		} else if parentUUID, ok := existing[n.ParentDeviceName]; ok {
			parentDMLs = append(parentDMLs, linkLayerDeviceParent{
				DeviceUUID: nameToUUID[n.Name],
				ParentUUID: parentUUID,
			})
		} else {
			st.logger.Warningf(context.TODO(), "parent device %q for %q not found in incoming or existing data",
				n.ParentDeviceName, n.Name)
		}
	}

	return nicsDML, dnsSearchDMLs, dnsAddrDMLs, parentDMLs, nameToUUID, nil
}

func (st *State) updateNetworkDevices(
	ctx context.Context,
	tx *sqlair.TX,
	newNics []linkLayerDeviceDML,
	dnsDoms []dnsSearchDomainRow,
	dnsAddrs []dnsAddressRow,
	parents []linkLayerDeviceParent,
	retainedDeviceUUIDs []string,
) error {
	if err := st.upsertLinkLayerDevices(ctx, tx, newNics); err != nil {
		return errors.Errorf("inserting link layer devices: %w", err)
	}

	if err := st.updateDNSSearchDomains(ctx, tx, dnsDoms, retainedDeviceUUIDs); err != nil {
		return errors.Errorf("updating DNS search domains: %w", err)
	}

	if err := st.updateDNSAddresses(ctx, tx, dnsAddrs, retainedDeviceUUIDs); err != nil {
		return errors.Errorf("updating DNS addresses: %w", err)
	}

	if err := st.updateDeviceParents(ctx, tx, parents, retainedDeviceUUIDs); err != nil {
		return errors.Errorf("inserting device parents: %w", err)
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

func (st *State) updateDeviceParents(
	ctx context.Context, tx *sqlair.TX, parents []linkLayerDeviceParent, devs uuids,
) error {
	stmt, err := st.Prepare("DELETE FROM link_layer_device_parent WHERE device_uuid IN ($uuids[:])", devs)
	if err != nil {
		return errors.Errorf("preparing device parent delete statement: %w", err)
	}

	if err := tx.Query(ctx, stmt, devs).Run(); err != nil {
		return errors.Errorf("running device parent delete statement: %w", err)
	}

	if len(parents) == 0 {
		return nil
	}

	stmt, err = st.Prepare("INSERT INTO link_layer_device_parent (*) VALUES ($linkLayerDeviceParent.*)", parents[0])
	if err != nil {
		return errors.Errorf("preparing device parent insert statement: %w", err)
	}

	if err := tx.Query(ctx, stmt, parents).Run(); err != nil {
		return errors.Errorf("running device parent insert statement: %w", err)
	}

	return nil
}

// getSubnetGroups retrieves all subnets, parses then into net.IPNet,
// and groups the UUIDs by CIDR.
func (st *State) getSubnetGroups(ctx context.Context, tx *sqlair.TX) (subnetGroups, error) {
	stmt, err := st.Prepare("SELECT &subnet.* FROM subnet", subnet{})
	if err != nil {
		return nil, errors.Errorf("preparing subnets statement: %w", err)
	}

	var subs []subnet
	if err := tx.Query(ctx, stmt).GetAll(&subs); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Errorf("running subnets query: %w", err)
	}

	results := make(map[string]subnetGroup)
	for _, sub := range subs {
		res, ok := results[sub.CIDR]
		if !ok {
			_, ipNet, err := net.ParseCIDR(sub.CIDR)
			if err != nil {
				return nil, errors.Capture(err)
			}
			results[sub.CIDR] = subnetGroup{
				ipNet: *ipNet,
				uuids: []string{sub.UUID},
			}
			continue
		}

		// This represents a multi-net case, which is possible on OpenStack
		// where multiple provider networks can contain the same CIDR.
		// At the time of writing it has never been observed in practice,
		// but our modelling allows it.
		res.uuids = append(results[sub.CIDR].uuids, sub.UUID)
		results[sub.CIDR] = res
	}

	return transform.MapToSlice(results, func(cidr string, s subnetGroup) []subnetGroup {
		return subnetGroups{s}
	}), nil
}

func (st *State) reconcileNetConfigAddresses(
	ctx context.Context,
	tx *sqlair.TX,
	nodeUUID string,
	nics []network.NetInterface,
	nicNameToUUID map[string]string,
	subs subnetGroups,
	lookups netConfigLookups,
) ([]ipAddressDML, []subnet, error) {
	var (
		addrsDML []ipAddressDML
		newSubs  []subnet
	)

	// Determine all the known UUIDs for incoming addresses,
	// and generate new UUIDs for the others.
	existingAddrs, err := st.getCurrentAddresses(ctx, tx, nodeUUID)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	for _, n := range nics {
		devUUID := nicNameToUUID[n.Name]

		addrToUUID := make(map[string]string, len(n.Addrs))
		for _, a := range n.Addrs {
			if existingAddr, ok := existingAddrs[a.AddressValue]; ok {
				addrToUUID[a.AddressValue] = existingAddr.UUID
				continue
			}

			addrUUID, err := network.NewAddressUUID()
			if err != nil {
				return nil, nil, errors.Capture(err)
			}
			addrToUUID[a.AddressValue] = addrUUID.String()
		}

		for _, a := range n.Addrs {
			existingAddr := existingAddrs[a.AddressValue]

			addrDML, err := netAddrToDML(a, nodeUUID, devUUID, addrToUUID, lookups)
			if err != nil {
				return nil, nil, errors.Capture(err)
			}

			// If the address already has a subnet UUID, we use it.
			// Otherwise, we try to find a subnet by locating an existing subnet
			// that contains it.
			// If we cannot find a *unique* subnet for the address,
			// we create a new /32 or /128 subnet in the alpha space and
			// link this address to it.
			// The instance-poller will attempt to reconcile the real subnet
			// using the provider subnet ID subsequently.
			var subnetUUID string
			if existingAddr.SubnetUUID.Valid && existingAddr.SubnetUUID.String != "" {
				subnetUUID = existingAddr.SubnetUUID.String
			} else {
				subnetUUID, err = subs.subnetForIP(a.AddressValue)
				if err != nil {
					// TODO (manadart 2025-04-29): Figure out what to do with
					// loopback addresses before making
					// ip_address.subnet_uuid NOT NULL.
					st.logger.Warningf(ctx, "determining subnet: %v", err)
				}
			}

			if subnetUUID != "" {
				addrDML.SubnetUUID = &subnetUUID
			} else if err == nil {
				ip, _, _ := net.ParseCIDR(a.AddressValue)
				if ip == nil {
					return nil, nil, errors.Errorf("invalid IP address %q", a.AddressValue)
				}

				suffix := "/32"
				if a.AddressType == corenetwork.IPv6Address {
					suffix = "/128"
				}

				sUUID, err := network.NewSubnetUUID()
				if err != nil {
					return nil, nil, errors.Capture(err)
				}
				newSubUUID := sUUID.String()

				newSub := subnet{
					UUID:      newSubUUID,
					CIDR:      ip.String() + suffix,
					SpaceUUID: corenetwork.AlphaSpaceId,
				}
				newSubs = append(newSubs, newSub)
				addrDML.SubnetUUID = &newSubUUID
			}

			addrsDML = append(addrsDML, addrDML)
		}
	}

	return addrsDML, newSubs, nil
}

func (st *State) getCurrentAddresses(
	ctx context.Context, tx *sqlair.TX, nodeUUID string,
) (map[string]ipAddressValue, error) {
	nUUID := entityUUID{UUID: nodeUUID}

	qry := "SELECT &ipAddressValue.* FROM ip_address WHERE net_node_uuid = $entityUUID.uuid"
	stmt, err := st.Prepare(qry, nUUID, ipAddressValue{})
	if err != nil {
		return nil, errors.Errorf("preparing current addresses statement: %w", err)
	}

	var addrs []ipAddressValue
	if err := tx.Query(ctx, stmt, nUUID).GetAll(&addrs); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Errorf("running current devices query: %w", err)
	}

	return transform.SliceToMap(addrs, func(d ipAddressValue) (string, ipAddressValue) {
		return d.Value, d
	}), nil
}

func (st *State) insertSubnets(ctx context.Context, tx *sqlair.TX, subs []subnet) error {
	if len(subs) == 0 {
		return nil
	}

	st.logger.Debugf(ctx, "inserting new subnets %#v", subs)

	stmt, err := st.Prepare("INSERT INTO subnet (*) VALUES ($subnet.*)", subs[0])
	if err != nil {
		return errors.Errorf("preparing subnet insert statement: %w", err)
	}

	err = tx.Query(ctx, stmt, subs).Run()
	if err != nil {
		return errors.Errorf("running subnet insert statement: %w", err)
	}

	return nil
}

func (st *State) upsertIPAddresses(ctx context.Context, tx *sqlair.TX, addrs []ipAddressDML) error {
	// This guard is present in SetMachineNetConfig, but we play it safe.
	if len(addrs) == 0 {
		return nil
	}

	st.logger.Debugf(ctx, "updating IP addresses %#v", addrs)

	dml := `
INSERT INTO ip_address (*) VALUES ($ipAddressDML.*)
ON CONFLICT (uuid) DO UPDATE SET
    device_uuid = EXCLUDED.device_uuid,
    address_value = EXCLUDED.address_value,
    config_type_id = EXCLUDED.config_type_id,
    type_id = EXCLUDED.type_id,
    subnet_uuid = EXCLUDED.subnet_uuid,
    scope_id = EXCLUDED.scope_id,
    is_secondary = EXCLUDED.is_secondary,
    is_shadow = EXCLUDED.is_shadow`

	stmt, err := st.Prepare(dml, addrs[0])
	if err != nil {
		return errors.Errorf("preparing address insert statement: %w", err)
	}

	err = tx.Query(ctx, stmt, addrs).Run()
	if err != nil {
		return errors.Errorf("running address insert statement: %w", err)
	}

	return nil
}

// deleteAddressesNotObserved deletes addresses under the remit
// of the machine that were not observed in the incoming data.
func (st *State) deleteAddressesNotObserved(
	ctx context.Context, tx *sqlair.TX, nodeUUID string, retainedAddresses []ipAddressDML,
) error {
	nUUID := entityUUID{UUID: nodeUUID}
	retainedAddressUUIDs := uuids(transform.Slice(retainedAddresses, func(a ipAddressDML) string { return a.UUID }))

	addrDML := `
DELETE FROM ip_address 
WHERE  net_node_uuid = $entityUUID.uuid
AND    uuid NOT IN ($uuids[:])
AND    origin_id = 0`

	addrStmt, err := st.Prepare(addrDML, nUUID, retainedAddressUUIDs)
	if err != nil {
		return errors.Errorf("preparing address deletion statement: %w", err)
	}

	if err := tx.Query(ctx, addrStmt, nUUID, retainedAddressUUIDs).Run(); err != nil {
		return errors.Errorf("running address deletion statement: %w", err)
	}

	return nil
}

// deleteDevicesNotObserved deletes devices not observed in the incoming data,
// provided they have no addresses and no provider ID, and are not in
// parent-child relationships that would result in orphans.
func (st *State) deleteDevicesNotObserved(
	ctx context.Context, tx *sqlair.TX, nodeUUID string, retainedDeviceUUIDs []string,
) error {
	nUUID := entityUUID{UUID: nodeUUID}
	retainedDevices := uuids(retainedDeviceUUIDs)

	devSQL := `
SELECT d.uuid AS &entityUUID.uuid
FROM   link_layer_device d
       LEFT JOIN provider_link_layer_device p ON d.uuid = p.device_uuid
       LEFT JOIN ip_address a ON d.uuid = a.device_uuid
WHERE  d.net_node_uuid = $entityUUID.uuid
AND    p.device_uuid IS NULL
AND    a.device_uuid IS NULL
AND    d.uuid NOT IN ($uuids[:])`

	devStmt, err := st.Prepare(devSQL, nUUID, retainedDevices)
	if err != nil {
		return errors.Errorf("preparing devices for deletion statement: %w", err)
	}

	var devsToDelete []entityUUID
	if err := tx.Query(ctx, devStmt, nUUID, retainedDevices).GetAll(&devsToDelete); err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running devices for deletion query: %w", err)
		}
	}

	if len(devsToDelete) == 0 {
		return nil
	}

	deletionCandidates := transform.Slice(devsToDelete, func(d entityUUID) string { return d.UUID })

	// We need to ensure that we delete parent devices only if:
	// - they have no children or;
	// - their child devices are also being deleted.
	parentSQL := `
SELECT &linkLayerDeviceParent.*
FROM   link_layer_device_parent
WHERE  parent_uuid IN ($uuids[:])`

	parentStmt, err := st.Prepare(parentSQL, linkLayerDeviceParent{}, uuids(deletionCandidates))
	if err != nil {
		return errors.Errorf("preparing child devices statement: %w", err)
	}

	var parents []linkLayerDeviceParent
	if err := tx.Query(ctx, parentStmt, uuids(deletionCandidates)).GetAll(&parents); err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running child devices query: %w", err)
		}
	}

	deletions := set.NewStrings(deletionCandidates...)
	for _, p := range parents {
		if !deletions.Contains(p.DeviceUUID) {
			st.logger.Debugf(ctx, "not removing device %q as it is a parent of existing device %q",
				p.ParentUUID, p.DeviceUUID)

			deletions.Remove(p.ParentUUID)
		}
	}

	if len(deletions) == 0 {
		return nil
	}

	delArg := uuids(deletions.Values())
	for _, delSQL := range []string{
		"DELETE FROM link_layer_device_parent WHERE device_uuid IN ($uuids[:])",
		"DELETE FROM link_layer_device_parent WHERE parent_uuid IN ($uuids[:])",
		"DELETE FROM link_layer_device_dns_domain WHERE device_uuid IN ($uuids[:])",
		"DELETE FROM link_layer_device_dns_address WHERE device_uuid IN ($uuids[:])",
		"DELETE FROM link_layer_device WHERE uuid IN ($uuids[:])",
	} {
		delStmt, err := st.Prepare(delSQL, delArg)
		if err != nil {
			return errors.Errorf("preparing device deletion statement: %w", err)
		}

		if err := tx.Query(ctx, delStmt, delArg).Run(); err != nil {
			return errors.Errorf("running device deletion statement: %w", err)
		}
	}

	return nil
}
