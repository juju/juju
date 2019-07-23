// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

// LinkLayerDevice returns the link-layer device matching the given name. An
// error satisfying errors.IsNotFound() is returned when no such device exists
// on the machine.
func (m *Machine) LinkLayerDevice(name string) (*LinkLayerDevice, error) {
	linkLayerDevices, closer := m.st.db().GetCollection(linkLayerDevicesC)
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
		result[i].MACAddress = device.MACAddress()
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

	return m.st.db().RunTransaction(ops)
}

func (m *Machine) removeAllLinkLayerDevicesOps() ([]txn.Op, error) {
	var ops []txn.Op
	callbackFunc := func(resultDoc *linkLayerDeviceDoc) {
		removeOps := removeLinkLayerDeviceUnconditionallyOps(resultDoc.DocID)
		ops = append(ops, removeOps...)
		if resultDoc.ProviderID != "" {
			providerId := corenetwork.Id(resultDoc.ProviderID)
			op := m.st.networkEntityGlobalKeyRemoveOp("linklayerdevice", providerId)
			ops = append(ops, op)
		}
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
	ProviderID corenetwork.Id

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
		logger.Debugf("no device addresses to set")
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
			if err := m.isStillAlive(); err != nil {
				return nil, errors.Trace(err)
			}
			allIds, err := m.st.allProviderIDsForLinkLayerDevices()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, args := range devicesArgs {
				if allIds.Contains(string(args.ProviderID)) {
					err := NewProviderIDNotUniqueError(args.ProviderID)
					return nil, errors.Annotatef(err, "invalid device %q", args.Name)
				}
			}
		}

		// We've checked the model is alive directly, and we assert the machine is alive, we don't need to also
		// assert the model is alive, because then the machine would be dying as well.
		ops := []txn.Op{
			m.assertAliveOp(),
		}

		setDevicesOps, err := m.setDevicesFromDocsOps(newDocs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(setDevicesOps) == 0 {
			// No need to assert only that the machine is alive
			logger.Debugf("no changes to LinkLayerDevices for machine %q", m.Id())
			return nil, jujutxn.ErrNoOperations
		}
		return append(ops, setDevicesOps...), nil
	}
	if err := m.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *State) allProviderIDsForLinkLayerDevices() (set.Strings, error) {
	return st.allProviderIDsForEntity("linklayerdevice")
}

func (st *State) allProviderIDsForAddresses() (set.Strings, error) {
	return st.allProviderIDsForEntity("address")
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

func (m *Machine) prepareOneSetLinkLayerDeviceArgs(args *LinkLayerDeviceArgs, pendingNames set.Strings) (_ *linkLayerDeviceDoc, err error) {
	defer errors.DeferredAnnotatef(&err, "invalid device %q", args.Name)

	if err := m.validateSetLinkLayerDeviceArgs(args); err != nil {
		return nil, errors.Trace(err)
	}

	if pendingNames.Contains(args.Name) {
		return nil, errors.NewNotValid(nil, "Name specified more than once")
	}

	return m.newLinkLayerDeviceDocFromArgs(args), nil
}

func (m *Machine) validateSetLinkLayerDeviceArgs(args *LinkLayerDeviceArgs) error {
	if args.Name == "" {
		return errors.NotValidf("empty Name")
	}
	if !IsValidLinkLayerDeviceName(args.Name) {
		logger.Warningf(
			"link-layer device %q on machine %q has invalid name (using anyway)",
			args.Name, m.Id(),
		)
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
		logger.Warningf(
			"parent link-layer device %q on machine %q has invalid name (using anyway)",
			args.ParentName, m.Id(),
		)
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
	if newDoc.ProviderID != "" {
		id := corenetwork.Id(newDoc.ProviderID)
		ops = append(ops, m.st.networkEntityGlobalKeyOp("linklayerdevice", id))
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
	// none of the ops in this function are assert-only, so callers can know if there are any changes by just checking len(ops)
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
			id := corenetwork.Id(newDoc.ProviderID)
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
	ConfigMethod AddressConfigMethod

	// ProviderID is the provider-specific ID of the address. Empty when not
	// supported. Cannot be changed once set to non-empty.
	ProviderID corenetwork.Id

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
}

// SetDevicesAddresses sets the addresses of all devices in devicesAddresses,
// adding new or updating existing assignments as needed, in a single
// transaction. ProviderID field can be empty if not supported by the provider,
// but when set must be unique within the model. Errors are returned in the
// following cases:
// - Machine is no longer alive or is missing;
// - Subnet inferred from any CIDRAddress field in args is known but no longer
//   alive (no error reported if the CIDRAddress does not match a known subnet);
// - Model no longer alive;
// - errors.NotValidError, when any of the fields in args contain invalid values;
// - errors.NotFoundError, when any DeviceName in args refers to unknown device;
// - ErrProviderIDNotUnique, when one or more specified ProviderIDs are not unique.
func (m *Machine) SetDevicesAddresses(devicesAddresses ...LinkLayerDeviceAddress) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set link-layer device addresses of machine %q", m.doc.Id)
	if len(devicesAddresses) == 0 {
		logger.Debugf("no device addresses to set")
		return nil
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		newDocs, err := m.prepareToSetDevicesAddresses(devicesAddresses)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if err := m.isStillAlive(); err != nil {
			return nil, errors.Trace(err)
		}
		if attempt > 0 {
			allIds, err := m.st.allProviderIDsForAddresses()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, args := range devicesAddresses {
				if allIds.Contains(string(args.ProviderID)) {
					err := NewProviderIDNotUniqueError(args.ProviderID)
					return nil, errors.Annotatef(err, "invalid address %q", args.CIDRAddress)
				}
			}
		}

		// we checked the model is active, but we only assert the machine is alive, because it will be dying if
		// the model is dying.
		ops := []txn.Op{
			m.assertAliveOp(),
		}

		setAddressesOps, err := m.setDevicesAddressesFromDocsOps(newDocs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(setAddressesOps) == 0 {
			// no actual address changes to be queued, so no need to create an op that just asserts
			// the machine is alive
			logger.Debugf("no changes to DevicesAddresses for machine %q", m.Id())
			return nil, jujutxn.ErrNoOperations
		}
		return append(ops, setAddressesOps...), nil
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
	if !IsValidLinkLayerDeviceName(args.DeviceName) {
		logger.Warningf(
			"address %q on machine %q has invalid device name %q (using anyway)",
			args.CIDRAddress, m.Id(), args.DeviceName,
		)
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
	subnetCIDR := ipNet.String()
	subnet, err := m.st.Subnet(subnetCIDR)
	if errors.IsNotFound(err) {
		logger.Debugf(
			"address %q on machine %q uses unknown or machine-local subnet %q",
			addressValue, m.Id(), subnetCIDR,
		)
	} else if err != nil {
		return nil, errors.Trace(err)
	} else if err := m.verifySubnetAlive(subnet); err != nil {
		return nil, errors.Trace(err)
	}

	globalKey := ipAddressGlobalKey(m.doc.Id, args.DeviceName, addressValue)
	ipAddressDocID := m.st.docID(globalKey)
	providerID := string(args.ProviderID)

	modelUUID := m.st.ModelUUID()

	newDoc := &ipAddressDoc{
		DocID:            ipAddressDocID,
		ModelUUID:        modelUUID,
		ProviderID:       providerID,
		DeviceName:       args.DeviceName,
		MachineID:        m.doc.Id,
		SubnetCIDR:       subnetCIDR,
		ConfigMethod:     args.ConfigMethod,
		Value:            addressValue,
		DNSServers:       args.DNSServers,
		DNSSearchDomains: args.DNSSearchDomains,
		GatewayAddress:   args.GatewayAddress,
		IsDefaultGateway: args.IsDefaultGateway,
	}
	return newDoc, nil
}

func (m *Machine) verifySubnetAlive(subnet *Subnet) error {
	if subnet.Life() != Alive {
		return errors.Errorf("subnet %q is not alive", subnet.CIDR())
	}
	return nil
}

func (m *Machine) setDevicesAddressesFromDocsOps(newDocs []ipAddressDoc) ([]txn.Op, error) {
	addresses, closer := m.st.db().GetCollection(ipAddressesC)
	defer closer()

	var ops []txn.Op

	for _, newDoc := range newDocs {
		var thisDeviceOps []txn.Op
		hasChanges := false
		deviceDocID := m.linkLayerDeviceDocIDFromName(newDoc.DeviceName)
		thisDeviceOps = append(thisDeviceOps, assertLinkLayerDeviceExistsOp(deviceDocID))

		var existingDoc ipAddressDoc
		err := addresses.FindId(newDoc.DocID).One(&existingDoc)
		if err == mgo.ErrNotFound {
			// Address does not exist yet - insert it.
			hasChanges = true
			thisDeviceOps = append(thisDeviceOps, insertIPAddressDocOp(&newDoc))
			if newDoc.ProviderID != "" {
				id := corenetwork.Id(newDoc.ProviderID)
				thisDeviceOps = append(thisDeviceOps, m.st.networkEntityGlobalKeyOp("address", id))
			}
		} else if err == nil {
			// Address already exists - update what's possible.
			var ipOp txn.Op
			ipOp, hasChanges = updateIPAddressDocOp(&existingDoc, &newDoc)
			thisDeviceOps = append(thisDeviceOps, ipOp)
			if newDoc.ProviderID != "" {
				if existingDoc.ProviderID != "" && existingDoc.ProviderID != newDoc.ProviderID {
					return nil, errors.Errorf("cannot change ProviderID of link address %q", existingDoc.Value)
				}
				if existingDoc.ProviderID != newDoc.ProviderID {
					// Need to insert the new provider id in providerIDsC
					id := corenetwork.Id(newDoc.ProviderID)
					thisDeviceOps = append(thisDeviceOps, m.st.networkEntityGlobalKeyOp("address", id))
					hasChanges = true
				}
			}
		} else {
			return nil, errors.Trace(err)
		}

		thisDeviceOps, err = m.maybeAssertSubnetAliveOps(&newDoc, thisDeviceOps)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if hasChanges {
			ops = append(ops, thisDeviceOps...)
		}
	}
	return ops, nil
}

func (m *Machine) maybeAssertSubnetAliveOps(newDoc *ipAddressDoc, opsSoFar []txn.Op) ([]txn.Op, error) {
	subnet, err := m.st.Subnet(newDoc.SubnetCIDR)
	if errors.IsNotFound(err) {
		// Subnet is machine-local, no need to assert whether it's alive.
		return opsSoFar, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	if err := m.verifySubnetAlive(subnet); err != nil {
		return nil, errors.Trace(err)
	}

	// Subnet exists and is still alive, assert that is stays that way.
	return append(opsSoFar, txn.Op{
		C:      subnetsC,
		Id:     m.st.docID(newDoc.SubnetCIDR),
		Assert: isAliveDoc,
	}), nil
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

// AllAddresses returns the all addresses assigned to all devices of the
// machine.
func (m *Machine) AllAddresses() ([]*Address, error) {
	var allAddresses []*Address
	callbackFunc := func(resultDoc *ipAddressDoc) {
		allAddresses = append(allAddresses, newIPAddress(m.st, *resultDoc))
	}

	findQuery := findAddressesQuery(m.doc.Id, "")
	if err := m.st.forEachIPAddressDoc(findQuery, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allAddresses, nil
}

// AllSpaces returns the set of spaces that this machine is actively
// connected to.
func (m *Machine) AllSpaces() (set.Strings, error) {
	// TODO(jam): 2016-12-18 This should evolve to look at the
	// LinkLayerDevices directly, instead of using the Addresses the devices
	// are in to link back to spaces.
	spaces := set.NewStrings()
	addresses, err := m.AllAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, address := range addresses {
		subnet, err := address.Subnet()
		if err != nil {
			if errors.IsNotFound(err) {
				// We don't know what this subnet is, so it can't be a space. It
				// might just be the loopback device.
				continue
			}
			return nil, errors.Trace(err)
		}
		spaceName := subnet.SpaceName()
		if spaceName != "" {
			spaces.Add(spaceName)
		}
	}
	logger.Tracef("machine %q found AllSpaces() = %s",
		m.Id(), network.QuoteSpaceSet(spaces))
	return spaces, nil
}

// AllNetworkAddresses returns the result of AllAddresses(), but transformed to
// []network.Address.
func (m *Machine) AllNetworkAddresses() ([]network.Address, error) {
	stateAddresses, err := m.AllAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	networkAddresses := make([]network.Address, len(stateAddresses))
	for i := range stateAddresses {
		networkAddresses[i] = stateAddresses[i].NetworkAddress()
	}
	// TODO(jam): 20161130 NetworkAddress object has a SpaceName attribute.
	// However, we are not filling in that information here.
	return networkAddresses, nil
}

// deviceMapToSortedList takes a map from device name to LinkLayerDevice
// object, and returns the list of LinkLayerDevice object using
// NaturallySortDeviceNames
func deviceMapToSortedList(deviceMap map[string]*LinkLayerDevice) []*LinkLayerDevice {
	names := make([]string, 0, len(deviceMap))
	for name := range deviceMap {
		// name must == device.Name()
		names = append(names, name)
	}
	sortedNames := network.NaturallySortDeviceNames(names...)
	result := make([]*LinkLayerDevice, len(sortedNames))
	for i, name := range sortedNames {
		result[i] = deviceMap[name]
	}
	return result
}

// LinkLayerDevicesForSpaces takes a list of spaces, and returns the devices on
// this machine that are in that space that we feel would be useful for
// containers to know about.  (eg, if there is a host device that has been
// bridged, we return the bridge, rather than the underlying device, but if we
// have only the host device, we return that.)
// Note that devices like 'lxdbr0' that are bridges that might might not be
// externally accessible may be returned if "" is listed as one of the desired
// spaces.
func (m *Machine) LinkLayerDevicesForSpaces(spaces []string) (map[string][]*LinkLayerDevice, error) {
	addresses, err := m.AllAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	devices, err := m.AllLinkLayerDevices()
	if err != nil {
		return nil, errors.Trace(err)
	}
	deviceByName := make(map[string]*LinkLayerDevice, len(devices))
	for _, dev := range devices {
		deviceByName[dev.Name()] = dev
	}
	requestedSpaces := set.NewStrings(spaces...)
	spaceToDevices := make(map[string]map[string]*LinkLayerDevice, 0)
	processedDeviceNames := set.NewStrings()
	includeDevice := func(spaceName string, device *LinkLayerDevice) {
		spaceInfo, ok := spaceToDevices[spaceName]
		if !ok {
			spaceInfo = make(map[string]*LinkLayerDevice)
			spaceToDevices[spaceName] = spaceInfo
		}
		spaceInfo[device.Name()] = device
	}
	// First pass, iterate the addresses, lookup the associated spaces, and
	// gather the devices.
	for _, addr := range addresses {
		subnet, err := addr.Subnet()
		spaceName := ""
		if err != nil {
			if errors.IsNotFound(err) {
				// unknown subnets are considered part of the "unknown" space
				spaceName = ""
			} else {
				// We don't understand the error, so error out for now
				return nil, errors.Trace(err)
			}
		} else {
			spaceName = subnet.SpaceName()
		}
		device, ok := deviceByName[addr.DeviceName()]
		if !ok {
			return nil, errors.Errorf("address %v for machine %q refers to a missing device %q",
				addr, m.Id(), addr.DeviceName())
		}
		processedDeviceNames.Add(device.Name())
		if device.Type() == LoopbackDevice {
			// We skip loopback devices here
			continue
		}
		includeDevice(spaceName, device)
	}
	// Now grab any devices we may have missed. For now, any device without an
	// address must be in the "unknown" space.
	for devName, device := range deviceByName {
		if processedDeviceNames.Contains(devName) {
			continue
		}
		// Loopback devices aren't considered part of the empty space
		// Also, devices that are attached to another device also aren't
		// considered to be in the unknown space.
		if device.Type() == LoopbackDevice || device.ParentName() != "" {
			continue
		}
		includeDevice("", device)
	}
	result := make(map[string][]*LinkLayerDevice, len(spaceToDevices))
	for spaceName, deviceMap := range spaceToDevices {
		if !requestedSpaces.Contains(spaceName) {
			continue
		}
		result[spaceName] = deviceMapToSortedList(deviceMap)
	}
	return result, nil
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
			// FIXME: this obliterates the ProviderID of *all*
			// devices if any *one* of them is not unique.
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
		// FIXME: this obliterates the ProviderID of *all*
		// addresses if any *one* of them is not unique.
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

// MachineNetworkInfoResult contains an error or a list of NetworkInfo structures for a specific space.
type MachineNetworkInfoResult struct {
	NetworkInfos []network.NetworkInfo
	Error        error
}

// Add address to a device in list or create a new device with this address.
func addAddressToResult(networkInfos []network.NetworkInfo, address *Address) ([]network.NetworkInfo, error) {
	ifaceAddress := network.InterfaceAddress{
		Address: address.Value(),
		CIDR:    address.SubnetCIDR(),
	}
	for i := range networkInfos {
		networkInfo := &networkInfos[i]
		if networkInfo.InterfaceName == address.DeviceName() {
			networkInfo.Addresses = append(networkInfo.Addresses, ifaceAddress)
			return networkInfos, nil
		}
	}

	MAC := ""
	device, err := address.Device()
	if err == nil {
		MAC = device.MACAddress()
	} else if !errors.IsNotFound(err) {
		return nil, err
	}
	networkInfo := network.NetworkInfo{
		InterfaceName: address.DeviceName(),
		MACAddress:    MAC,
		Addresses:     []network.InterfaceAddress{ifaceAddress},
	}
	return append(networkInfos, networkInfo), nil
}

// GetNetworkInfoForSpaces returns MachineNetworkInfoResult with a list of devices for each space in spaces
// TODO(wpk): 2017-05-04 This does not work for L2-only devices as it iterates over addresses, needs to be fixed.
// When changing the method we have to keep the ordering.
func (m *Machine) GetNetworkInfoForSpaces(spaces set.Strings) map[string]MachineNetworkInfoResult {
	results := make(map[string]MachineNetworkInfoResult)

	var privateAddress network.Address

	if spaces.Contains(corenetwork.DefaultSpaceName) {
		var err error
		privateAddress, err = m.PrivateAddress()
		if err != nil {
			results[corenetwork.DefaultSpaceName] = MachineNetworkInfoResult{Error: errors.Annotatef(
				err, "getting machine %q preferred private address", m.MachineTag())}
			spaces.Remove(corenetwork.DefaultSpaceName)
		}
	}

	addresses, err := m.AllAddresses()
	logger.Debugf("Looking for something from spaces %v in %v", spaces, addresses)
	if err != nil {
		result := MachineNetworkInfoResult{Error: errors.Annotate(err, "cannot get devices addresses")}
		for space := range spaces {
			if _, ok := results[space]; !ok {
				results[space] = result
			}
		}
		return results
	}
	actualSpaces := set.NewStrings()
	for _, addr := range addresses {
		subnet, err := addr.Subnet()
		switch {
		case errors.IsNotFound(err):
			logger.Debugf("skipping %s: not linked to a known subnet (%v)", addr, err)
		case err != nil:
			logger.Errorf("cannot get subnet for address %q - %q", addr, err)
		default:
			space := subnet.SpaceName()
			actualSpaces.Add(space)
			if spaces.Contains(space) {
				r := results[space]
				r.NetworkInfos, err = addAddressToResult(r.NetworkInfos, addr)
				if err != nil {
					r.Error = err
				} else {
					results[space] = r
				}
			}
			if spaces.Contains(corenetwork.DefaultSpaceName) && privateAddress.Value == addr.Value() {
				r := results[corenetwork.DefaultSpaceName]
				r.NetworkInfos, err = addAddressToResult(r.NetworkInfos, addr)
				if err != nil {
					r.Error = err
				} else {
					results[corenetwork.DefaultSpaceName] = r
				}
			}
		}
	}

	// For a spaceless model we won't find a subnet that's linked to privateAddress,
	// we have to work around that and at least return minimal information.
	if r, ok := results[corenetwork.DefaultSpaceName]; !ok && spaces.Contains(corenetwork.DefaultSpaceName) {
		r.NetworkInfos = []network.NetworkInfo{{
			Addresses: []network.InterfaceAddress{{
				Address: privateAddress.Value,
			}},
		}}
		results[corenetwork.DefaultSpaceName] = r
	}
	actualSpacesStr := network.QuoteSpaceSet(actualSpaces)

	for space := range spaces {
		if _, ok := results[space]; !ok {
			results[space] = MachineNetworkInfoResult{
				Error: errors.Errorf("machine %q has no devices in space %q, only spaces %s",
					m.doc.Id, space, actualSpacesStr),
			}
		}
	}
	return results
}
