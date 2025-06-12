// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"net"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/ipaddress"
	domainlife "github.com/juju/juju/domain/life"
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
		newNics, dnsSearchDoms, dnsAddrs, parents, nicNameToUUID, err := st.reconcileNetConfigDevices(nodeUUID, nics)
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

		if err = st.insertDeviceParents(ctx, tx, parents); err != nil {
			return errors.Errorf("inserting device parents: %w", err)
		}

		subs, err := st.getSubnetGroups(ctx, tx)
		if err != nil {
			return errors.Errorf("getting subnet groups: %w", err)
		}
		st.logger.Debugf(ctx, "matching with subnet groups: %#v", subs)

		addrsToInsert, newSubs, err := st.reconcileNetConfigAddresses(ctx, nodeUUID, nics, nicNameToUUID, subs)
		if err != nil {
			return errors.Errorf("reconciling incoming ip addresses: %w", err)
		}

		if len(addrsToInsert) == 0 {
			return nil
		}

		if err := st.insertSubnets(ctx, tx, newSubs); err != nil {
			return errors.Errorf("inserting subnets: %w", err)
		}

		if err = st.insertIPAddresses(ctx, tx, addrsToInsert); err != nil {
			return errors.Errorf("inserting IP addresses: %w", err)
		}

		return nil
	})

	return errors.Capture(err)
}

func (st *State) reconcileNetConfigDevices(
	nodeUUID string, nics []network.NetInterface,
) ([]linkLayerDeviceDML, []dnsSearchDomainRow, []dnsAddressRow, []deviceParent, map[string]string, error) {
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
		nicDML, dnsSearchDML, dnsAddrDML, err := netInterfaceToDML(n, nodeUUID, nameToUUID)
		if err != nil {
			return nil, nil, nil, nil, nil, errors.Capture(err)
		}

		nicsDML[i] = nicDML
		dnsSearchDMLs = append(dnsSearchDMLs, dnsSearchDML...)
		dnsAddrDMLs = append(dnsAddrDMLs, dnsAddrDML...)
	}

	// Use the nameToUUID map to populate device parents.
	var parentDMLs []deviceParent
	for _, n := range nics {
		if n.ParentDeviceName == "" {
			continue
		}

		if parentUUID, ok := nameToUUID[n.ParentDeviceName]; ok {
			parentDMLs = append(parentDMLs, deviceParent{
				DeviceUUID: nameToUUID[n.Name],
				ParentUUID: parentUUID,
			})
		} else {
			st.logger.Warningf(context.TODO(), "parent device %q for %q not found in incoming data",
				n.ParentDeviceName, n.Name)
		}
	}

	return nicsDML, dnsSearchDMLs, dnsAddrDMLs, parentDMLs, nameToUUID, nil
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

func (st *State) insertDeviceParents(ctx context.Context, tx *sqlair.TX, parents []deviceParent) error {
	if len(parents) == 0 {
		return nil
	}

	stmt, err := st.Prepare("INSERT INTO link_layer_device_parent (*) VALUES ($deviceParent.*)", parents[0])
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
	nodeUUID string,
	nics []network.NetInterface,
	nicNameToUUID map[string]string,
	subs subnetGroups,
) ([]ipAddressDML, []subnet, error) {
	var (
		addrsDML []ipAddressDML
		newSubs  []subnet
	)

	for _, n := range nics {
		// If we don't know this NIC, we can assume that it is a deletion candidate.
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
				return nil, nil, errors.Capture(err)
			}
			addrToUUID[a.AddressValue] = addrUUID.String()
		}

		for _, a := range n.Addrs {
			addrDML, err := netAddrToDML(a, nodeUUID, devUUID, addrToUUID)
			if err != nil {
				return nil, nil, errors.Capture(err)
			}

			subnetUUID, err := subs.subnetForIP(a.AddressValue)
			if err != nil {
				// TODO (manadart 2025-04-29): Figure out what to do with
				// loopback addresses before making
				// ip_address.subnet_uuid NOT NULL.
				st.logger.Warningf(ctx, "determining subnet: %v", err)
			}

			if subnetUUID != "" {
				addrDML.SubnetUUID = &subnetUUID
			} else if err == nil {
				// If we cannot find a *unique* subnet for the address,
				// we create a new /32 or /128 subnet in the alpha space and
				// link this address to it.
				// The instance-poller will attempt to reconcile the real subnet
				// using the provider subnet ID subsequently.
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

func (st *State) insertIPAddresses(ctx context.Context, tx *sqlair.TX, addrs []ipAddressDML) error {
	// This guard is present in SetMachineNetConfig, but we play it safe.
	if len(addrs) == 0 {
		return nil
	}

	st.logger.Debugf(ctx, "inserting IP addresses %#v", addrs)

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

// GetUnitAndK8sServiceAddresses returns the addresses of the specified unit.
// The addresses are taken by unioning the net node UUIDs of the cloud service
// (if any) and the net node UUIDs of the unit, where each net node has an
// associated address.
// This apprach allows us to get the addresses regardless of the substrate
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
	ident := unitUUID{UnitUUID: uuid}
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
JOIN      ip_address AS ip ON ip.device_uuid = lld.uuid
LEFT JOIN subnet AS sn ON sn.uuid = ip.subnet_uuid
WHERE     n.uuid = $unitUUID.uuid
`, spaceAddress{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDead(ctx, tx, unitUUID{UnitUUID: uuid}); err != nil {
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
	ident := unitUUID{UnitUUID: uuid}
	queryUnitPublicAddressesStmt, err := st.Prepare(`
SELECT    &spaceAddress.*
FROM      unit u
JOIN      link_layer_device AS lld ON u.net_node_uuid = lld.net_node_uuid
JOIN      ip_address AS ip ON ip.device_uuid = lld.uuid
LEFT JOIN subnet AS sn ON ip.subnet_uuid = sn.uuid
WHERE     u.uuid = $unitUUID.uuid
`, spaceAddress{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkUnitNotDead(ctx, tx, unitUUID{UnitUUID: uuid}); err != nil {
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
SELECT &unitUUID.*
FROM   unit
WHERE  name = $unitName.name
`, unitUUID{}, unitName)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	unitUUID := unitUUID{}
	err = tx.Query(ctx, query, unitName).Get(&unitUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q not found", name).Add(applicationerrors.UnitNotFound)
	}
	return unitUUID.UnitUUID, errors.Capture(err)
}

// checkUnitNotDead checks if the unit exists and is not dead. It's possible to
// access alive and dying units, but not dead ones:
// - If the unit is not found, [applicationerrors.UnitNotFound] is returned.
// - If the unit is dead, [applicationerrors.UnitIsDead] is returned.
func (st *State) checkUnitNotDead(ctx context.Context, tx *sqlair.TX, ident unitUUID) error {
	query := `
SELECT &lifeID.*
FROM unit
WHERE uuid = $unitUUID.uuid;
`
	stmt, err := st.Prepare(query, ident, lifeID{})
	if err != nil {
		return errors.Errorf("preparing query for unit %q: %w", ident.UnitUUID, err)
	}

	var result lifeID
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("checking unit %q exists: %w", ident.UnitUUID, err)
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
		spaceUUID = address.SpaceUUID.V
	}
	// The saved address value is in the form 192.0.2.1/24,
	// parse the parts for the MachineAddress
	ipAddr, ipNet, err := net.ParseCIDR(address.Value)
	if err != nil {
		return corenetwork.SpaceAddress{}, err
	}
	cidr := ipNet.String()
	// Prefer the subnet cidr if one exists.
	if address.SubnetCIDR.Valid {
		cidr = address.SubnetCIDR.String
	}
	return corenetwork.SpaceAddress{
		SpaceID: spaceUUID,
		Origin:  ipaddress.UnMarshallOrigin(ipaddress.Origin(address.OriginID)),
		MachineAddress: corenetwork.MachineAddress{
			Value:      ipAddr.String(),
			CIDR:       cidr,
			Type:       ipaddress.UnMarshallAddressType(ipaddress.AddressType(address.TypeID)),
			Scope:      ipaddress.UnMarshallScope(ipaddress.Scope(address.ScopeID)),
			ConfigType: ipaddress.UnMarshallConfigType(ipaddress.ConfigType(address.ConfigTypeID)),
		},
	}, nil
}
