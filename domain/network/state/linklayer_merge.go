// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// mergeLinkLayerDevice is a subset of linklayerdevice.LinkLayerDevice
// that is used to merge the existing link layer devices with the
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

// mergeAddress is a subset of ipaddress.IPAddress that is used to merge
// the existing addresses with the incoming ones.
//
// It contains only the fields that are used to identify and merge the addresses
type mergeAddress struct {
	UUID       string
	Value      string
	ProviderID string
}

// mergeLinkLayerDevicesChanges contains the changes to be applied to the
// link layer devices.
type mergeLinkLayerDevicesChanges struct {
	// toAdd maps provider IDs to LinkLayerDeviceUUIDs to be added
	// in provider_link_layer_device.
	toAdd map[string]string
	// ToRemove are the provider IDs to remove from provider_link_layer_device
	toRemove []string
	// toRelinquish is a list of LinkLayerDeviceUUIDs to
	// relinquish to the machine, i.e., set all their addresses origin to machine.
	toRelinquish []string
	// newDevices are the incoming devices that did not match any we already
	// have in state.
	newDevices []mergeLinkLayerDevice
}

// mergeAddressesChanges contains the changes to be applied to the
// addresses.
type mergeAddressesChanges struct {
	// toAdd maps provider IDs to ip_address UUID to be added
	// in provider_link_layer_device.
	toAdd map[string]string
	// ToRemove are the provider IDs to remove from provider_ip_address
	toRemove []string
	// toRelinquish are a list of ip_address to
	// relinquish to machine, i.e., set their origin to machine.
	toRelinquish []string
}

// MergeLinkLayerDevice merges the existing link layer devices with the
// incoming ones.
func (st *State) MergeLinkLayerDevice(
	ctx context.Context,
	machineUUID string,
	incoming []network.NetInterface,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(
		ctx, func(ctx context.Context, tx *sqlair.TX) error {
			nodeUUID, err := st.getMachineNetNodeUUID(ctx, tx,
				machineUUID)
			if err != nil {
				return errors.Errorf(
					"getting net node UUID for machine %q: %w",
					machineUUID, err,
				)
			}

			existingDevices, err := st.getExistingLinkLayerDevices(
				ctx, tx, nodeUUID,
			)
			if err != nil {
				return errors.Errorf(
					"getting existing link layer devices for node %q: %w",
					nodeUUID, err,
				)
			}

			if len(existingDevices) == 0 {
				// Noop
				st.logger.Debugf(ctx, "no existing devices, "+
					"ignoring %d incoming device for net node %q",
					len(incoming), nodeUUID)
				return nil
			}

			normalized, namelessHWAddrs,
				err := st.normalizeLinkLayeredDevices(ctx,
				incoming,
				existingDevices,
			)
			if err != nil {
				return errors.Capture(err)
			}

			lldChanges := st.computeMergeLinkLayerDeviceChanges(
				ctx, existingDevices, normalized, namelessHWAddrs,
			)
			addressChanges := st.computeMergeAddressChanges(
				normalized, existingDevices,
			)

			return st.applyMergeLinkLayerChanges(
				ctx, tx, lldChanges,
				addressChanges,
			)
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
	stmt, err := st.Prepare(
		`
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
	return nil
}

// applyMergeLinkLayerChanges applies the changes to the link layer devices.
func (st *State) applyMergeLinkLayerChanges(
	ctx context.Context, tx *sqlair.TX,
	lldChanges mergeLinkLayerDevicesChanges,
	addressChanges mergeAddressesChanges,
) error {
	// Add addresses of relinquished link layer devices to the overall set of
	// addresses to relinquish
	addresses, err := st.getAllAddressesUUIDForLinkLayerDeviceUUIDs(
		ctx, tx,
		lldChanges.toRelinquish,
	)
	if err != nil {
		return errors.Errorf(
			"getting addresses for link layer devices to relinquish: %w", err,
		)
	}
	addressChanges.toRelinquish = append(
		addressChanges.toRelinquish, addresses...,
	)

	err = st.deleteProviderLinkLayerDevice(ctx, tx, lldChanges.toRemove)
	if err != nil {
		return errors.Errorf(
			"removing provider IDs from link layer devices: %w", err,
		)
	}
	err = st.deleteProviderAddress(ctx, tx, addressChanges.toRemove)
	if err != nil {
		return errors.Errorf("removing provider IDs from addresses: %w", err)
	}
	err = st.addProviderLinkLayerDevice(ctx, tx, lldChanges.toAdd)
	if err != nil {
		return errors.Errorf(
			"adding provider IDs to link layer devices: %w",
			err,
		)
	}
	err = st.addProviderAddress(ctx, tx, addressChanges.toAdd)
	if err != nil {
		return errors.Errorf("adding provider IDs to addresses: %w", err)
	}
	err = st.relinquishAddresses(ctx, tx, addressChanges.toRelinquish)
	if err != nil {
		return errors.Errorf("relinquishing addresses: %w", err)
	}

	// TODO (manadart 2020-06-12): It should be unlikely for the provider to be
	//   aware of devices that the machiner knows nothing about.
	//   At the time of writing we preserve existing behaviour and do not add
	//   them.
	//   Log for now and consider adding such devices in the future.
	for _, dev := range lldChanges.newDevices {
		st.logger.Debugf(
			ctx,
			"ignoring unrecognised device %q (%s) with addresses %v",
			dev.Name, dev.MACAddress, dev.Addresses,
		)
	}

	return nil
}

// deleteProviderLinkLayerDevice removes provider-link layer devices mappings
// for  given provider IDs.
func (st *State) deleteProviderLinkLayerDevice(
	ctx context.Context, tx *sqlair.TX, providerUUIDs []string,
) error {
	type uuids []string
	stmt, err := st.Prepare(`
DELETE FROM provider_link_layer_device
WHERE provider_id IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, uuids(providerUUIDs)).Run()
}

// deleteProviderAddress removes provider-addresses mappings for given
// provider IDs.
func (st *State) deleteProviderAddress(
	ctx context.Context, tx *sqlair.TX, providerUUIDs []string,
) error {
	type uuids []string
	stmt, err := st.Prepare(`
DELETE FROM provider_ip_address
WHERE provider_id IN ($uuids[:])`, uuids{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, uuids(providerUUIDs)).Run()
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
		incomingAddresses[device.Name] = append(
			incomingAddresses[device.Name], device.Addresses...,
		)
	}

	result := mergeAddressesChanges{
		toAdd:        make(map[string]string),
		toRemove:     nil,
		toRelinquish: nil,
	}
	for _, device := range existingDevices {
		deviceName, addresses := device.Name, device.Addresses
		incomings, _ := incomingAddresses[deviceName]
		for _, existing := range addresses {
			matchIncoming, ok := findMatchingAddresses(existing, incomings)
			if ok && matchIncoming.ProviderID == existing.ProviderID {
				continue
			}
			result.toRemove = append(
				result.toRemove, existing.ProviderID,
			)
			if !ok {
				result.toRelinquish = append(result.toRelinquish, existing.UUID)
				continue
			}
			result.toAdd[matchIncoming.ProviderID] = existing.UUID
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
	notProcessed := set.NewStrings(
		slices.Collect(
			maps.Keys(
				incomingByNames,
			),
		)...,
	)
	lldChanges := mergeLinkLayerDevicesChanges{
		toAdd:        make(map[string]string),
		toRemove:     make([]string, 0),
		toRelinquish: make([]string, 0),
	}
	for _, device := range existingDevices {
		notProcessed.Remove(device.Name)
		incomingDevice, ok := incomingByNames[device.Name]
		// If this device matches an incoming hardware address that we gave a
		// surrogate name to, do not relinquish it,
		if !ok && namelessHWAddrs.Contains(device.MACAddress) {
			continue
		}

		// Log a warning if we are changing a provider ID that is already set.
		if ok && device.ProviderID != "" &&
			device.ProviderID != incomingDevice.ProviderID {
			st.logger.Warningf(
				ctx,
				"changing provider ID for device %q from %q to %q",
				device.Name, device.ProviderID, incomingDevice.ProviderID,
			)
		} else if ok && device.ProviderID == incomingDevice.ProviderID {
			// Don't change which doesn't change.
			continue
		}

		// In any cases, we will remove all existing providers ids for this
		// machine. However, if there is a replacement provider id we will
		// add it and if none we will relinquish the addresses.
		lldChanges.toRemove = append(lldChanges.toRemove, device.ProviderID)
		if !ok {
			lldChanges.toRelinquish = append(
				lldChanges.toRelinquish, device.UUID,
			)
			continue
		}
		lldChanges.toAdd[incomingDevice.ProviderID] = device.UUID
	}
	// Collect
	lldChanges.newDevices = transform.Slice(
		notProcessed.Values(),
		func(name string) mergeLinkLayerDevice {
			return incomingByNames[name]
		},
	)
	return lldChanges
}

// findMatchingAddresses finds the matching address in the incoming addresses
// that matches the existing address.
//
// It returns the matching address and a boolean indicating if the address
// was found.
//
// If the address is not found, it returns an empty address and false.
func findMatchingAddresses(
	existing mergeAddress,
	incomings []mergeAddress,
) (mergeAddress, bool) {
	for _, incoming := range incomings {
		if strings.HasPrefix(incoming.Value, existing.Value) {
			return incoming, true
		}
	}
	return mergeAddress{}, false
}

// getAllAddressesUUIDForLinkLayerDeviceUUIDs retrieves all IP address UUIDs
// associated with a given set of link-layer device UUIDs from the database.
func (st *State) getAllAddressesUUIDForLinkLayerDeviceUUIDs(
	ctx context.Context, tx *sqlair.TX,
	relinquish []string,
) ([]string, error) {
	type address struct {
		UUID string `db:"uuid"`
	}
	type uuids []string
	stmt, err := st.Prepare(`
SELECT &address.uuid 
FROM ip_address 
WHERE device_uuid in ($uuids[:])
`, address{}, uuids{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var addresses []address
	err = tx.Query(
		ctx, stmt, uuids(relinquish),
	).GetAll(&addresses)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf(
			"getting all addresses for link layer devices %q: %w",
			relinquish, err,
		)
	}
	return transform.Slice(addresses, func(f address) string {
		return f.UUID
	}), nil
}

// getExistingLinkLayerDevices retrieves existing link layer devices for a given net node UUID.
// It queries the database to fetch devices and their associated IP addresses.
func (st *State) getExistingLinkLayerDevices(
	ctx context.Context, tx *sqlair.TX,
	netNodeUUID string,
) ([]mergeLinkLayerDevice, error) {
	type device struct {
		UUID       string `db:"uuid"`
		Name       string `db:"name"`
		MACAddress string `db:"mac_address"`
		ProviderID string `db:"provider_id"`
		TypeID     int64  `db:"device_type_id"`
	}
	type address struct {
		UUID       string `db:"uuid"`
		Value      string `db:"address_value"`
		ProviderID string `db:"provider_id"`
	}
	type netNode struct {
		UUID string `db:"uuid"`
	}
	getDevicesStmt, err := st.Prepare(`
SELECT &device.*
FROM link_layer_device AS lld
LEFT JOIN provider_link_layer_device AS plld ON lld.uuid = plld.device_uuid
WHERE lld.net_node_uuid = $netNode.uuid
`, device{}, netNode{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	getAddressesStmt, err := st.Prepare(`
SELECT &address.*
FROM ip_address AS ip
LEFT JOIN provider_ip_address AS pip ON ip.uuid = pip.address_uuid
WHERE ip.device_uuid = ($device.uuid)`, address{}, device{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var devices []device
	if err := tx.Query(ctx, getDevicesStmt,
		netNode{UUID: netNodeUUID}).GetAll(
		&devices); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf(
			"getting all link layer devices from net notd %q: %w",
			netNodeUUID, err)
	}

	var result []mergeLinkLayerDevice
	for _, device := range devices {
		var addresses []address
		if err := tx.Query(ctx, getAddressesStmt,
			device).GetAll(&addresses); err != nil &&
			!errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Errorf(
				"getting all addresses for link layer device %q: %w",
				device.UUID, err)
		}
		deviceType, err := decodeDeviceType(device.TypeID)
		if err != nil {
			return nil, errors.Errorf(
				"decoding device type %d: %w", device.TypeID, err)
		}
		result = append(result, mergeLinkLayerDevice{
			UUID:       device.UUID,
			Name:       device.Name,
			MACAddress: device.MACAddress,
			ProviderID: device.ProviderID,
			Type:       deviceType,
			Addresses: transform.Slice(addresses,
				func(a address) mergeAddress {
					return mergeAddress{
						UUID:       a.UUID,
						Value:      a.Value,
						ProviderID: a.ProviderID,
					}
				}),
		})
	}

	return result, nil
}

// getMachineNetNodeUUID retrieves the NetNodeUUID associated with a given
// Machine UUID from the database using a prepared SQL statement.
// Returns the NetNodeUUID or an error if the operation fails.
func (st *State) getMachineNetNodeUUID(
	ctx context.Context, tx *sqlair.TX,
	machineUUID string,
) (string, error) {
	type node struct {
		MachineUUID string `db:"machine_uuid"`
		NetNodeUUID string `db:"net_node_uuid"`
	}

	machine := node{MachineUUID: machineUUID}

	stmt, err := st.Prepare(
		`
SELECT &node.net_node_uuid 
FROM   machine 
WHERE  uuid = $node.machine_uuid
`, machine,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	return machine.NetNodeUUID, errors.Capture(
		tx.Query(
			ctx, stmt, machine,
		).Get(&machine),
	)
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
			st.logger.Debugf(
				ctx, "duplicate name %q in incoming network"+
					" interfaces", netInterface.Name,
			)
			continue
		}
		result[netInterface.Name] = netInterface
	}
	return result
}

// normaliseIncoming is intended to accommodate providers such as EC2
// that know device hardware addresses, but not device names.
// We populate names on the incoming data based on
// matching existing devices by hardware address.
// If we locate multiple existing devices with the hardware address,
// such as will be the case for bridged NICs, fallback through the
// following options.
//   - If there is a device that already has a provider ID, use that name.
//   - If the devices are of different types, choose an ethernet device over
//     a bridge (as observed for MAAS).
func (st *State) normalizeLinkLayeredDevices(
	ctx context.Context,
	incoming []network.NetInterface,
	devices []mergeLinkLayerDevice,
) ([]mergeLinkLayerDevice, set.Strings, error) {
	namelessHWAddrs := set.NewStrings()

	normalizedIncoming := transform.Slice(incoming,
		func(dev network.NetInterface) mergeLinkLayerDevice {
			if dev.MACAddress == nil {
				st.logger.Debugf(ctx,
					"empty MACAddress for an incoming device")
			}
			return mergeLinkLayerDevice{
				Name:       dev.Name,
				MACAddress: deref(dev.MACAddress),
				ProviderID: string(deref(dev.ProviderID)),
				Type:       dev.Type,
				Addresses: transform.Slice(dev.Addrs,
					func(addr network.NetAddr) mergeAddress {
						return mergeAddress{
							Value:      addr.AddressValue,
							ProviderID: string(deref(addr.ProviderID)),
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
		if dev.ProviderID != "" {
			continue
		}
		if seenProviders.Contains(dev.ProviderID) {
			duplicatedProviders.Add(dev.ProviderID)
		}
		seenProviders.Add(dev.ProviderID)
	}
	if len(duplicatedProviders) > 0 {
		return nil, namelessHWAddrs, errors.Errorf(
			"unable to set provider IDs %q for multiple devices",
			duplicatedProviders.Values(),
		)
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

	// Set the names and remembers normalized nameless addresses
	for _, dev := range normalizedIncoming {
		if existing, ok := devByHWAddr[dev.MACAddress]; ok && dev.Name == "" {
			dev.Name = existing.Name
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

// deref returns the value pointed to by t or the zero value of T if t is nil.
func deref[T any](t *T) T {
	var zero T
	if t == nil {
		return zero
	}
	return *t
}
