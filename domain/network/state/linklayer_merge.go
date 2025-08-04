// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"net"
	"slices"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// mergeLinkLayerDevice is used to merge the existing link layer devices with the
// incoming ones.
//
// It contains only the fields that are used to identify and merge the devices
type mergeLinkLayerDevice struct {
	UUID       string
	Name       string
	MACAddress string
	ProviderID string
	Type       corenetwork.LinkLayerDeviceType
	Addresses  []mergeAddress
}

// mergeAddress is used to merge the existing addresses with the incoming ones.
//
// It contains the fields that are used to identify and merge the addresses, as
// well as add any new addresses from the provider.
type mergeAddress struct {
	UUID             string
	Value            string
	ProviderID       string
	ProviderSubnetID string
	SubnetCIDR       string
	AddressType      corenetwork.AddressType
	ConfigType       corenetwork.AddressConfigType
	Origin           corenetwork.Origin
	Scope            corenetwork.Scope
	IsSecondary      bool
	IsShadow         bool
}

// mergeLinkLayerDevicesChanges contains the changes to be applied to the
// link layer devices.
type mergeLinkLayerDevicesChanges struct {
	// toAddOrUpdate maps provider IDs to LinkLayerDeviceUUIDs to be added or
	// updated in provider_link_layer_device.
	toAddOrUpdate map[string]string
	// deviceToRelinquish are the device UUIDs to remove from provider_link_layer_device
	deviceToRelinquish []string
	// addressToRelinquish is a list of AddressUUIDs linked to relinquished link layer
	// devices.
	addressToRelinquish []string
	// newDevices are the incoming devices that did not match any we already
	// have in state.
	newDevices []mergeLinkLayerDevice
}

// mergeAddressesChanges contains the changes to be applied to the
// addresses.
type mergeAddressesChanges struct {
	// providerIDsToAddOrUpdate maps provider IDs to ip_address UUID to be added or updated
	// in provider_link_layer_device.
	providerIDsToAddOrUpdate map[string]string
	// toRelinquish are a list of ip_address to
	// relinquish to machine, i.e., set their origin to machine
	// and remove from provider_ip_address
	toRelinquish []string

	// subnetToUpdate holds a list of merge address where the subnet needs to be
	// updated
	subnetToUpdate []mergeAddress

	// addressesToAdd is a map of device UUIDs of device to address
	addressesToAdd map[string][]mergeAddress
}

// MergeLinkLayerDevice is part of the [service.LinkLayerDeviceState]
// interface.
func (st *State) MergeLinkLayerDevice(
	ctx context.Context,
	netNodeUUID string,
	incoming []network.NetInterface,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(
		ctx, func(ctx context.Context, tx *sqlair.TX) error {
			existingDevices, err := st.getExistingLinkLayerDevicesWithAddresses(ctx, tx, netNodeUUID)
			if err != nil {
				return errors.Errorf("getting existing link layer devices for node %q: %w", netNodeUUID, err)
			}

			if len(existingDevices) == 0 {
				// Noop
				st.logger.Infof(ctx, "no existing devices, ignoring %d incoming device for net node %q",
					len(incoming), netNodeUUID)
				return nil
			}

			normalized, namelessHWAddrs, err := st.normalizeLinkLayerDevices(ctx, incoming, existingDevices)
			if err != nil {
				return errors.Capture(err)
			}

			lldChanges := st.computeMergeLinkLayerDeviceChanges(ctx, existingDevices, normalized, namelessHWAddrs)
			addressChanges := st.computeMergeAddressChanges(normalized, existingDevices)
			return st.applyMergeLinkLayerChanges(ctx, tx, lldChanges, addressChanges, netNodeUUID)
		},
	)
}

// addProviderLinkLayerDevice associates provider IDs with device UUIDs in the
// database.
// It inserts mappings from the input map into the provider_link_layer_device
// table.
// Returns an error if the database operation fails.
func (st *State) addProviderLinkLayerDevice(
	ctx context.Context, tx *sqlair.TX,
	providerIDToDeviceUUID map[string]string,
) error {
	type insert struct {
		ProviderID string `db:"provider_id"`
		DeviceUUID string `db:"device_uuid"`
	}
	stmt, err := st.Prepare(`
INSERT INTO provider_link_layer_device
VALUES ($insert.provider_id, $insert.device_uuid)
`, insert{})
	if err != nil {
		return errors.Capture(err)
	}
	for providerID, deviceUUID := range providerIDToDeviceUUID {
		insert := insert{
			ProviderID: providerID,
			DeviceUUID: deviceUUID,
		}
		if err := tx.Query(ctx, stmt, insert).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// addProviderAddress associates provider IDs with address UUIDs in the database.
// It inserts mappings from the input map into the provider_ip_address table.
// Returns an error if the database operation fails.
func (st *State) addProviderAddress(
	ctx context.Context, tx *sqlair.TX, add map[string]string,
) error {
	type insert struct {
		ProviderID  string `db:"provider_id"`
		AddressUUID string `db:"address_uuid"`
	}
	stmt, err := st.Prepare(
		`
INSERT INTO provider_ip_address
VALUES ($insert.provider_id, $insert.address_uuid)
`, insert{})
	if err != nil {
		return errors.Capture(err)
	}

	for providerID, addressUUID := range add {
		insert := insert{
			ProviderID:  providerID,
			AddressUUID: addressUUID,
		}
		if err := tx.Query(ctx, stmt, insert).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	// Update ip_address origin
	type uuids []string
	updateOriginStmt, err := st.Prepare(`
UPDATE ip_address 
SET origin_id = 1 -- set origin to provider
WHERE uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, updateOriginStmt, uuids(slices.Collect(maps.Values(add)))).Run()
}

// applyMergeLinkLayerChanges applies the changes to the link layer devices.
func (st *State) applyMergeLinkLayerChanges(
	ctx context.Context, tx *sqlair.TX,
	lldChanges mergeLinkLayerDevicesChanges,
	addressChanges mergeAddressesChanges,
	netNodeUUID string,
) error {
	getValue := func(_, value string) []string {
		return []string{value}
	}
	addressChanges.toRelinquish = append(addressChanges.toRelinquish, lldChanges.addressToRelinquish...)

	deviceToRemove := append(lldChanges.deviceToRelinquish, transform.MapToSlice(lldChanges.toAddOrUpdate, getValue)...)
	err := st.removeDeviceProviderIDs(ctx, tx, deviceToRemove)
	if err != nil {
		return errors.Errorf("removing provider IDs from link layer devices: %w", err)
	}
	addressesToRemove := append(addressChanges.toRelinquish, transform.MapToSlice(addressChanges.providerIDsToAddOrUpdate, getValue)...)
	err = st.removeAddressProviderIDs(ctx, tx, addressesToRemove)
	if err != nil {
		return errors.Errorf("removing provider IDs from addresses: %w", err)
	}
	err = st.addProviderLinkLayerDevice(ctx, tx, lldChanges.toAddOrUpdate)
	if err != nil {
		return errors.Errorf("adding provider IDs to link layer devices: %w", err)
	}
	err = st.addProviderAddress(ctx, tx, addressChanges.providerIDsToAddOrUpdate)
	if err != nil {
		return errors.Errorf("adding provider IDs to addresses: %w", err)
	}
	err = st.relinquishAddresses(ctx, tx, addressChanges.toRelinquish)
	if err != nil {
		return errors.Errorf("relinquishing addresses: %w", err)
	}
	err = st.addAddressFromProvider(ctx, tx, netNodeUUID, addressChanges.addressesToAdd)
	if err != nil {
		return errors.Errorf("adding addresses: %w", err)
	}

	// Process subnet updates
	err = st.updateSubnets(ctx, tx, addressChanges.subnetToUpdate)
	if err != nil {
		return errors.Errorf("updating subnets: %w", err)
	}

	// Remove subnets that are no longer needed
	if err := st.cleanupUniqueAddressOrphanSubnets(ctx, tx); err != nil {
		return errors.Errorf("cleaning up orphan subnets: %w", err)
	}

	// TODO (manadart 2020-06-12): It should be unlikely for the provider to be
	//   aware of devices that the machiner knows nothing about.
	//   At the time of writing we preserve existing behaviour and do not add
	//   them.
	//   Log for now and consider adding such devices in the future.
	for _, dev := range lldChanges.newDevices {
		st.logger.Debugf(ctx, "ignoring unrecognised device %q (%s) with addresses %v",
			dev.Name, dev.MACAddress, dev.Addresses)
	}
	return nil
}

// cleanupUniqueAddressOrphanSubnets removes orphan subnets that are unique IP
// addresses and unassociated with providers.
// It queries the database for IPv4 /32 or IPv6 /128 subnets not linked to
// addresses or providers and deletes them.
// Those subnets are created when addresses are created when their related subnet
// aren't already known, which is part of the SetMachineNetConfig responsibility.
func (st *State) cleanupUniqueAddressOrphanSubnets(ctx context.Context, tx *sqlair.TX) error {
	type orphan struct {
		UUID string `db:"uuid"`
	}
	// Fetch orphan subnets
	stmt, err := st.Prepare(`
SELECT s.uuid AS &orphan.uuid
FROM subnet AS s
LEFT JOIN ip_address AS a ON s.uuid = a.subnet_uuid
LEFT JOIN provider_subnet AS ps ON s.uuid = ps.subnet_uuid
WHERE a.uuid IS NULL -- orphan subnet, linked to no addresses
AND ps.provider_id IS NULL -- subnet without any provider id
AND (
    s.cidr LIKE '%.%/32' -- single address ipv4 subnet 
    OR  
    s.cidr LIKE '%:%/128' -- single address ipv6 subnet 
    )`, orphan{})
	if err != nil {
		return errors.Capture(err)
	}
	var orphanSubnets []orphan
	if err := tx.Query(ctx, stmt).GetAll(&orphanSubnets); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting orphan subnets: %w", err)
	}
	if len(orphanSubnets) == 0 {
		return nil
	}

	// remove orphan subnets
	err = st.removeSubnets(ctx, tx, transform.Slice(orphanSubnets, func(o orphan) string { return o.UUID }))
	if err != nil {
		return errors.Errorf("removing orphan subnets: %w", err)
	}
	return nil
}

// removeSubnets removes subnets from the subnet table.
// Subnets should not be linked to a provider subnet id nor a provider network
// id, or this function will fails.
// This function works only for subnet which are not related to any provider
// (such as placeholder subnet we created for addresses belonging to unknown
// subnets)
func (st *State) removeSubnets(
	ctx context.Context, tx *sqlair.TX,
	subnetUUIDs []string,
) error {
	type uuids []string

	// First remove any availability zone subnet mappings
	stmt, err := st.Prepare(`
DELETE FROM availability_zone_subnet
WHERE subnet_uuid IN ($uuids[:])
`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, uuids(subnetUUIDs)).Run()
	if err != nil {
		return errors.Capture(err)
	}

	// Finally remove the subnets
	stmt, err = st.Prepare(`
DELETE FROM subnet
WHERE uuid IN ($uuids[:])
`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	// This may fail if there is a provider ID associated with this subnet.
	// We don't remove it in this function to avoid side effects.
	// A subnet should be allowed to be removed only once it is no longer
	// associated to a provider_id.
	return errors.Capture(tx.Query(ctx, stmt, uuids(subnetUUIDs)).Run())
}

// removeDeviceProviderIDs removes provider-link layer devices mappings
// for  given device UUIDs.
func (st *State) removeDeviceProviderIDs(
	ctx context.Context, tx *sqlair.TX, deviceUUIDs []string,
) error {
	type uuids []string
	stmt, err := st.Prepare(`
DELETE FROM provider_link_layer_device
WHERE device_uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, uuids(deviceUUIDs)).Run()
}

// removeAddressProviderIDs removes provider-addresses mappings for given
// address UUIDs.
func (st *State) removeAddressProviderIDs(
	ctx context.Context, tx *sqlair.TX, addressUUIDs []string,
) error {
	type uuids []string
	stmt, err := st.Prepare(`
DELETE FROM provider_ip_address
WHERE address_uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, uuids(addressUUIDs)).Run()
}

// computeMergeAddressChanges prepares the changes to be applied to the addresses.
//
// It takes the normalized devices and the existing devices and returns the
// changes to be applied to the addresses.
func (st *State) computeMergeAddressChanges(
	normalized []mergeLinkLayerDevice, existingDevices []mergeLinkLayerDevice,
) mergeAddressesChanges {
	incomingAddresses := make(map[string][]mergeAddress)
	for _, device := range normalized {
		incomingAddresses[device.Name] = append(incomingAddresses[device.Name], device.Addresses...)
	}

	result := mergeAddressesChanges{
		providerIDsToAddOrUpdate: make(map[string]string),
		toRelinquish:             nil,
		addressesToAdd:           make(map[string][]mergeAddress),
	}
	for _, device := range existingDevices {
		deviceName, addresses := device.Name, device.Addresses
		incomings, _ := incomingAddresses[deviceName]
		// Find updates to existing addresses.
		for _, existing := range addresses {
			matchIncoming, ok := findMatchingAddresses(existing, incomings)
			// This device is no longer seen by the provider and the addresses
			// do not have a machine origin.
			if !ok && !hasAllMachineAddresses([]mergeAddress{existing}) {
				result.toRelinquish = append(result.toRelinquish, existing.UUID)
				continue
			}
			// Don't update which doesn't change
			if matchIncoming.ProviderID != "" && matchIncoming.ProviderID != existing.ProviderID {
				result.providerIDsToAddOrUpdate[matchIncoming.ProviderID] = existing.UUID
			}

			// If we already have a non empty provider subnet ID which doesn't
			// have changed
			if existing.ProviderSubnetID != "" &&
				matchIncoming.ProviderSubnetID == existing.ProviderSubnetID {
				continue // no changes
			}
			// Update if we have a new provider subnet id
			if matchIncoming.ProviderSubnetID != "" {
				existing.ProviderSubnetID = matchIncoming.ProviderSubnetID
				result.subnetToUpdate = append(result.subnetToUpdate, existing)
				continue
			}
			// Rematch if there is no subnet associated this address or if
			// it is a solo ip subnet
			ip, ipnet, _ := net.ParseCIDR(existing.SubnetCIDR)
			if ipnet == nil || strings.HasPrefix(ipnet.String(), ip.String()) {
				result.subnetToUpdate = append(result.subnetToUpdate, existing)
				continue
			}
		}
		// Find new addresses for the device.
		for _, incoming := range incomings {
			_, ok := findMatchingAddresses(incoming, addresses)
			if !ok {
				result.addressesToAdd[device.UUID] = append(result.addressesToAdd[device.UUID], incoming)
			}
		}
	}
	return result
}

// computeMergeLinkLayerDeviceChanges prepares the changes to be applied to the
// link layer devices.
//
// It takes the normalized devices and the existing devices and returns the
// changes to be applied to the link layer devices.
func (st *State) computeMergeLinkLayerDeviceChanges(
	ctx context.Context,
	existingDevices []mergeLinkLayerDevice,
	incomingDevices []mergeLinkLayerDevice,
	namelessHWAddrs set.Strings,
) mergeLinkLayerDevicesChanges {
	incomingByNames := st.matchByName(ctx, incomingDevices)
	notProcessed := set.NewStrings(slices.Collect(maps.Keys(incomingByNames))...)
	lldChanges := mergeLinkLayerDevicesChanges{
		toAddOrUpdate:       make(map[string]string),
		deviceToRelinquish:  make([]string, 0),
		addressToRelinquish: make([]string, 0),
	}
	for _, device := range existingDevices {
		notProcessed.Remove(device.Name)
		incomingDevice, ok := incomingByNames[device.Name]
		// If this device matches an incoming hardware address that we gave a
		// surrogate name to, do not relinquish it,
		if !ok && namelessHWAddrs.Contains(device.MACAddress) {
			continue
		}
		// This device is no longer seen by the provider and the addresses
		// do not have a machine origin.
		if !ok && !hasAllMachineAddresses(device.Addresses) {
			lldChanges.deviceToRelinquish = append(lldChanges.deviceToRelinquish, device.UUID)
			lldChanges.addressToRelinquish = append(lldChanges.addressToRelinquish,
				transform.Slice(device.Addresses, func(a mergeAddress) string { return a.UUID })...)
			continue
		}

		// if the provider id didn't change
		if device.ProviderID == incomingDevice.ProviderID {
			// Don't change which doesn't change.
			continue
		}

		// Log a warning if we are changing a provider ID that is already set.
		if device.ProviderID != "" &&
			device.ProviderID != incomingDevice.ProviderID {
			st.logger.Warningf(ctx, "changing provider ID for device %q from %q to %q",
				device.Name, device.ProviderID, incomingDevice.ProviderID)
		}
		lldChanges.toAddOrUpdate[incomingDevice.ProviderID] = device.UUID
	}
	// Collect
	lldChanges.newDevices = transform.Slice(notProcessed.Values(),
		func(name string) mergeLinkLayerDevice {
			return incomingByNames[name]
		},
	)
	return lldChanges
}

// findMatchingAddresses finds the matching address in the seachPool addresses
// that matches the lookFor address.
//
// It returns the matching address and a boolean indicating if the address
// was found.
//
// If the address is not found, it returns an empty address and false.
func findMatchingAddresses(
	lookFor mergeAddress,
	searchPool []mergeAddress,
) (mergeAddress, bool) {
	for _, potential := range searchPool {
		if strings.Split(lookFor.Value, "/")[0] == strings.Split(potential.Value, "/")[0] {
			return potential, true
		}
	}
	return mergeAddress{}, false
}

// hasAllMachineAddresses returns true if any of the addresses do
// no have a machine origin
func hasAllMachineAddresses(addresses []mergeAddress) bool {
	for _, addr := range addresses {
		if addr.Origin != "machine" {
			return false
		}
	}
	return true
}

// getExistingLinkLayerDevicesWithAddresses retrieves existing link layer devices for a given net node UUID.
// It queries the database to fetch devices and their associated IP addresses.
func (st *State) getExistingLinkLayerDevicesWithAddresses(
	ctx context.Context, tx *sqlair.TX,
	netNodeUUID string,
) ([]mergeLinkLayerDevice, error) {
	type device struct {
		UUID       string `db:"uuid"`
		Name       string `db:"name"`
		MACAddress string `db:"mac_address"`
		ProviderID string `db:"provider_id"`
		Type       string `db:"device_type"`
	}
	type address struct {
		UUID             string `db:"uuid"`
		DeviceUUID       string `db:"device_uuid"`
		Value            string `db:"address_value"`
		ProviderID       string `db:"provider_id"`
		ProviderSubnetID string `db:"provider_subnet_id"`
		SubnetCIDR       string `db:"subnet_cidr"`
		Origin           string `db:"origin"`
	}
	type netNode struct {
		UUID string `db:"uuid"`
	}
	getDevicesStmt, err := st.Prepare(`
SELECT 
	lld.uuid AS &device.uuid,
	lld.name AS &device.name,
	lld.mac_address AS &device.mac_address,
	plld.provider_id AS &device.provider_id,
    lldt.name AS &device.device_type
FROM link_layer_device AS lld
JOIN link_layer_device_type AS lldt ON lld.device_type_id = lldt.id
LEFT JOIN provider_link_layer_device AS plld ON lld.uuid = plld.device_uuid
WHERE lld.net_node_uuid = $netNode.uuid
`, device{}, netNode{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	getAddressesStmt, err := st.Prepare(`
SELECT
    ip.uuid AS &address.uuid,
    ip.device_uuid AS &address.device_uuid,
    ip.address_value AS &address.address_value,
    pip.provider_id AS &address.provider_id,
    ps.provider_id AS &address.provider_subnet_id,
    s.cidr AS &address.subnet_cidr,
    iao.name AS &address.origin
FROM  ip_address AS ip
LEFT JOIN provider_ip_address AS pip ON ip.uuid = pip.address_uuid
LEFT JOIN provider_subnet AS ps ON ip.subnet_uuid = ps.subnet_uuid
LEFT JOIN subnet AS s ON ip.subnet_uuid = s.uuid
JOIN  ip_address_origin AS iao ON ip.origin_id = iao.id
WHERE ip.net_node_uuid = $netNode.uuid`, address{}, netNode{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var devices []device
	if err := tx.Query(ctx, getDevicesStmt, netNode{UUID: netNodeUUID}).GetAll(&devices); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting all link layer devices from net node %q: %w", netNodeUUID, err)
	}
	var addresses []address
	if err := tx.Query(ctx, getAddressesStmt, netNode{UUID: netNodeUUID}).GetAll(&addresses); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("getting all addresses from net node %q: %w", netNodeUUID, err)
	}
	addressByDeviceUUID := make(map[string][]address)
	for _, address := range addresses {
		addressByDeviceUUID[address.DeviceUUID] = append(addressByDeviceUUID[address.DeviceUUID], address)
	}

	var result []mergeLinkLayerDevice
	for _, device := range devices {
		if !corenetwork.IsValidLinkLayerDeviceType(device.Type) {
			return nil, errors.Errorf("unexpected device type %q", device.Type)
		}
		addresses, _ := addressByDeviceUUID[device.UUID]
		result = append(result, mergeLinkLayerDevice{
			UUID:       device.UUID,
			Name:       device.Name,
			MACAddress: device.MACAddress,
			ProviderID: device.ProviderID,
			Type:       corenetwork.LinkLayerDeviceType(device.Type),
			Addresses: transform.Slice(addresses,
				func(a address) mergeAddress {
					return mergeAddress{
						UUID:             a.UUID,
						Value:            a.Value,
						ProviderID:       a.ProviderID,
						ProviderSubnetID: a.ProviderSubnetID,
						SubnetCIDR:       a.SubnetCIDR,
						Origin:           corenetwork.Origin(a.Origin),
					}
				}),
		})
	}

	return result, nil
}

// matchByName matches the incoming devices by name.
//
// It returns a map of the devices by name. If there are duplicate names,
// the first one is used.
func (st *State) matchByName(
	ctx context.Context, normalized []mergeLinkLayerDevice,
) map[string]mergeLinkLayerDevice {
	result := make(map[string]mergeLinkLayerDevice, len(normalized))
	for _, netInterface := range normalized {
		if _, found := result[netInterface.Name]; found {
			st.logger.Debugf(ctx, "duplicate name %q in incoming network interfaces", netInterface.Name)
			continue
		}
		result[netInterface.Name] = netInterface
	}
	return result
}

// normalizeLinkLayerDevices matches existing devices with incoming devices
// to mitigate various provider behavior.
//
// For instance, in some providers, such as EC2, know device hardware addresses,
// but not device names.
// We populate names on the incoming data based on
// matching existing devices by hardware address.
// If we locate multiple existing devices with the hardware address,
// such as will be the case for bridged NICs, fallback through the
// following options.
//   - If there is a device that already has a provider ID, use that name.
//   - If the devices are of different types, choose an ethernet device over
//     a bridge (as observed for MAAS).
func (st *State) normalizeLinkLayerDevices(
	ctx context.Context,
	incoming []network.NetInterface,
	devices []mergeLinkLayerDevice,
) ([]mergeLinkLayerDevice, set.Strings, error) {
	namelessHWAddrs := set.NewStrings()

	normalizedIncoming := transform.Slice(incoming,
		func(dev network.NetInterface) mergeLinkLayerDevice {
			if dev.MACAddress == nil {
				st.logger.Debugf(ctx, "empty MACAddress for an incoming device")
			}
			macAddr := dereferenceOrEmpty(dev.MACAddress)
			return mergeLinkLayerDevice{
				Name:       dev.Name,
				MACAddress: strings.ToLower(macAddr),
				ProviderID: string(dereferenceOrEmpty(dev.ProviderID)),
				Type:       dev.Type,
				Addresses: transform.Slice(dev.Addrs,
					func(addr network.NetAddr) mergeAddress {
						return mergeAddress{
							Value:            addr.AddressValue,
							ProviderID:       string(dereferenceOrEmpty(addr.ProviderID)),
							ProviderSubnetID: string(dereferenceOrEmpty(addr.ProviderSubnetID)),
							AddressType:      addr.AddressType,
							ConfigType:       addr.ConfigType,
							Origin:           addr.Origin,
							Scope:            addr.Scope,
							IsShadow:         addr.IsShadow,
							IsSecondary:      addr.IsSecondary,
						}
					}),
			}
		},
	)

	// Check that the incoming data is not using a provider ID for more
	// than one device. This is not verified by transaction assertions.
	seenProviders := set.Strings{}
	duplicatedProviders := set.Strings{}
	for _, dev := range normalizedIncoming {
		if dev.ProviderID == "" {
			continue
		}
		if seenProviders.Contains(dev.ProviderID) {
			duplicatedProviders.Add(dev.ProviderID)
		}
		seenProviders.Add(dev.ProviderID)
	}
	if len(duplicatedProviders) > 0 {
		return nil, namelessHWAddrs, errors.Errorf("unable to set provider IDs %q for multiple devices",
			duplicatedProviders.Values())
	}

	// If the incoming devices have names, no action is required
	// (assuming all or none here per current known provider implementations
	// of `NetworkInterfaces`)
	if len(normalizedIncoming) > 0 && normalizedIncoming[0].Name != "" {
		return normalizedIncoming, namelessHWAddrs, nil
	}

	// Given that the incoming devices do not have names, first get the best
	// device per hardware address.
	devByHWAddr := make(map[string]mergeLinkLayerDevice)
	for _, dev := range devices {
		hwAddr := dev.MACAddress

		// If this is the first one we've seen, select it.
		current, ok := devByHWAddr[hwAddr]
		if !ok {
			devByHWAddr[hwAddr] = dev
			continue
		}

		// If we have a matching device that already has a provider ID,
		// I.e. it was previously matched to the hardware address,
		// make sure the same one is resolved thereafter.
		if current.ProviderID != "" {
			continue
		}

		// Otherwise choose a physical NIC over other device types.
		if dev.Type == corenetwork.EthernetDevice {
			devByHWAddr[hwAddr] = dev
		}
	}

	// Set the names and remember normalized nameless addresses
	for i, dev := range normalizedIncoming {
		if existing, ok := devByHWAddr[dev.MACAddress]; ok && dev.Name == "" {
			normalizedIncoming[i].Name = existing.Name
			namelessHWAddrs.Add(dev.MACAddress)
		}
	}
	return normalizedIncoming, namelessHWAddrs, nil
}

// relinquishAddresses relinquish ip addresses associated with input uuids to
// machine origin.
func (st *State) relinquishAddresses(
	ctx context.Context, tx *sqlair.TX, uuidsToRelinquish []string,
) error {
	type uuids []string
	stmt, err := st.Prepare(`
UPDATE ip_address 
SET origin_id = 0 -- relinquished to machine
WHERE uuid IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, uuids(uuidsToRelinquish)).Run()
}

func (st *State) updateSubnets(ctx context.Context, tx *sqlair.TX, update []mergeAddress) error {
	// split the list between address that need to be rematched and addresses that need to be updated
	var toRematch []mergeAddress
	var toUpdate []mergeAddress
	for _, address := range update {
		if address.ProviderSubnetID == "" {
			toRematch = append(toRematch, address)
		} else {
			toUpdate = append(toUpdate, address)
		}
	}

	// update subnets for address with a provider subnet id
	for _, address := range toUpdate {
		err := st.updateSubnetFromProviderID(ctx, tx, address)
		if err != nil {
			return errors.Errorf("failed to update subnet for address %q (%s) with provider subnet id %q: %w",
				address.Value, address.UUID, address.ProviderSubnetID, err)
		}
	}
	return nil
}

// updateSubnetFromProviderID updates the subnet_uuid field for a specific
// address using its provider subnet id.
// Updates will be ignored if the provider subnet id doesn't exist or if
// the address doesn't belong to the identified subnet.
func (st *State) updateSubnetFromProviderID(ctx context.Context, tx *sqlair.TX, address mergeAddress) error {
	type updateAddress struct {
		UUID       string `db:"uuid"`
		SubnetUUID string `db:"subnet_uuid"`
	}

	subnetUUID, err := st.validateSubnetUpdate(ctx, tx, address)
	if err != nil {
		st.logger.Warningf(ctx, "ignoring subnet update for address %q (%s): %s",
			address.Value, address.UUID, err)
		return nil
	}

	stmt, err := st.Prepare(`
UPDATE ip_address 
SET subnet_uuid = $updateAddress.subnet_uuid
WHERE uuid = $updateAddress.uuid`, updateAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, updateAddress{
		UUID:       address.UUID,
		SubnetUUID: subnetUUID,
	}).Run()
}

// validateSubnetUpdate verifies if a given address is valid within its
// associated subnet and retrieves the subnet ID.
// It checks if the address belongs to the CIDR range of the candidate
// subnet from the database, identified through its providerSubnetID
func (st *State) validateSubnetUpdate(ctx context.Context, tx *sqlair.TX, address mergeAddress) (string, error) {
	candidateSubnet, err := st.getSubnetByProviderID(ctx, tx, address.ProviderSubnetID)
	if err != nil {
		return "", errors.Errorf("getting subnet for provider id %q: %w", address.ProviderSubnetID, err)
	}
	cidr, err := candidateSubnet.ParsedCIDRNetwork()
	if err != nil {
		return "", errors.Errorf("parsing candidate subnet cidr %q: %w", candidateSubnet.CIDR, err)
	}
	if !cidr.Contains(net.ParseIP(strings.Split(address.Value, "/")[0])) {
		return "", errors.Errorf("address %q (%s) is not in subnet %q (providerID: %q)",
			address.Value,
			address.UUID,
			candidateSubnet.CIDR,
			address.ProviderSubnetID)
	}
	return candidateSubnet.ID.String(), nil
}

// subnetCIDRUUIDByProviderID returns a map of subnet provider IDs to a
// struct including the subnet CIDR and UUID.
func (st *State) subnetCIDRUUIDByProviderID(
	ctx context.Context,
	tx *sqlair.TX,
	add map[string][]mergeAddress,
) (map[string]providerSubnetCIDR, error) {
	type ids []string
	input := make(ids, 0)
	for _, addrs := range add {
		for _, addr := range addrs {
			input = append(input, addr.ProviderSubnetID)
		}
	}
	stmt, err := st.Prepare(`
SELECT &providerSubnetCIDR.*
FROM provider_subnet AS ps
JOIN subnet AS s ON ps.subnet_uuid = s.uuid
WHERE ps.provider_id IN ($ids[:])
`, providerSubnetCIDR{}, ids{})
	if err != nil {
		return nil, errors.Errorf("preparing subnet query: %w", err)
	}

	output := []providerSubnetCIDR{}
	err = tx.Query(ctx, stmt, input).GetAll(&output)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	result := transform.SliceToMap(output, func(in providerSubnetCIDR) (string, providerSubnetCIDR) {
		return in.ProviderID, in
	})

	return result, nil
}

// ensureAddressSubnetSuffix return an address including the subnet's suffix
// and the subnetUUID. If the address is not an IPv4 nor IPv6 address, no
// suffix is added.
func ensureAddressSubnetSuffix(value, subnetProviderID string, data map[string]providerSubnetCIDR) (string, string, error) {
	// Getting the subnet is helpful, but not essential.
	subnet, _ := data[subnetProviderID]

	parts := strings.Split(value, "/")
	if len(parts) == 2 {
		// Best case, the address has a CIDR suffix.
		return value, subnet.SubnetUUID, nil
	}

	// Find the CIDR suffix, try the subnetCIDR first, if
	// not define based on address type.
	var suffix string
	subnetCIDRParts := strings.Split(subnet.CIDR, "/")
	switch len(subnetCIDRParts) {
	case 2:
		suffix = subnetCIDRParts[1]
	case 1:
		addType := corenetwork.DeriveAddressType(value)
		if addType == corenetwork.IPv4Address {
			suffix = "32"
		} else if addType == corenetwork.IPv6Address {
			suffix = "128"
		} else {
			return subnet.SubnetUUID, value, nil
		}
	default:
		return "", "", errors.Errorf("unable to get CIDR mask from: %q", subnet.CIDR)
	}

	addressValue := value + "/" + suffix
	return addressValue, subnet.SubnetUUID, nil
}

func (st *State) addAddressFromProvider(
	ctx context.Context,
	tx *sqlair.TX,
	netNodeUUID string,
	add map[string][]mergeAddress,
) error {
	if len(add) == 0 {
		return nil
	}

	subnets, err := st.subnetCIDRUUIDByProviderID(ctx, tx, add)
	if err != nil {
		return errors.Capture(err)
	}
	lookups, err := st.getNetConfigLookups(ctx, tx)
	if err != nil {
		return errors.Capture(err)
	}
	providerIDAddrs := make(map[string]string)
	ipAddresses := make([]ipAddressDML, 0)
	for devUUID, addresses := range add {
		for _, addr := range addresses {
			ipAddrUUID, err := uuid.NewUUID()
			if err != nil {
				return errors.Capture(err)
			}
			value, subnetUUID, err := ensureAddressSubnetSuffix(addr.Value, addr.ProviderSubnetID, subnets)
			if err != nil {
				return errors.Capture(err)
			}
			ipAddresses = append(ipAddresses, ipAddressDML{
				UUID:         ipAddrUUID.String(),
				NodeUUID:     netNodeUUID,
				DeviceUUID:   devUUID,
				AddressValue: value,
				SubnetUUID:   nilZeroPtr(subnetUUID),
				TypeID:       lookups.addrType[addr.AddressType],
				ConfigTypeID: lookups.addrConfigType[addr.ConfigType],
				OriginID:     lookups.origin[addr.Origin],
				ScopeID:      lookups.scope[addr.Scope],
				IsSecondary:  addr.IsSecondary,
				IsShadow:     addr.IsShadow,
			})
			if addr.ProviderID != "" {
				providerIDAddrs[addr.ProviderID] = ipAddrUUID.String()
			}
		}
	}

	insertAddressStmt, err := sqlair.Prepare(`
INSERT INTO ip_address (*)
VALUES ($ipAddressDML.*)
;`, ipAddressDML{})
	if err != nil {
		return errors.Capture(err)
	}

	for _, ipAddress := range ipAddresses {
		if err = tx.Query(ctx, insertAddressStmt, ipAddress).Run(); err != nil {
			return errors.Capture(err)
		}
	}

	return st.addProviderAddress(ctx, tx, providerIDAddrs)
}
