// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"net"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/network"
)

// LinkLayerDevice returns the link-layer device matching the given name. An
// error satisfying errors.IsNotFound() is returned when no such device exists
// on the machine.
func (m *Machine) LinkLayerDevice(name string) (*LinkLayerDevice, error) {
	devID := linkLayerDeviceDocIDFromName(m.st, m.doc.Id, name)
	dev, err := m.st.LinkLayerDevice(devID)
	return dev, errors.Trace(err)
}

func linkLayerDeviceDocIDFromName(st *State, machineID, deviceName string) string {
	return st.docID(linkLayerDeviceGlobalKey(machineID, deviceName))
}

// AllLinkLayerDevices returns all exiting link-layer devices of the machine.
func (m *Machine) AllLinkLayerDevices() ([]*LinkLayerDevice, error) {
	var allDevices []*LinkLayerDevice
	callbackFunc := func(resultDoc *linkLayerDeviceDoc) {
		allDevices = append(allDevices, newLinkLayerDevice(m.st, *resultDoc))
	}

	if err := m.forEachLinkLayerDeviceDoc(nil, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allDevices, nil
}

func (m *Machine) forEachLinkLayerDeviceDoc(
	docFieldsToSelect bson.D, callbackFunc func(resultDoc *linkLayerDeviceDoc),
) error {
	linkLayerDevices, closer := m.st.db().GetCollection(linkLayerDevicesC)
	defer closer()

	query := linkLayerDevices.Find(bson.D{{"machine-id", m.doc.Id}})
	if docFieldsToSelect != nil {
		query = query.Select(docFieldsToSelect)
	}
	iter := query.Iter()

	var resultDoc linkLayerDeviceDoc
	for iter.Next(&resultDoc) {
		callbackFunc(&resultDoc)
	}

	return errors.Trace(iter.Close())
}

// AllProviderInterfaceInfos returns the provider details for all of
// the link layer devices belonging to this machine. These can be used
// to identify the devices when interacting with the provider
// directly (for example, releasing container addresses).
func (m *Machine) AllProviderInterfaceInfos() ([]network.ProviderInterfaceInfo, error) {
	devices, err := m.AllLinkLayerDevices()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]network.ProviderInterfaceInfo, len(devices))
	for i, device := range devices {
		result[i].InterfaceName = device.Name()
		result[i].HardwareAddress = device.MACAddress()
		result[i].ProviderId = device.ProviderID()
	}
	return result, nil
}

// RemoveAllLinkLayerDevices removes all existing link-layer devices of the
// machine in a single transaction. No error is returned when some or all of the
// devices were already removed.
func (m *Machine) RemoveAllLinkLayerDevices() error {
	ops, err := m.removeAllLinkLayerDevicesOps()
	if err != nil {
		return errors.Trace(err)
	}
	if len(ops) == 0 {
		return nil
	}
	return m.st.db().RunTransaction(ops)
}

func (m *Machine) removeAllLinkLayerDevicesOps() ([]txn.Op, error) {
	var ops []txn.Op
	callbackFunc := func(resultDoc *linkLayerDeviceDoc) {
		removeOps := removeLinkLayerDeviceUnconditionallyOps(resultDoc.DocID)
		ops = append(ops, removeOps...)
		if resultDoc.ProviderID != "" {
			providerId := network.Id(resultDoc.ProviderID)
			op := m.st.networkEntityGlobalKeyRemoveOp("linklayerdevice", providerId)
			ops = append(ops, op)
		}
	}

	if err := m.forEachLinkLayerDeviceDoc(nil, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}

	return ops, nil
}

// LinkLayerDeviceArgs contains the arguments accepted by Machine.SetLinkLayerDevices().
type LinkLayerDeviceArgs struct {
	// Name is the name of the device as it appears on the machine.
	Name string

	// MTU is the maximum transmission unit the device can handle.
	MTU uint

	// ProviderID is a provider-specific ID of the device. Empty when not
	// supported by the provider. Cannot be cleared once set.
	ProviderID network.Id

	// Type is the type of the underlying link-layer device.
	Type network.LinkLayerDeviceType

	// MACAddress is the media access control address for the device.
	MACAddress string

	// IsAutoStart is true if the device should be activated on boot.
	IsAutoStart bool

	// IsUp is true when the device is up (enabled).
	IsUp bool

	// ParentName is the name of the parent device, which may be empty. If set,
	// it needs to be an existing device on the same machine, unless the current
	// device is inside a container, in which case ParentName can be a global
	// key of a BridgeDevice on the host machine of the container. Traffic
	// originating from a device egresses from its parent device.
	ParentName string

	// If this is device is part of a virtual switch, this field indicates
	// the type of switch (e.g. an OVS bridge ) this port belongs to.
	VirtualPortType network.VirtualPortType
}

// AddLinkLayerDeviceOps returns transaction operations for adding the input
// link-layer device and the supplied addresses to the machine.
// TODO (manadart 2020-07-22): This method is currently used only for adding
// machine sourced devices and addresses.
// If it is used in future to set provider devices, the provider ID args must
// be included and the global ID collection must be maintained and verified.
func (m *Machine) AddLinkLayerDeviceOps(
	devArgs LinkLayerDeviceArgs, addrArgs ...LinkLayerDeviceAddress,
) ([]txn.Op, error) {
	devDoc := m.newLinkLayerDeviceDocFromArgs(&devArgs)
	ops := []txn.Op{insertLinkLayerDeviceDocOp(devDoc)}
	for _, addr := range addrArgs {
		address, subnet, err := addr.addressAndSubnet()
		if err != nil {
			return nil, errors.Trace(err)
		}

		newDoc := ipAddressDoc{
			DeviceName:       devDoc.Name,
			DocID:            devDoc.DocID + "#ip#" + address,
			ModelUUID:        m.doc.ModelUUID,
			MachineID:        m.doc.Id,
			SubnetCIDR:       subnet,
			ConfigMethod:     addr.ConfigMethod,
			Value:            address,
			DNSServers:       addr.DNSServers,
			DNSSearchDomains: addr.DNSSearchDomains,
			GatewayAddress:   addr.GatewayAddress,
			IsDefaultGateway: addr.IsDefaultGateway,
			Origin:           addr.Origin,
		}
		ops = append(ops, insertIPAddressDocOp(&newDoc))
	}

	return ops, nil
}

// SetLinkLayerDevices sets link-layer devices on the machine, adding or
// updating existing devices as needed, in a single transaction. ProviderID
// field can be empty if not supported by the provider, but when set must be
// unique within the model, and cannot be unset once set. Errors are returned in
// the following cases:
// - Machine is no longer alive or is missing;
// - Model no longer alive;
// - errors.NotValidError, when any of the fields in args contain invalid values;
//
// Deprecated: (manadart 2020-10-12) This method is only used by tests and is in
// the process of removal. Do not add new usages of it.
func (m *Machine) SetLinkLayerDevices(devicesArgs ...LinkLayerDeviceArgs) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set link-layer devices to machine %q", m.doc.Id)

	if len(devicesArgs) == 0 {
		logger.Debugf(context.TODO(), "no device addresses to set")
		return nil
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		newDocs, err := m.prepareToSetLinkLayerDevices(devicesArgs)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if m.doc.Life != Alive {
			return nil, errors.Errorf("machine %q not alive", m.doc.Id)
		}
		if attempt > 0 {
			allIds, err := m.st.allProviderIDsForLinkLayerDevices()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, args := range devicesArgs {
				if allIds.Contains(string(args.ProviderID)) {
					return nil, errors.Annotatef(
						newProviderIDNotUniqueError(args.ProviderID), "invalid device %q", args.Name)
				}
			}
		}

		setDevicesOps, err := m.setDevicesFromDocsOps(newDocs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(setDevicesOps) == 0 {
			logger.Debugf(context.TODO(), "no changes to LinkLayerDevices for machine %q", m.Id())
			return nil, jujutxn.ErrNoOperations
		}
		return append([]txn.Op{m.assertAliveOp()}, setDevicesOps...), nil
	}
	if err := m.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *State) allProviderIDsForLinkLayerDevices() (set.Strings, error) {
	return st.allProviderIDsForEntity("linklayerdevice")
}

func (st *State) allProviderIDsForEntity(entityName string) (set.Strings, error) {
	idCollection, closer := st.db().GetCollection(providerIDsC)
	defer closer()

	allProviderIDs := set.NewStrings()
	var doc struct {
		ID string `bson:"_id"`
	}

	pattern := fmt.Sprintf("^%s:%s:.+$", st.ModelUUID(), entityName)
	modelProviderIDs := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	iter := idCollection.Find(modelProviderIDs).Iter()
	for iter.Next(&doc) {
		localProviderID := st.localID(doc.ID)[len(entityName)+1:]
		allProviderIDs.Add(localProviderID)
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return allProviderIDs, nil
}

func (m *Machine) prepareToSetLinkLayerDevices(devicesArgs []LinkLayerDeviceArgs) ([]linkLayerDeviceDoc, error) {
	var pendingDocs []linkLayerDeviceDoc
	pendingNames := set.NewStrings()

	for _, args := range devicesArgs {
		newDoc, err := m.prepareOneSetLinkLayerDeviceArgs(&args, pendingNames)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pendingNames.Add(args.Name)
		pendingDocs = append(pendingDocs, *newDoc)
	}
	return pendingDocs, nil
}

func (m *Machine) prepareOneSetLinkLayerDeviceArgs(
	args *LinkLayerDeviceArgs, pendingNames set.Strings,
) (_ *linkLayerDeviceDoc, err error) {
	defer errors.DeferredAnnotatef(&err, "invalid device %q", args.Name)

	if pendingNames.Contains(args.Name) {
		return nil, errors.NewNotValid(nil, "Name specified more than once")
	}

	return m.newLinkLayerDeviceDocFromArgs(args), nil
}

func parseLinkLayerDeviceParentNameAsGlobalKey(parentName string) (hostMachineID, parentDeviceName string, err error) {
	hostMachineID, parentDeviceName, canBeGlobalKey := parseLinkLayerDeviceGlobalKey(parentName)
	if !canBeGlobalKey {
		return "", "", nil
	} else if hostMachineID == "" {
		return "", "", errors.NotValidf("ParentName %q format", parentName)
	}
	return hostMachineID, parentDeviceName, nil
}

func (m *Machine) newLinkLayerDeviceDocFromArgs(args *LinkLayerDeviceArgs) *linkLayerDeviceDoc {
	linkLayerDeviceDocID := linkLayerDeviceDocIDFromName(m.st, m.doc.Id, args.Name)

	providerID := string(args.ProviderID)
	modelUUID := m.st.ModelUUID()

	return &linkLayerDeviceDoc{
		DocID:           linkLayerDeviceDocID,
		Name:            args.Name,
		ModelUUID:       modelUUID,
		MTU:             args.MTU,
		ProviderID:      providerID,
		MachineID:       m.doc.Id,
		Type:            args.Type,
		MACAddress:      args.MACAddress,
		IsAutoStart:     args.IsAutoStart,
		IsUp:            args.IsUp,
		ParentName:      args.ParentName,
		VirtualPortType: args.VirtualPortType,
	}
}

func (m *Machine) assertAliveOp() txn.Op {
	return txn.Op{
		C:      machinesC,
		Id:     m.doc.Id,
		Assert: isAliveDoc,
	}
}

func (m *Machine) setDevicesFromDocsOps(newDocs []linkLayerDeviceDoc) ([]txn.Op, error) {
	devices, closer := m.st.db().GetCollection(linkLayerDevicesC)
	defer closer()

	var ops []txn.Op
	for _, newDoc := range newDocs {
		var existingDoc linkLayerDeviceDoc
		if err := devices.FindId(newDoc.DocID).One(&existingDoc); err == mgo.ErrNotFound {
			// Device does not exist yet - insert it.
			insertOps, err := m.insertLinkLayerDeviceOps(&newDoc)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, insertOps...)
		} else if err == nil {
			// Device already exists - update what's possible.
			updateOps, err := m.updateLinkLayerDeviceOps(&existingDoc, &newDoc)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, updateOps...)
		} else {
			return nil, errors.Trace(err)
		}
	}
	return ops, nil
}

func (m *Machine) insertLinkLayerDeviceOps(newDoc *linkLayerDeviceDoc) ([]txn.Op, error) {
	var ops []txn.Op
	if newDoc.ProviderID != "" {
		id := network.Id(newDoc.ProviderID)
		ops = append(ops, m.st.networkEntityGlobalKeyOp("linklayerdevice", id))
	}
	return append(ops, insertLinkLayerDeviceDocOp(newDoc)), nil
}

func (m *Machine) parentDocIDFromDeviceDoc(doc *linkLayerDeviceDoc) (string, error) {
	hostMachineID, parentName, err := parseLinkLayerDeviceParentNameAsGlobalKey(doc.ParentName)
	if err != nil {
		return "", errors.Trace(err)
	}
	if parentName == "" {
		// doc.ParentName is not a global key, but on the same machine.
		return linkLayerDeviceDocIDFromName(m.st, m.doc.Id, doc.ParentName), nil
	}
	// doc.ParentName is a global key, on a different host machine.
	return m.st.docID(linkLayerDeviceGlobalKey(hostMachineID, parentName)), nil
}

func (m *Machine) updateLinkLayerDeviceOps(existingDoc, newDoc *linkLayerDeviceDoc) (ops []txn.Op, err error) {
	// None of the ops in this function are assert-only,
	// so callers can know if there are any changes by just checking len(ops).
	var newParentDocID string
	if newDoc.ParentName != "" {
		newParentDocID, err = m.parentDocIDFromDeviceDoc(newDoc)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	var existingParentDocID string
	if existingDoc.ParentName != "" {
		existingParentDocID, err = m.parentDocIDFromDeviceDoc(existingDoc)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if newParentDocID != "" && existingParentDocID != "" && newParentDocID != existingParentDocID {
		ops = append(ops,
			assertLinkLayerDeviceExistsOp(newParentDocID),
			assertLinkLayerDeviceExistsOp(existingParentDocID),
		)
	} else if newParentDocID != "" && existingParentDocID == "" {
		ops = append(ops, assertLinkLayerDeviceExistsOp(newParentDocID))
	} else if newParentDocID == "" && existingParentDocID != "" {
		ops = append(ops, assertLinkLayerDeviceExistsOp(existingParentDocID))
	}
	updateDeviceOp, deviceHasChanges := updateLinkLayerDeviceDocOp(existingDoc, newDoc)
	if deviceHasChanges {
		// we only include the op if it will actually change something
		ops = append(ops, updateDeviceOp)
	}

	if newDoc.ProviderID != "" {
		if existingDoc.ProviderID != "" && existingDoc.ProviderID != newDoc.ProviderID {
			return nil, errors.Errorf("cannot change ProviderID of link layer device %q", existingDoc.Name)
		}
		if existingDoc.ProviderID != newDoc.ProviderID {
			// Need to insert the new provider id in providerIDsC
			id := network.Id(newDoc.ProviderID)
			ops = append(ops, m.st.networkEntityGlobalKeyOp("linklayerdevice", id))
		}
	}
	return ops, nil
}

// LinkLayerDeviceAddress contains an IP address assigned to a link-layer
// device.
type LinkLayerDeviceAddress struct {
	// DeviceName is the name of the link-layer device that has this address.
	DeviceName string

	// ConfigMethod is the method used to configure this address.
	ConfigMethod network.AddressConfigType

	// ProviderID is the provider-specific ID of the address. Empty when not
	// supported. Cannot be changed once set to non-empty.
	ProviderID network.Id

	// ProviderNetworkID is the provider-specific network ID of the address.
	// It can be left empty if not supported or known.
	ProviderNetworkID network.Id

	// ProviderSubnetID is the provider-specific subnet ID to which the
	// device is attached.
	ProviderSubnetID network.Id

	// CIDRAddress is the IP address assigned to the device, in CIDR format
	// (e.g. 10.20.30.5/24 or fc00:1234::/64).
	CIDRAddress string

	// DNSServers contains a list of DNS nameservers to use, which can be empty.
	DNSServers []string

	// DNSSearchDomains contains a list of DNS domain names to qualify
	// hostnames, and can be empty.
	DNSSearchDomains []string

	// GatewayAddress is the address of the gateway to use, which can be empty.
	GatewayAddress string

	// IsDefaultGateway is set to true if this address on this device is the
	// default gw on a machine.
	IsDefaultGateway bool

	// Origin represents the authoritative source of the address.
	// it is set using precedence, with "provider" overriding "machine".
	// It is used to determine whether the address is no longer recognised
	// and is safe to remove.
	Origin network.Origin

	// IsSecondary if true, indicates that this address is
	// not the primary address associated with the NIC.
	IsSecondary bool
}

// TODO (manadart 2020-07-21): This is silly. We already received the args
// as an address/subnet pair and validated them when transforming them to
// the CIDRAddress. Now we unpack and validate again.
// When the old link-layer update logic is removed, just pass it all
// through as-is.
// This method can then be removed and previous callers
// then need not include an error return.
func (a *LinkLayerDeviceAddress) addressAndSubnet() (string, string, error) {
	ip, ipNet, err := net.ParseCIDR(a.CIDRAddress)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return ip.String(), ipNet.String(), nil
}

// SetDevicesAddresses sets the addresses of all devices in devicesAddresses,
// adding new or updating existing assignments as needed, in a single
// transaction. ProviderID field can be empty if not supported by the provider,
// but when set must be unique within the model. Errors are returned in the
// following cases:
//   - Machine is no longer alive or is missing;
//   - Subnet inferred from any CIDRAddress field in args is known but no longer
//     alive (no error reported if the CIDRAddress does not match a known subnet);
//   - Model no longer alive;
//   - errors.NotValidError, when any of the fields in args contain invalid values;
//   - errors.NotFoundError, when any DeviceName in args refers to unknown device;
//   - ErrProviderIDNotUnique, when one or more specified ProviderIDs are not unique.
//
// Deprecated: (manadart 2021-05-04) This method is only used by tests and is in
// the process of removal. Do not add new usages of it.
func (m *Machine) SetDevicesAddresses(devicesAddresses ...LinkLayerDeviceAddress) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set link-layer device addresses of machine %q", m.doc.Id)
	if len(devicesAddresses) == 0 {
		logger.Debugf(context.TODO(), "no device addresses to set")
		return nil
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		newDocs, err := m.prepareToSetDevicesAddresses(devicesAddresses)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if m.doc.Life != Alive {
			return nil, errors.Errorf("machine %q not alive", m.doc.Id)
		}

		setAddressesOps, err := m.setDevicesAddressesFromDocsOps(newDocs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(setAddressesOps) == 0 {
			logger.Debugf(context.TODO(), "no changes to DevicesAddresses for machine %q", m.Id())
			return nil, jujutxn.ErrNoOperations
		}
		return append([]txn.Op{m.assertAliveOp()}, setAddressesOps...), nil
	}
	if err := m.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *Machine) prepareToSetDevicesAddresses(devicesAddresses []LinkLayerDeviceAddress) ([]ipAddressDoc, error) {
	var pendingDocs []ipAddressDoc
	for _, args := range devicesAddresses {
		newDoc, err := m.prepareOneSetDevicesAddresses(&args)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pendingDocs = append(pendingDocs, *newDoc)
	}
	return pendingDocs, nil
}

func (m *Machine) prepareOneSetDevicesAddresses(args *LinkLayerDeviceAddress) (_ *ipAddressDoc, err error) {
	defer errors.DeferredAnnotatef(&err, "invalid address %q", args.CIDRAddress)

	if err := m.validateSetDevicesAddressesArgs(args); err != nil {
		return nil, errors.Trace(err)
	}
	return m.newIPAddressDocFromArgs(args)
}

func (m *Machine) validateSetDevicesAddressesArgs(args *LinkLayerDeviceAddress) error {
	if args.CIDRAddress == "" {
		return errors.NotValidf("empty CIDRAddress")
	}
	if _, _, err := net.ParseCIDR(args.CIDRAddress); err != nil {
		return errors.NewNotValid(err, "CIDRAddress")
	}

	if args.DeviceName == "" {
		return errors.NotValidf("empty DeviceName")
	}
	if !network.IsValidLinkLayerDeviceName(args.DeviceName) {
		logger.Warningf(context.TODO(),
			"address %q on machine %q has invalid device name %q (using anyway)",
			args.CIDRAddress, m.Id(), args.DeviceName,
		)
	}
	if err := m.verifyDeviceAlreadyExists(args.DeviceName); err != nil {
		return errors.Trace(err)
	}

	if args.GatewayAddress != "" {
		if ip := net.ParseIP(args.GatewayAddress); ip == nil {
			return errors.NotValidf("GatewayAddress %q", args.GatewayAddress)
		}
	}

	return nil
}

func (m *Machine) verifyDeviceAlreadyExists(deviceName string) error {
	if _, err := m.LinkLayerDevice(deviceName); errors.Is(err, errors.NotFound) {
		return errors.NotFoundf("DeviceName %q on machine %q", deviceName, m.Id())
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *Machine) newIPAddressDocFromArgs(args *LinkLayerDeviceAddress) (*ipAddressDoc, error) {
	ip, ipNet, err := net.ParseCIDR(args.CIDRAddress)
	if err != nil {
		// We already validated CIDRAddress earlier, so this cannot happen in
		// practice, but we handle it anyway.
		return nil, errors.Trace(err)
	}
	addressValue := ip.String()
	subnetCIDR := ipNet.String()

	globalKey := ipAddressGlobalKey(m.doc.Id, args.DeviceName, addressValue)
	ipAddressDocID := m.st.docID(globalKey)

	modelUUID := m.st.ModelUUID()

	newDoc := &ipAddressDoc{
		DocID:             ipAddressDocID,
		ModelUUID:         modelUUID,
		ProviderID:        args.ProviderID.String(),
		ProviderNetworkID: args.ProviderNetworkID.String(),
		ProviderSubnetID:  args.ProviderSubnetID.String(),
		DeviceName:        args.DeviceName,
		MachineID:         m.doc.Id,
		SubnetCIDR:        subnetCIDR,
		ConfigMethod:      args.ConfigMethod,
		Value:             addressValue,
		DNSServers:        args.DNSServers,
		DNSSearchDomains:  args.DNSSearchDomains,
		GatewayAddress:    args.GatewayAddress,
		IsDefaultGateway:  args.IsDefaultGateway,
		Origin:            args.Origin,
	}
	return newDoc, nil
}

func (m *Machine) setDevicesAddressesFromDocsOps(newDocs []ipAddressDoc) ([]txn.Op, error) {
	addresses, closer := m.st.db().GetCollection(ipAddressesC)
	defer closer()

	providerIDAddrs := make(map[string]string)

	var ops []txn.Op
	for _, newDoc := range newDocs {
		var thisDeviceOps []txn.Op
		hasChanges := false
		deviceDocID := linkLayerDeviceDocIDFromName(m.st, m.doc.Id, newDoc.DeviceName)
		thisDeviceOps = append(thisDeviceOps, assertLinkLayerDeviceExistsOp(deviceDocID))

		var existingDoc ipAddressDoc
		switch err := addresses.FindId(newDoc.DocID).One(&existingDoc); err {
		case mgo.ErrNotFound:
			// Address does not exist yet - insert it.
			hasChanges = true
			thisDeviceOps = append(thisDeviceOps, insertIPAddressDocOp(&newDoc))

			pIDOps, err := m.maybeAddAddressProviderIDOps(providerIDAddrs, newDoc, "")
			if err != nil {
				return nil, errors.Trace(err)
			}
			thisDeviceOps = append(thisDeviceOps, pIDOps...)

		case nil:
			// Address already exists - update what's possible.
			var ipOp txn.Op
			ipOp, hasChanges = updateIPAddressDocOp(&existingDoc, &newDoc)
			thisDeviceOps = append(thisDeviceOps, ipOp)

			pIDOps, err := m.maybeAddAddressProviderIDOps(providerIDAddrs, newDoc, existingDoc.ProviderID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if len(pIDOps) > 0 {
				hasChanges = true
				thisDeviceOps = append(thisDeviceOps, pIDOps...)
			}

		default:
			return nil, errors.Trace(err)
		}

		if hasChanges {
			ops = append(ops, thisDeviceOps...)
		}
	}
	return ops, nil
}

// maybeAddAddressProviderIDOps ensures that the address provider ID is valid
// and that we only accrue a transaction operation to add the ID if we have
// not already.
func (m *Machine) maybeAddAddressProviderIDOps(
	providerIDAddrs map[string]string, doc ipAddressDoc, oldProviderID string,
) ([]txn.Op, error) {
	if doc.ProviderID == "" {
		return nil, nil
	}
	if doc.ProviderID == oldProviderID {
		return nil, nil
	}
	if oldProviderID != "" {
		return nil, errors.Errorf("cannot change provider ID of link address %q", doc.Value)
	}

	// If this provider ID has been added for the same address,
	// it is valid, but do not attempt to add the ID again.
	// If we have different addresses with the same provider ID,
	// return an error.
	addr, ok := providerIDAddrs[doc.ProviderID]
	if ok {
		if addr == doc.Value {
			return nil, nil
		}
		return nil, errors.Annotatef(
			newProviderIDNotUniqueError(network.Id(doc.ProviderID)), "multiple addresses %q, %q", addr, doc.Value)
	}

	providerIDAddrs[doc.ProviderID] = doc.Value
	return []txn.Op{m.st.networkEntityGlobalKeyOp("address", network.Id(doc.ProviderID))}, nil
}

// RemoveAllAddresses removes all assigned addresses to all devices of the
// machine, in a single transaction. No error is returned when some or all of
// the addresses were already removed.
func (m *Machine) RemoveAllAddresses() error {
	ops, err := m.removeAllAddressesOps()
	if err != nil {
		return errors.Trace(err)
	}

	return m.st.db().RunTransaction(ops)
}

func (m *Machine) removeAllAddressesOps() ([]txn.Op, error) {
	findQuery := findAddressesQuery(m.doc.Id, "")
	return m.st.removeMatchingIPAddressesDocOps(findQuery)
}

// AllDeviceAddresses returns all known addresses assigned to
// link-layer devices on the machine.
func (m *Machine) AllDeviceAddresses() ([]*Address, error) {
	var allAddresses []*Address
	callbackFunc := func(doc *ipAddressDoc) {
		allAddresses = append(allAddresses, newIPAddress(m.st, *doc))
	}

	findQuery := findAddressesQuery(m.doc.Id, "")
	if err := m.st.forEachIPAddressDoc(findQuery, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allAddresses, nil
}

// AllSpaces returns the set of spaceIDs that this machine is
// actively connected to.
// TODO(jam): 2016-12-18 This should evolve to look at the
// LinkLayerDevices directly, instead of using the Addresses
// the devices are in to link back to spaces.
func (m *Machine) AllSpaces(allSubnets network.SubnetInfos) (set.Strings, error) {
	spaces := set.NewStrings()
	callback := func(doc *ipAddressDoc) {
		// Don't bother with these. They are not in a space.
		if doc.ConfigMethod == network.ConfigLoopback || doc.SubnetCIDR == "" {
			return
		}

		for _, sub := range allSubnets {
			if sub.CIDR == doc.SubnetCIDR {
				spaces.Add(sub.SpaceID.String())
				break
			}
		}
	}
	if err := m.st.forEachIPAddressDoc(findAddressesQuery(m.doc.Id, ""), callback); err != nil {
		return nil, errors.Trace(err)
	}

	return spaces, nil
}
