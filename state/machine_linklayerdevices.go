// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// LinkLayerDevice returns the link-layer device matching the given name. An
// error satisfying errors.IsNotFound() is returned when no such device exists
// on the machine.
func (m *Machine) LinkLayerDevice(name string) (*LinkLayerDevice, error) {
	linkLayerDevices, closer := m.st.getCollection(linkLayerDevicesC)
	defer closer()

	linkLayerDeviceDocID := m.linkLayerDeviceDocIDFromName(name)
	deviceAsString := m.deviceAsStringFromName(name)

	var doc linkLayerDeviceDoc
	err := linkLayerDevices.FindId(linkLayerDeviceDocID).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("%s", deviceAsString)
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot get %s", deviceAsString)
	}
	return newLinkLayerDevice(m.st, doc), nil
}

func (m *Machine) linkLayerDeviceDocIDFromName(deviceName string) string {
	return m.st.docID(m.linkLayerDeviceGlobalKeyFromName(deviceName))
}

func (m *Machine) linkLayerDeviceGlobalKeyFromName(deviceName string) string {
	return linkLayerDeviceGlobalKey(m.doc.Id, deviceName)
}

func (m *Machine) deviceAsStringFromName(deviceName string) string {
	return fmt.Sprintf("device %q on machine %q", deviceName, m.doc.Id)
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

func (m *Machine) forEachLinkLayerDeviceDoc(docFieldsToSelect bson.D, callbackFunc func(resultDoc *linkLayerDeviceDoc)) error {
	linkLayerDevices, closer := m.st.getCollection(linkLayerDevicesC)
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

// RemoveAllLinkLayerDevices removes all existing link-layer devices of the
// machine in a single transaction. No error is returned when some or all of the
// devices were already removed.
func (m *Machine) RemoveAllLinkLayerDevices() error {
	ops, err := m.removeAllLinkLayerDevicesOps()
	if err != nil {
		return errors.Trace(err)
	}

	return m.st.runTransaction(ops)
}

func (m *Machine) removeAllLinkLayerDevicesOps() ([]txn.Op, error) {
	var ops []txn.Op
	callbackFunc := func(resultDoc *linkLayerDeviceDoc) {
		removeOps := removeLinkLayerDeviceUnconditionallyOps(resultDoc.DocID)
		ops = append(ops, removeOps...)
	}

	selectDocIDOnly := bson.D{{"_id", 1}}
	if err := m.forEachLinkLayerDeviceDoc(selectDocIDOnly, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}

	return ops, nil
}

// LinkLayerDeviceArgs contains the arguments accepted by Machine.AddLinkLayerDevices().
type LinkLayerDeviceArgs struct {
	// Name is the name of the device as it appears on the machine.
	Name string

	// MTU is the maximum transmission unit the device can handle.
	MTU uint

	// ProviderID is a provider-specific ID of the device. Empty when not
	// supported by the provider. Cannot be cleared once set.
	ProviderID network.Id

	// Type is the type of the underlying link-layer device.
	Type LinkLayerDeviceType

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
}

// AddLinkLayerDevices creates one or more link-layer devices on the machine,
// each initialized from the items in the given devicesArgs, and using a single
// transaction for all. ProviderID field can be empty if not supported by the
// provider, but when set must be unique within the model. Errors are returned
// and no changes are applied, in the following cases:
// - Zero arguments given;
// - Machine is no longer alive or missing;
// - Model no longer alive;
// - errors.NotValidError, when any of the fields in args contain invalid values;
// - errors.NotFoundError, when ParentName is set but cannot be found on the
//   machine;
// - errors.AlreadyExistsError, when Name is set to an existing device.
// - ErrProviderIDNotUnique, when one or more specified ProviderIDs are not unique;
// Adding parent devices must be done in a separate call than adding their children
// on the same machine.
func (m *Machine) AddLinkLayerDevices(devicesArgs ...LinkLayerDeviceArgs) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add link-layer devices to machine %q", m.doc.Id)

	if len(devicesArgs) == 0 {
		return errors.Errorf("no devices to add")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		existingProviderIDs, err := m.st.allProviderIDsForModelLinkLayerDevices()
		if err != nil {
			return nil, errors.Trace(err)
		}
		newDocs, err := m.prepareToAddLinkLayerDevices(devicesArgs, existingProviderIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if attempt > 0 {
			if err := checkModeLife(m.st); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.isStillAlive(); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.areLinkLayerDeviceDocsStillValid(newDocs); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops := []txn.Op{
			assertModelAliveOp(m.st.ModelUUID()),
			m.assertAliveOp(),
		}

		for _, newDoc := range newDocs {
			insertOps, err := m.insertLinkLayerDeviceOps(&newDoc)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, insertOps...)
		}
		return ops, nil
	}
	if err := m.st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	// Even without an error, we still need to verify if any of the newDocs was
	// not inserted due to ProviderID unique index violation.
	return m.rollbackUnlessAllLinkLayerDevicesWithProviderIDAdded(devicesArgs)
}

func (st *State) allProviderIDsForModelLinkLayerDevices() (set.Strings, error) {
	return st.allProviderIDsForModelCollection(linkLayerDevicesC, "link-layer devices")
}

func (st *State) allProviderIDsForModelCollection(collectionName, entityLabelPlural string) (_ set.Strings, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot get ProviderIDs for all %s", entityLabelPlural)

	entities, closer := st.getCollection(collectionName)
	defer closer()

	allProviderIDs := set.NewStrings()
	var doc struct {
		ProviderID string `bson:"providerid"`
	}

	pattern := fmt.Sprintf("^%s:.+$", st.ModelUUID())
	modelProviderIDs := bson.D{{"providerid", bson.D{{"$regex", pattern}}}}
	onlyProviderIDField := bson.D{{"providerid", 1}}

	iter := entities.Find(modelProviderIDs).Select(onlyProviderIDField).Iter()
	for iter.Next(&doc) {
		localProviderID := st.localID(doc.ProviderID)
		allProviderIDs.Add(localProviderID)
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return allProviderIDs, nil
}

func (m *Machine) prepareToAddLinkLayerDevices(devicesArgs []LinkLayerDeviceArgs, existingProviderIDs set.Strings) ([]linkLayerDeviceDoc, error) {
	var pendingDocs []linkLayerDeviceDoc
	pendingNames := set.NewStrings()
	allProviderIDs := set.NewStrings(existingProviderIDs.Values()...)

	for _, args := range devicesArgs {
		newDoc, err := m.prepareOneAddLinkLayerDeviceArgs(&args, pendingNames, allProviderIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pendingNames.Add(args.Name)
		pendingDocs = append(pendingDocs, *newDoc)
		if args.ProviderID != "" {
			allProviderIDs.Add(string(args.ProviderID))
		}
	}
	return pendingDocs, nil
}

func (m *Machine) prepareOneAddLinkLayerDeviceArgs(args *LinkLayerDeviceArgs, pendingNames, allProviderIDs set.Strings) (_ *linkLayerDeviceDoc, err error) {
	defer errors.DeferredAnnotatef(&err, "invalid device %q", args.Name)

	if err := m.validateAddLinkLayerDeviceArgs(args); err != nil {
		return nil, errors.Trace(err)
	}

	if pendingNames.Contains(args.Name) {
		return nil, errors.NewNotValid(nil, "Name specified more than once")
	}

	if allProviderIDs.Contains(string(args.ProviderID)) {
		return nil, NewProviderIDNotUniqueError(args.ProviderID)
	}

	return m.newLinkLayerDeviceDocFromArgs(args), nil
}

func (m *Machine) validateAddLinkLayerDeviceArgs(args *LinkLayerDeviceArgs) error {
	if args.Name == "" {
		return errors.NotValidf("empty Name")
	}
	if !IsValidLinkLayerDeviceName(args.Name) {
		return errors.NotValidf("Name %q", args.Name)
	}

	if args.ParentName != "" {
		if err := m.validateLinkLayerDeviceParent(args); err != nil {
			return errors.Trace(err)
		}
	}

	if !IsValidLinkLayerDeviceType(string(args.Type)) {
		return errors.NotValidf("Type %q", args.Type)
	}

	if args.MACAddress != "" {
		if _, err := net.ParseMAC(args.MACAddress); err != nil {
			return errors.NotValidf("MACAddress %q", args.MACAddress)
		}
	}
	return nil
}

func (m *Machine) validateLinkLayerDeviceParent(args *LinkLayerDeviceArgs) error {
	hostMachineID, parentDeviceName, err := parseLinkLayerDeviceParentNameAsGlobalKey(args.ParentName)
	if err != nil {
		return errors.Trace(err)
	} else if hostMachineID == "" {
		// Not a global key, so validate as usual.
		return m.validateParentDeviceNameWhenNotAGlobalKey(args)
	}
	ourParentMachineID, hasParent := m.ParentId()
	if !hasParent {
		// Using global key for ParentName not allowed for non-container machine
		// devices.
		return errors.NotValidf("ParentName %q for non-container machine %q", args.ParentName, m.Id())
	}
	if hostMachineID != ourParentMachineID {
		// ParentName as global key only allowed when the key's machine ID is
		// the container's host machine.
		return errors.NotValidf("ParentName %q on non-host machine %q", args.ParentName, hostMachineID)
	}

	err = m.verifyHostMachineParentDeviceExistsAndIsABridgeDevice(hostMachineID, parentDeviceName)
	return errors.Trace(err)
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

func (m *Machine) verifyHostMachineParentDeviceExistsAndIsABridgeDevice(hostMachineID, parentDeviceName string) error {
	hostMachine, err := m.st.Machine(hostMachineID)
	if errors.IsNotFound(err) {
		return errors.NotFoundf("host machine %q of parent device %q", hostMachineID, parentDeviceName)
	} else if err != nil {
		return errors.Trace(err)
	}

	parentDevice, err := hostMachine.LinkLayerDevice(parentDeviceName)
	if errors.IsNotFound(err) {
		return errors.NotFoundf("parent device %q on host machine %q", parentDeviceName, hostMachineID)
	} else if err != nil {
		return errors.Trace(err)
	}

	if parentDevice.Type() != BridgeDevice {
		errorMessage := fmt.Sprintf(
			"parent device %q on host machine %q must be of type %q, not type %q",
			parentDeviceName, hostMachineID, BridgeDevice, parentDevice.Type(),
		)
		return errors.NewNotValid(nil, errorMessage)
	}
	return nil
}

func (m *Machine) validateParentDeviceNameWhenNotAGlobalKey(args *LinkLayerDeviceArgs) error {
	if !IsValidLinkLayerDeviceName(args.ParentName) {
		return errors.NotValidf("ParentName %q", args.ParentName)
	}
	if args.Name == args.ParentName {
		return errors.NewNotValid(nil, "Name and ParentName must be different")
	}
	return nil
}

func (m *Machine) newLinkLayerDeviceDocFromArgs(args *LinkLayerDeviceArgs) *linkLayerDeviceDoc {
	linkLayerDeviceDocID := m.linkLayerDeviceDocIDFromName(args.Name)

	providerID := string(args.ProviderID)
	if providerID != "" {
		providerID = m.st.docID(providerID)
	}

	modelUUID := m.st.ModelUUID()

	return &linkLayerDeviceDoc{
		DocID:       linkLayerDeviceDocID,
		Name:        args.Name,
		ModelUUID:   modelUUID,
		MTU:         args.MTU,
		ProviderID:  providerID,
		MachineID:   m.doc.Id,
		Type:        args.Type,
		MACAddress:  args.MACAddress,
		IsAutoStart: args.IsAutoStart,
		IsUp:        args.IsUp,
		ParentName:  args.ParentName,
	}
}

func (m *Machine) isStillAlive() error {
	if machineAlive, err := isAlive(m.st, machinesC, m.doc.Id); err != nil {
		return errors.Trace(err)
	} else if !machineAlive {
		return errors.Errorf("machine not found or not alive")
	}
	return nil
}

func (m *Machine) areLinkLayerDeviceDocsStillValid(newDocs []linkLayerDeviceDoc) error {
	for _, newDoc := range newDocs {
		if newDoc.ParentName != "" {
			if err := m.verifyParentDeviceExists(newDoc.Name, newDoc.ParentName); err != nil {
				return errors.Trace(err)
			}
		}
		if err := m.verifyDeviceDoesNotExistYet(newDoc.Name); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (m *Machine) verifyParentDeviceExists(name, parentName string) error {
	if _, err := m.LinkLayerDevice(parentName); errors.IsNotFound(err) {
		return errors.NotFoundf("parent device %q of device %q", parentName, name)
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *Machine) verifyDeviceDoesNotExistYet(deviceName string) error {
	if _, err := m.LinkLayerDevice(deviceName); err == nil {
		return errors.AlreadyExistsf("device %q", deviceName)
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	return nil
}

func (m *Machine) assertAliveOp() txn.Op {
	return txn.Op{
		C:      machinesC,
		Id:     m.doc.Id,
		Assert: isAliveDoc,
	}
}

func (m *Machine) insertLinkLayerDeviceOps(newDoc *linkLayerDeviceDoc) ([]txn.Op, error) {
	modelUUID, linkLayerDeviceDocID := newDoc.ModelUUID, newDoc.DocID

	var ops []txn.Op
	if newDoc.ParentName != "" {
		var parentDocID string

		hostMachineID, parentDeviceName, err := parseLinkLayerDeviceParentNameAsGlobalKey(newDoc.ParentName)
		if err != nil {
			return nil, errors.Trace(err)
		} else if hostMachineID != "" {
			parentGlobalKey := linkLayerDeviceGlobalKey(hostMachineID, parentDeviceName)
			parentDocID = m.st.docID(parentGlobalKey)
		} else {
			parentDocID = m.linkLayerDeviceDocIDFromName(newDoc.ParentName)
		}

		ops = append(ops, incrementDeviceNumChildrenOp(parentDocID))
	}
	return append(ops,
		insertLinkLayerDeviceDocOp(newDoc),
		insertLinkLayerDevicesRefsOp(modelUUID, linkLayerDeviceDocID),
	), nil
}

// rollbackUnlessAllLinkLayerDevicesWithProviderIDAdded prepares a transaction
// to verify any devices with ProviderID specified in devicesArgs were addded
// successfully. If any device is missing due to an unique ProviderID index
// violation, all devices in devicesArgs will be removed in a single
// transactions.
func (m *Machine) rollbackUnlessAllLinkLayerDevicesWithProviderIDAdded(devicesArgs []LinkLayerDeviceArgs) error {
	usedProviderIDs := set.NewStrings()
	allDevicesDocIDs := set.NewStrings()
	var assertAllWithProviderIDAddedOps []txn.Op

	for _, args := range devicesArgs {
		linkLayerDeviceDocID := m.linkLayerDeviceDocIDFromName(args.Name)
		allDevicesDocIDs.Add(linkLayerDeviceDocID)

		if args.ProviderID == "" {
			continue
		}
		usedProviderIDs.Add(string(args.ProviderID))
		assertAllWithProviderIDAddedOps = append(
			assertAllWithProviderIDAddedOps,
			assertLinkLayerDeviceExistsOp(linkLayerDeviceDocID),
		)
	}
	if len(assertAllWithProviderIDAddedOps) == 0 {
		// No devices added with ProviderID, so no chance for some of them to
		// have caused unique ProviderID index violation.
		return nil
	}

	var oneOrMoreWithProviderIDNotAdded bool
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// When one or more documents were not inserted due to ProviderID
			// unique index violation, rollback to the state before
			// AddLinkLayerDevices() was called, removing any successfully added
			// devices.
			oneOrMoreWithProviderIDNotAdded = true
			var removeAllAddedOps []txn.Op
			for _, docID := range allDevicesDocIDs.Values() {
				removeAllAddedOps = append(removeAllAddedOps, removeLinkLayerDeviceUnconditionallyOps(docID)...)
			}
			return removeAllAddedOps, nil
		}
		return assertAllWithProviderIDAddedOps, nil
	}
	err := m.st.run(buildTxn)
	if err == nil && oneOrMoreWithProviderIDNotAdded {
		return newProviderIDNotUniqueErrorFromStrings(usedProviderIDs.SortedValues())
	}
	return errors.Trace(err)
}

// LinkLayerDeviceAddress contains an IP address assigned to a link-layer
// device.
type LinkLayerDeviceAddress struct {
	// DeviceName is the name of the link-layer device that has this address.
	DeviceName string

	// ConfigMethod is the method used to configure this address.
	ConfigMethod AddressConfigMethod

	// ProviderID is the provider-specific ID of the address. Empty when not
	// supported.
	ProviderID network.Id

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
}

const (
	loopbackIPv4CIDR = "127.0.0.0/8"
	loopbackIPv6CIDR = "::1/128"
)

// TODO: Temporary helper used to test the addresses methods apart from
// SetDevicesAddresses. Remove once the later is implemented.
func (dev *LinkLayerDevice) AddAddress(address LinkLayerDeviceAddress) (*Address, error) {
	ipAddress, ipNet, err := net.ParseCIDR(address.CIDRAddress)
	if err != nil {
		return nil, errors.Trace(err)
	}
	addressValue := ipAddress.String()
	subnetID := ipNet.String()
	if subnetID == loopbackIPv4CIDR || subnetID == loopbackIPv6CIDR {
		// Loopback addresses are not linked to a subnet.
		subnetID = ""
	}

	globalKey := ipAddressGlobalKey(dev.doc.MachineID, address.DeviceName, addressValue)
	ipAddressDocID := dev.st.docID(globalKey)

	providerID := string(address.ProviderID)
	if providerID != "" {
		providerID = dev.st.docID(providerID)
	}

	newDoc := &ipAddressDoc{
		DocID:            ipAddressDocID,
		ModelUUID:        dev.st.ModelUUID(),
		ProviderID:       providerID,
		DeviceName:       address.DeviceName,
		MachineID:        dev.doc.MachineID,
		SubnetID:         subnetID,
		ConfigMethod:     address.ConfigMethod,
		Value:            addressValue,
		DNSServers:       address.DNSServers,
		DNSSearchDomains: address.DNSSearchDomains,
		GatewayAddress:   address.GatewayAddress,
	}

	ops := []txn.Op{
		insertIPAddressDocOp(newDoc),
	}

	newAddress := newIPAddress(dev.st, *newDoc)
	err = onAbort(dev.st.runTransaction(ops), errors.AlreadyExistsf("%s", newAddress))
	if err == nil {
		return newAddress, nil
	}
	return nil, errors.Trace(err)
}

// SetDevicesAddresses sets the addresses of all devices in devicesAddresses,
// replacing existing assignments as needed, in a single transaction. ProviderID
// field can be empty if not supported by the provider, but when set must be
// unique within the model. Errors are returned and no changes are applied, in
// the following cases:
// - Zero arguments given;
// - Machine is no longer alive or missing;
// - Model no longer alive;
// - errors.NotValidError, when any of the fields in args contain invalid values;
// - ErrProviderIDNotUnique, when one or more specified ProviderIDs are not unique.
func (m *Machine) SetDevicesAddresses(devicesAddresses ...LinkLayerDeviceAddress) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set link-layer device addresses of machine %q", m.doc.Id)

	if len(devicesAddresses) == 0 {
		return errors.Errorf("no addresses to add")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		existingProviderIDs, err := m.st.allProviderIDsForModelIPAddresses()
		if err != nil {
			return nil, errors.Trace(err)
		}
		newDocs, err := m.prepareToSetDevicesAddresses(devicesAddresses, existingProviderIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if attempt > 0 {
			if err := checkModeLife(m.st); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.isStillAlive(); err != nil {
				return nil, errors.Trace(err)
			}
			//if err := m.areIPAddressDocsStillValid(newDocs); err != nil {
			//	return nil, errors.Trace(err)
			//}
		}

		ops := []txn.Op{
			assertModelAliveOp(m.st.ModelUUID()),
			m.assertAliveOp(),
		}

		for _, newDoc := range newDocs {
			ops = append(ops, insertIPAddressDocOp(&newDoc))
		}
		return ops, nil
	}
	if err := m.st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
	// Even without an error, we still need to verify if any of the newDocs was
	// not inserted due to ProviderID unique index violation.
	//return m.rollbackUnlessAllLinkLayerDevicesWithProviderIDAdded(devicesArgs)
}

func (st *State) allProviderIDsForModelIPAddresses() (set.Strings, error) {
	return st.allProviderIDsForModelCollection(ipAddressesC, "IP addresses")
}

func (m *Machine) prepareToSetDevicesAddresses(devicesAddresses []LinkLayerDeviceAddress, existingProviderIDs set.Strings) ([]ipAddressDoc, error) {
	var pendingDocs []ipAddressDoc
	allProviderIDs := set.NewStrings(existingProviderIDs.Values()...)

	for _, args := range devicesAddresses {
		newDoc, err := m.prepareOneSetDevicesAddresses(&args, allProviderIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pendingDocs = append(pendingDocs, *newDoc)
		if args.ProviderID != "" {
			allProviderIDs.Add(string(args.ProviderID))
		}
	}
	return pendingDocs, nil
}

func (m *Machine) prepareOneSetDevicesAddresses(args *LinkLayerDeviceAddress, allProviderIDs set.Strings) (_ *ipAddressDoc, err error) {
	defer errors.DeferredAnnotatef(&err, "invalid address %q", args.CIDRAddress)

	if err := m.validateSetDevicesAddressesArgs(args); err != nil {
		return nil, errors.Trace(err)
	}

	if allProviderIDs.Contains(string(args.ProviderID)) {
		return nil, NewProviderIDNotUniqueError(args.ProviderID)
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
	if !IsValidLinkLayerDeviceName(args.DeviceName) {
		return errors.NotValidf("DeviceName %q", args.DeviceName)
	}

	if !IsValidAddressConfigMethod(string(args.ConfigMethod)) {
		return errors.NotValidf("ConfigMethod %q", args.ConfigMethod)
	}

	return nil
}

func (m *Machine) newIPAddressDocFromArgs(args *LinkLayerDeviceAddress) (*ipAddressDoc, error) {
	// Ignoring the error below, as we have already checked earlier CIDRAddress
	// parses OK.
	ip, ipNet, _ := net.ParseCIDR(args.CIDRAddress)
	addressValue := ip.String()
	subnetID := ipNet.String()
	if subnetID == loopbackIPv4CIDR || subnetID == loopbackIPv6CIDR {
		// Loopback addresses are not linked to a subnet.
		subnetID = ""
	}

	globalKey := ipAddressGlobalKey(m.doc.Id, args.DeviceName, addressValue)
	ipAddressDocID := m.st.docID(globalKey)

	providerID := string(args.ProviderID)
	if providerID != "" {
		providerID = m.st.docID(providerID)
	}
	modelUUID := m.st.ModelUUID()

	if _, err := m.st.Subnet(subnetID); err != nil {
		return nil, errors.Trace(err)
	}

	newDoc := &ipAddressDoc{
		DocID:            ipAddressDocID,
		ModelUUID:        modelUUID,
		ProviderID:       providerID,
		DeviceName:       args.DeviceName,
		MachineID:        m.doc.Id,
		SubnetID:         subnetID,
		ConfigMethod:     args.ConfigMethod,
		Value:            addressValue,
		DNSServers:       args.DNSServers,
		DNSSearchDomains: args.DNSSearchDomains,
		GatewayAddress:   args.GatewayAddress,
	}
	return newDoc, nil
}

// RemoveAllAddresses removes all assigned addresses to all devices of the
// machine, in a single transaction. No error is returned when some or all of
// the addresses were already removed.
func (m *Machine) RemoveAllAddresses() error {
	ops, err := m.removeAllAddressesOps()
	if err != nil {
		return errors.Trace(err)
	}

	return m.st.runTransaction(ops)
}

func (m *Machine) removeAllAddressesOps() ([]txn.Op, error) {
	findQuery := findAddressesQuery(m.doc.Id, "")
	return m.st.removeMatchingIPAddressesDocOps(findQuery)
}

// AllAddresses returns the all addresses assigned to all devices of the
// machine.
func (m *Machine) AllAddresses() ([]*Address, error) {
	var allAddresses []*Address
	callbackFunc := func(resultDoc *ipAddressDoc) {
		allAddresses = append(allAddresses, newIPAddress(m.st, *resultDoc))
	}

	findQuery := findAddressesQuery(m.doc.Id, "")
	if err := m.st.forEachIPAddressDoc(findQuery, nil, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allAddresses, nil
}
