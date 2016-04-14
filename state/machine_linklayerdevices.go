// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"math/rand"
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

// SetLinkLayerDevices sets link-layer devices on the machine, adding or
// updating existing devices as needed, in a single transaction. ProviderID
// field can be empty if not supported by the provider, but when set must be
// unique within the model, and cannot be unset once set. Errors are returned in
// the following cases:
// - Machine is no longer alive or is missing;
// - Model no longer alive;
// - errors.NotValidError, when any of the fields in args contain invalid values;
// - ErrProviderIDNotUnique, when one or more specified ProviderIDs are not unique;
// Setting new parent devices must be done in a separate call than setting their
// children on the same machine.
func (m *Machine) SetLinkLayerDevices(devicesArgs ...LinkLayerDeviceArgs) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set link-layer devices to machine %q", m.doc.Id)

	if len(devicesArgs) == 0 {
		logger.Warningf("no device addresses to set")
		return nil
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		existingProviderIDs, err := m.st.allProviderIDsForModelLinkLayerDevices()
		if err != nil {
			return nil, errors.Trace(err)
		}
		newDocs, err := m.prepareToSetLinkLayerDevices(devicesArgs, existingProviderIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if attempt > 0 {
			if err := checkModelActive(m.st); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.isStillAlive(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops := []txn.Op{
			assertModelActiveOp(m.st.ModelUUID()),
			m.assertAliveOp(),
		}

		setDevicesOps, err := m.setDevicesFromDocsOps(newDocs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return append(ops, setDevicesOps...), nil
	}
	if err := m.st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
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

func (m *Machine) prepareToSetLinkLayerDevices(devicesArgs []LinkLayerDeviceArgs, existingProviderIDs set.Strings) ([]linkLayerDeviceDoc, error) {
	var pendingDocs []linkLayerDeviceDoc
	pendingNames := set.NewStrings()
	allProviderIDs := set.NewStrings(existingProviderIDs.Values()...)

	for _, args := range devicesArgs {
		newDoc, err := m.prepareOneSetLinkLayerDeviceArgs(&args, pendingNames, allProviderIDs)
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

func (m *Machine) prepareOneSetLinkLayerDeviceArgs(args *LinkLayerDeviceArgs, pendingNames, allProviderIDs set.Strings) (_ *linkLayerDeviceDoc, err error) {
	defer errors.DeferredAnnotatef(&err, "invalid device %q", args.Name)

	if err := m.validateSetLinkLayerDeviceArgs(args); err != nil {
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

func (m *Machine) validateSetLinkLayerDeviceArgs(args *LinkLayerDeviceArgs) error {
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
		if err := m.validateParentDeviceNameWhenNotAGlobalKey(args); errors.IsNotFound(err) {
			return errors.NewNotValid(err, "ParentName not valid")
		} else if err != nil {
			return errors.Trace(err)
		}
		return nil
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
	if errors.IsNotFound(err) || err == nil && hostMachine.Life() != Alive {
		return errors.Errorf("host machine %q of parent device %q not found or not alive", hostMachineID, parentDeviceName)
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
	if err := m.verifyParentDeviceExists(args.ParentName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *Machine) verifyParentDeviceExists(parentName string) error {
	if _, err := m.LinkLayerDevice(parentName); err != nil {
		return errors.Trace(err)
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

func (m *Machine) assertAliveOp() txn.Op {
	return txn.Op{
		C:      machinesC,
		Id:     m.doc.Id,
		Assert: isAliveDoc,
	}
}

func (m *Machine) setDevicesFromDocsOps(newDocs []linkLayerDeviceDoc) ([]txn.Op, error) {
	devices, closer := m.st.getCollection(linkLayerDevicesC)
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
	modelUUID, linkLayerDeviceDocID := newDoc.ModelUUID, newDoc.DocID

	var ops []txn.Op
	if newDoc.ParentName != "" {
		newParentDocID, err := m.parentDocIDFromDeviceDoc(newDoc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if newParentDocID != "" {
			ops = append(ops, assertLinkLayerDeviceExistsOp(newParentDocID))
			ops = append(ops, incrementDeviceNumChildrenOp(newParentDocID))
		}
	}
	return append(ops,
		insertLinkLayerDeviceDocOp(newDoc),
		insertLinkLayerDevicesRefsOp(modelUUID, linkLayerDeviceDocID),
	), nil
}

func (m *Machine) parentDocIDFromDeviceDoc(doc *linkLayerDeviceDoc) (string, error) {
	hostMachineID, parentName, err := parseLinkLayerDeviceParentNameAsGlobalKey(doc.ParentName)
	if err != nil {
		return "", errors.Trace(err)
	}
	if parentName == "" {
		// doc.ParentName is not a global key, but on the same machine.
		return m.linkLayerDeviceDocIDFromName(doc.ParentName), nil
	}
	// doc.ParentName is a global key, on a different host machine.
	return m.st.docID(linkLayerDeviceGlobalKey(hostMachineID, parentName)), nil
}

func (m *Machine) updateLinkLayerDeviceOps(existingDoc, newDoc *linkLayerDeviceDoc) (ops []txn.Op, err error) {
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
			incrementDeviceNumChildrenOp(newParentDocID),
			assertLinkLayerDeviceExistsOp(existingParentDocID),
			decrementDeviceNumChildrenOp(existingParentDocID),
		)
	} else if newParentDocID != "" && existingParentDocID == "" {
		ops = append(ops, assertLinkLayerDeviceExistsOp(newParentDocID))
		ops = append(ops, incrementDeviceNumChildrenOp(newParentDocID))
	} else if newParentDocID == "" && existingParentDocID != "" {
		ops = append(ops, assertLinkLayerDeviceExistsOp(existingParentDocID))
		ops = append(ops, decrementDeviceNumChildrenOp(existingParentDocID))
	}
	return append(ops, updateLinkLayerDeviceDocOp(existingDoc, newDoc)), nil
}

// LinkLayerDeviceAddress contains an IP address assigned to a link-layer
// device.
type LinkLayerDeviceAddress struct {
	// DeviceName is the name of the link-layer device that has this address.
	DeviceName string

	// ConfigMethod is the method used to configure this address.
	ConfigMethod AddressConfigMethod

	// ProviderID is the provider-specific ID of the address. Empty when not
	// supported. Cannot be changed once set to non-empty.
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

// SetDevicesAddresses sets the addresses of all devices in devicesAddresses,
// adding new or updating existing assignments as needed, in a single
// transaction. ProviderID field can be empty if not supported by the provider,
// but when set must be unique within the model. Errors are returned in the
// following cases:
// - Machine is no longer alive or is missing;
// - Subnet inferred from any CIDRAddress field in args is known but no longer
//   alive (no error reported if the CIDRAddress matches IPv4 or IPv6 loopback
//   range or an unknown subnet);
// - Model no longer alive;
// - errors.NotValidError, when any of the fields in args contain invalid values;
// - errors.NotFoundError, when any DeviceName in args refers to unknown device;
// - ErrProviderIDNotUnique, when one or more specified ProviderIDs are not unique.
func (m *Machine) SetDevicesAddresses(devicesAddresses ...LinkLayerDeviceAddress) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set link-layer device addresses of machine %q", m.doc.Id)

	if len(devicesAddresses) == 0 {
		logger.Warningf("no device addresses to set")
		return nil
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
			if err := checkModelActive(m.st); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.isStillAlive(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops := []txn.Op{
			assertModelActiveOp(m.st.ModelUUID()),
			m.assertAliveOp(),
		}

		setAddressesOps, err := m.setDevicesAddressesFromDocsOps(newDocs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return append(ops, setAddressesOps...), nil
	}
	if err := m.st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
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
	if err := m.verifyDeviceAlreadyExists(args.DeviceName); err != nil {
		return errors.Trace(err)
	}

	if !IsValidAddressConfigMethod(string(args.ConfigMethod)) {
		return errors.NotValidf("ConfigMethod %q", args.ConfigMethod)
	}

	if args.GatewayAddress != "" {
		if ip := net.ParseIP(args.GatewayAddress); ip == nil {
			return errors.NotValidf("GatewayAddress %q", args.GatewayAddress)
		}
	}

	return nil
}

func (m *Machine) verifyDeviceAlreadyExists(deviceName string) error {
	if _, err := m.LinkLayerDevice(deviceName); errors.IsNotFound(err) {
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
	subnetID := ipNet.String()
	if subnetID == network.LoopbackIPv4CIDR ||
		subnetID == network.LoopbackIPv6CIDR {
		// Loopback addresses are not linked to a subnet.
		subnetID = ""
	} else {
		if err := m.verifyAddressSubnetAliveIfKnownWhenSet(subnetID); errors.IsNotFound(err) {
			logger.Infof("address %q on machine %q uses unknown subnet %q", addressValue, m.Id(), subnetID)
			subnetID = ""
		} else if err != nil {
			return nil, errors.Trace(err)
		}
	}

	globalKey := ipAddressGlobalKey(m.doc.Id, args.DeviceName, addressValue)
	ipAddressDocID := m.st.docID(globalKey)

	providerID := string(args.ProviderID)
	if providerID != "" {
		providerID = m.st.docID(providerID)
	}
	modelUUID := m.st.ModelUUID()

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

func (m *Machine) verifyAddressSubnetAliveIfKnownWhenSet(subnetID string) error {
	if subnetID == "" {
		return nil
	}

	subnet, err := m.st.Subnet(subnetID)
	if err != nil {
		return errors.Trace(err)
	}
	if subnet.Life() != Alive {
		return errors.Errorf("subnet %q is not alive", subnetID)
	}
	return nil
}

func (m *Machine) setDevicesAddressesFromDocsOps(newDocs []ipAddressDoc) ([]txn.Op, error) {
	addresses, closer := m.st.getCollection(ipAddressesC)
	defer closer()

	var ops []txn.Op

	for _, newDoc := range newDocs {
		deviceDocID := m.linkLayerDeviceDocIDFromName(newDoc.DeviceName)
		ops = append(ops, assertLinkLayerDeviceExistsOp(deviceDocID))

		var existingDoc ipAddressDoc
		err := addresses.FindId(newDoc.DocID).One(&existingDoc)
		if err == mgo.ErrNotFound {
			// Address does not exist yet - insert it.
			ops = append(ops, insertIPAddressDocOp(&newDoc))
		} else if err == nil {
			// Address already exists - update what's possible.
			ops = append(ops, updateIPAddressDocOp(&existingDoc, &newDoc))
		} else {
			return nil, errors.Trace(err)
		}

		ops = m.assertSubnetAliveWhenSetOps(&newDoc, ops)
	}
	return ops, nil
}

func (m *Machine) assertSubnetAliveWhenSetOps(newDoc *ipAddressDoc, opsSoFar []txn.Op) []txn.Op {
	if newDoc.SubnetID != "" {
		subnetDocID := m.st.docID(newDoc.SubnetID)
		return append(opsSoFar, txn.Op{
			C:      subnetsC,
			Id:     subnetDocID,
			Assert: isAliveDoc,
		})
	}
	return opsSoFar
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

// SetParentLinkLayerDevicesBeforeTheirChildren splits the given devicesArgs
// into multiple sets of args and calls SetLinkLayerDevices() for each set, such
// that child devices are set only after their parents.
func (m *Machine) SetParentLinkLayerDevicesBeforeTheirChildren(devicesArgs []LinkLayerDeviceArgs) error {
	seenNames := set.NewStrings("") // sentinel for empty ParentName.
	for {
		argsToSet := []LinkLayerDeviceArgs{}
		for _, args := range devicesArgs {
			if seenNames.Contains(args.Name) {
				// Already added earlier.
				continue
			}
			if seenNames.Contains(args.ParentName) {
				argsToSet = append(argsToSet, args)
			}
		}
		if len(argsToSet) == 0 {
			// We're done.
			break
		}
		logger.Debugf("setting link-layer devices %+v", argsToSet)
		if err := m.SetLinkLayerDevices(argsToSet...); IsProviderIDNotUniqueError(err) {
			// FIXME: Make updating devices with unchanged ProviderID idempotent.
			for i, args := range argsToSet {
				args.ProviderID = ""
				argsToSet[i] = args
			}
			if err := m.SetLinkLayerDevices(argsToSet...); err != nil {
				return errors.Trace(err)
			}
		} else if err != nil {
			return errors.Trace(err)
		}
		for _, args := range argsToSet {
			seenNames.Add(args.Name)
		}
	}
	return nil
}

// SetDevicesAddressesIdempotently calls SetDevicesAddresses() and if it fails
// with ErrProviderIDNotUnique, retries the call with all ProviderID fields in
// devicesAddresses set to empty.
func (m *Machine) SetDevicesAddressesIdempotently(devicesAddresses []LinkLayerDeviceAddress) error {
	if err := m.SetDevicesAddresses(devicesAddresses...); IsProviderIDNotUniqueError(err) {
		// FIXME: Make updating addresses with unchanged ProviderID idempotent.
		for i, args := range devicesAddresses {
			args.ProviderID = ""
			devicesAddresses[i] = args
		}
		if err := m.SetDevicesAddresses(devicesAddresses...); err != nil {
			return errors.Trace(err)
		}
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetContainerLinkLayerDevices sets the link-layer devices of the given
// containerMachine, setting each device linked to the corresponding
// BridgeDevice of the host machine m.
func (m *Machine) SetContainerLinkLayerDevices(containerMachine *Machine) error {
	allDevices, err := m.AllLinkLayerDevices()
	if err != nil {
		return errors.Annotate(err, "cannot get host machine devices")
	}
	logger.Debugf("using host machine %q devices: %+v", m.Id(), allDevices)

	var containerDevicesArgs []LinkLayerDeviceArgs
	for _, hostDevice := range allDevices {
		if hostDevice.Type() == BridgeDevice {
			containerDeviceName := fmt.Sprintf("eth%d", len(containerDevicesArgs))
			generatedMAC := generateMACAddress()
			args := LinkLayerDeviceArgs{
				Name:        containerDeviceName,
				Type:        EthernetDevice,
				MACAddress:  generatedMAC,
				MTU:         hostDevice.MTU(),
				IsUp:        true,
				IsAutoStart: true,
				ParentName:  hostDevice.globalKey(),
			}
			containerDevicesArgs = append(containerDevicesArgs, args)
		}
	}
	logger.Debugf("prepared container %q network config: %+v", containerMachine.Id(), containerDevicesArgs)

	if err := containerMachine.SetLinkLayerDevices(containerDevicesArgs...); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("container %q network config set", containerMachine.Id())
	return nil
}

// MACAddressTemplate is used to generate a unique MAC address for a
// container. Every '%x' is replaced by a random hexadecimal digit,
// while the rest is kept as-is.
const macAddressTemplate = "00:16:3e:%02x:%02x:%02x"

// generateMACAddress creates a random MAC address within the space defined by
// macAddressTemplate above.
//
// TODO(dimitern): We should make a best effort to ensure the MAC address we
// generate is unique at least within the current environment.
func generateMACAddress() string {
	digits := make([]interface{}, 3)
	for i := range digits {
		digits[i] = rand.Intn(256)
	}
	return fmt.Sprintf(macAddressTemplate, digits...)
}
