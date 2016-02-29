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
	var removeAllDevicesOps []txn.Op
	callbackFunc := func(resultDoc *linkLayerDeviceDoc) {
		removeOps := removeLinkLayerDeviceUnconditionallyOps(resultDoc.DocID)
		removeAllDevicesOps = append(removeAllDevicesOps, removeOps...)
	}

	selectDocIDOnly := bson.D{{"_id", 1}}
	if err := m.forEachLinkLayerDeviceDoc(selectDocIDOnly, callbackFunc); err != nil {
		return errors.Trace(err)
	}

	return m.st.runTransaction(removeAllDevicesOps)
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
// Adding parent devices must be done in a separate call than adding their children.
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
			ops = append(ops, m.insertLinkLayerDeviceOps(&newDoc)...)
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

func (st *State) allProviderIDsForModelLinkLayerDevices() (_ set.Strings, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot get ProviderIDs for all link-layer devices")

	linkLayerDevices, closer := st.getCollection(linkLayerDevicesC)
	defer closer()

	allProviderIDs := set.NewStrings()
	var doc struct {
		ProviderID string `bson:"providerid"`
	}

	pattern := fmt.Sprintf("^%s:.+$", st.ModelUUID())
	modelProviderIDs := bson.D{{"providerid", bson.D{{"$regex", pattern}}}}
	onlyProviderIDField := bson.D{{"providerid", 1}}

	iter := linkLayerDevices.Find(modelProviderIDs).Select(onlyProviderIDField).Iter()
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

	if err := validateAddLinkLayerDeviceArgs(args); err != nil {
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

func validateAddLinkLayerDeviceArgs(args *LinkLayerDeviceArgs) error {
	if args.Name == "" {
		return errors.NotValidf("empty Name")
	}
	if !IsValidLinkLayerDeviceName(args.Name) {
		return errors.NotValidf("Name %q", args.Name)
	}

	if args.ParentName != "" {
		if !IsValidLinkLayerDeviceName(args.ParentName) {
			return errors.NotValidf("ParentName %q", args.ParentName)
		}
		if args.Name == args.ParentName {
			return errors.NewNotValid(nil, "Name and ParentName must be different")
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

func (m *Machine) insertLinkLayerDeviceOps(newDoc *linkLayerDeviceDoc) []txn.Op {
	modelUUID, linkLayerDeviceDocID := newDoc.ModelUUID, newDoc.DocID

	var ops []txn.Op
	if newDoc.ParentName != "" {
		parentDocID := m.linkLayerDeviceDocIDFromName(newDoc.ParentName)
		ops = append(ops, incrementDeviceNumChildrenOp(parentDocID))
	}
	return append(ops,
		insertLinkLayerDeviceDocOp(newDoc),
		insertLinkLayerDevicesRefsOp(modelUUID, linkLayerDeviceDocID),
	)
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
