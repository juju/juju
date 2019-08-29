// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
)

// linkLayerDeviceDoc describes the persistent state of a link-layer network
// device for a machine.
type linkLayerDeviceDoc struct {
	// DocID is the link-layer device global key, prefixed by ModelUUID.
	DocID string `bson:"_id"`

	// Name is the name of the network device as it appears on the machine.
	Name string `bson:"name"`

	// ModelUUID is the UUID of the model this device belongs to.
	ModelUUID string `bson:"model-uuid"`

	// MTU is the maximum transmission unit the device can handle.
	MTU uint `bson:"mtu"`

	// ProviderID is a provider-specific ID of the device, prefixed by
	// ModelUUID. Empty when not supported by the provider.
	ProviderID string `bson:"providerid,omitempty"`

	// MachineID is the ID of the machine this device belongs to.
	MachineID string `bson:"machine-id"`

	// Type is the undelying type of the device.
	Type LinkLayerDeviceType `bson:"type"`

	// MACAddress is the media access control (MAC) address of the device.
	MACAddress string `bson:"mac-address"`

	// IsAutoStart is true if the device should be activated on boot.
	IsAutoStart bool `bson:"is-auto-start"`

	// IsUp is true when the device is up (enabled).
	IsUp bool `bson:"is-up"`

	// ParentName is the name of the parent device, which may be empty. When set
	// the parent device must be on the same machine, unless the current device
	// is inside a container, in which case ParentName can be a global key of a
	// BridgeDevice on the host machine of the container.
	ParentName string `bson:"parent-name"`
}

// LinkLayerDeviceType defines the type of a link-layer network device.
type LinkLayerDeviceType string

const (
	// LoopbackDevice is used for loopback devices.
	LoopbackDevice LinkLayerDeviceType = "loopback"

	// EthernetDevice is used for Ethernet (IEEE 802.3) devices.
	EthernetDevice LinkLayerDeviceType = "ethernet"

	// VLAN_8021QDevice is used for IEEE 802.1Q VLAN devices.
	VLAN_8021QDevice LinkLayerDeviceType = "802.1q"

	// BondDevice is used for bonding devices.
	BondDevice LinkLayerDeviceType = "bond"

	// BridgeDevice is used for OSI layer-2 bridge devices.
	BridgeDevice LinkLayerDeviceType = "bridge"
)

// IsValidLinkLayerDeviceType returns whether the given value is a valid
// link-layer network device type.
func IsValidLinkLayerDeviceType(value string) bool {
	switch LinkLayerDeviceType(value) {
	case LoopbackDevice, EthernetDevice,
		VLAN_8021QDevice,
		BondDevice, BridgeDevice:
		return true
	}
	return false
}

// LinkLayerDevice represents the state of a link-layer network device for a
// machine.
type LinkLayerDevice struct {
	st  *State
	doc linkLayerDeviceDoc
}

func newLinkLayerDevice(st *State, doc linkLayerDeviceDoc) *LinkLayerDevice {
	return &LinkLayerDevice{st: st, doc: doc}
}

// AllLinkLayerDevices returns all link layer devices in the model.
func (st *State) AllLinkLayerDevices() (devices []*LinkLayerDevice, err error) {
	devicesCollection, closer := st.db().GetCollection(linkLayerDevicesC)
	defer closer()

	sdocs := []linkLayerDeviceDoc{}
	err = devicesCollection.Find(nil).All(&sdocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all link layer devices")
	}
	for _, d := range sdocs {
		devices = append(devices, newLinkLayerDevice(st, d))
	}
	return devices, nil
}

// DocID returns the globally unique ID of the link-layer device, including the
// model UUID as prefix.
func (dev *LinkLayerDevice) DocID() string {
	return dev.st.docID(dev.doc.DocID)
}

// Name returns the name of the device, as it appears on the machine.
func (dev *LinkLayerDevice) Name() string {
	return dev.doc.Name
}

// MTU returns the maximum transmission unit the device can handle.
func (dev *LinkLayerDevice) MTU() uint {
	return dev.doc.MTU
}

// ProviderID returns the provider-specific device ID, if set.
func (dev *LinkLayerDevice) ProviderID() network.Id {
	return network.Id(dev.doc.ProviderID)
}

// MachineID returns the ID of the machine this device belongs to.
func (dev *LinkLayerDevice) MachineID() string {
	return dev.doc.MachineID
}

// Machine returns the Machine this device belongs to.
func (dev *LinkLayerDevice) Machine() (*Machine, error) {
	return dev.st.Machine(dev.doc.MachineID)
}

// Type returns this device's underlying type.
func (dev *LinkLayerDevice) Type() LinkLayerDeviceType {
	return dev.doc.Type
}

// IsLoopbackDevice returns whether this is a loopback device.
func (dev *LinkLayerDevice) IsLoopbackDevice() bool {
	return dev.doc.Type == LoopbackDevice
}

// MACAddress returns the media access control (MAC) address of the device.
func (dev *LinkLayerDevice) MACAddress() string {
	return dev.doc.MACAddress
}

// IsAutoStart returns whether the device is set to automatically start on boot.
func (dev *LinkLayerDevice) IsAutoStart() bool {
	return dev.doc.IsAutoStart
}

// IsUp returns whether the device is currently up.
func (dev *LinkLayerDevice) IsUp() bool {
	return dev.doc.IsUp
}

// ParentName returns the name of this device's parent device, if set. The
// parent device is almost always on the same machine as the child device, but
// as a special case a child device on a container machine can have a parent
// BridgeDevice on the container's host machine. In the last case ParentName()
// returns the global key of the parent device, not just its name.
func (dev *LinkLayerDevice) ParentName() string {
	return dev.doc.ParentName
}

func (dev *LinkLayerDevice) parentDeviceNameAndMachineID() (string, string) {
	if dev.doc.ParentName == "" {
		// No parent set, so no ID and name to return.
		return "", ""
	}
	// In case ParentName is a global key, try getting the host machine ID from
	// there first.
	hostMachineID, parentDeviceName, err := parseLinkLayerDeviceParentNameAsGlobalKey(dev.doc.ParentName)
	if err != nil {
		// We validate the ParentName before setting it, so this case cannot
		// happen and we're only logging the error.
		logger.Errorf("%s has invalid parent: %v", dev, err)
		return "", ""
	}
	if hostMachineID == "" {
		// Parent device is on the same machine and ParentName is not a global
		// key.
		return dev.doc.ParentName, dev.doc.MachineID
	}
	return parentDeviceName, hostMachineID
}

// ParentDevice returns the LinkLayerDevice corresponding to the parent device
// of this device, if set. When no parent device name is set, it returns nil and
// no error.
func (dev *LinkLayerDevice) ParentDevice() (*LinkLayerDevice, error) {
	if dev.doc.ParentName == "" {
		return nil, nil
	}

	parentDeviceName, parentMachineID := dev.parentDeviceNameAndMachineID()
	return dev.machineProxy(parentMachineID).LinkLayerDevice(parentDeviceName)
}

func (dev *LinkLayerDevice) parentDocID() string {
	parentDeviceName, parentMachineID := dev.parentDeviceNameAndMachineID()
	parentGlobalKey := linkLayerDeviceGlobalKey(parentMachineID, parentDeviceName)
	if parentGlobalKey == "" {
		return ""
	}
	return dev.st.docID(parentGlobalKey)
}

// machineProxy is a convenience wrapper for calling Machine.LinkLayerDevice()
// or Machine.forEachLinkLayerDeviceDoc() from a *LinkLayerDevice and machineID.
func (dev *LinkLayerDevice) machineProxy(machineID string) *Machine {
	return &Machine{st: dev.st, doc: machineDoc{Id: machineID}}
}

// Remove removes the device, if it exists. No error is returned when the device
// was already removed. ErrParentDeviceHasChildren is returned if this device is
// a parent to one or more existing devices and therefore cannot be removed.
func (dev *LinkLayerDevice) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove %s", dev)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err = dev.errNoOperationsIfMissing(); err != nil {
				return nil, err
			}
		}
		ops, err := removeLinkLayerDeviceOps(dev.st, dev.DocID(), dev.parentDocID())
		if err != nil {
			return nil, err
		}
		if dev.ProviderID() != "" {
			op := dev.st.networkEntityGlobalKeyRemoveOp("linklayerdevice", dev.ProviderID())
			ops = append(ops, op)
		}
		return ops, nil
	}
	return dev.st.db().Run(buildTxn)
}

func (dev *LinkLayerDevice) errNoOperationsIfMissing() error {
	_, err := dev.machineProxy(dev.doc.MachineID).LinkLayerDevice(dev.doc.Name)
	if errors.IsNotFound(err) {
		return jujutxn.ErrNoOperations
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// removeLinkLayerDeviceOps returns the list of operations needed to remove the
// device with the given linkLayerDeviceDocID, asserting it still exists and has
// no children referring to it. If the device is a child, parentDeviceDocID will
// be non-empty and the operations includes decrementing the parent's
// NumChildren.
func removeLinkLayerDeviceOps(st *State, linkLayerDeviceDocID, parentDeviceDocID string) ([]txn.Op, error) {
	var numChildren int
	if parentDeviceDocID == "" {
		// If not a child, verify it has no children.
		var err error
		numChildren, err = getParentDeviceNumChildrenRefs(st, linkLayerDeviceDocID)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// We know the DocID has a valid format for a global key, hence the last
	// return below is ignored.
	machineID, deviceName, canBeGlobalKey := parseLinkLayerDeviceGlobalKey(linkLayerDeviceDocID)
	if !canBeGlobalKey {
		return nil, errors.Errorf(
			"link-layer device %q on machine %q has unexpected key format",
			machineID, deviceName,
		)
	}
	if numChildren > 0 {
		return nil, newParentDeviceHasChildrenError(deviceName, numChildren)
	}

	var ops []txn.Op
	if parentDeviceDocID != "" {
		ops = append(ops, decrementDeviceNumChildrenOp(parentDeviceDocID))
	}

	addressesQuery := findAddressesQuery(machineID, deviceName)
	if addressesOps, err := st.removeMatchingIPAddressesDocOps(addressesQuery); err == nil {
		ops = append(ops, addressesOps...)
	} else {
		return nil, errors.Trace(err)
	}

	return append(ops,
		removeLinkLayerDeviceDocOp(linkLayerDeviceDocID),
		removeLinkLayerDevicesRefsOp(linkLayerDeviceDocID),
	), nil
}

// removeLinkLayerDeviceDocOp returns an operation to remove the
// linkLayerDeviceDoc matching the given linkLayerDeviceDocID, asserting it
// still exists.
func removeLinkLayerDeviceDocOp(linkLayerDeviceDocID string) txn.Op {
	return txn.Op{
		C:      linkLayerDevicesC,
		Id:     linkLayerDeviceDocID,
		Assert: txn.DocExists,
		Remove: true,
	}
}

// removeLinkLayerDeviceUnconditionallyOps returns the list of operations to
// unconditionally remove the device matching the given linkLayerDeviceDocID,
// along with its linkLayerDevicesRefsDoc. No asserts are included for the
// existence of both documents.
func removeLinkLayerDeviceUnconditionallyOps(linkLayerDeviceDocID string) []txn.Op {
	// Reuse the regular remove ops, but drop their asserts.
	removeDeviceDocOp := removeLinkLayerDeviceDocOp(linkLayerDeviceDocID)
	removeDeviceDocOp.Assert = nil
	removeRefsOp := removeLinkLayerDevicesRefsOp(linkLayerDeviceDocID)
	removeRefsOp.Assert = nil

	return []txn.Op{
		removeDeviceDocOp,
		removeRefsOp,
	}
}

// insertLinkLayerDeviceDocOp returns an operation inserting the given newDoc,
// asserting it does not exist yet.
func insertLinkLayerDeviceDocOp(newDoc *linkLayerDeviceDoc) txn.Op {
	return txn.Op{
		C:      linkLayerDevicesC,
		Id:     newDoc.DocID,
		Assert: txn.DocMissing,
		Insert: *newDoc,
	}
}

// updateLinkLayerDeviceDocOp returns an operation updating the fields of
// existingDoc with the respective values of those fields in newDoc. DocID,
// ModelUUID, MachineID, and Name cannot be changed. ProviderID cannot be
// changed once set. In all other cases newDoc values overwrites existingDoc
// values.
func updateLinkLayerDeviceDocOp(existingDoc, newDoc *linkLayerDeviceDoc) (txn.Op, bool) {
	changes := make(bson.M)
	if existingDoc.ProviderID == "" && newDoc.ProviderID != "" {
		// Only allow changing the ProviderID if it was empty.
		changes["providerid"] = newDoc.ProviderID
	}
	if existingDoc.Type != newDoc.Type {
		changes["type"] = newDoc.Type
	}
	if existingDoc.MTU != newDoc.MTU {
		changes["mtu"] = newDoc.MTU
	}
	if existingDoc.MACAddress != newDoc.MACAddress {
		changes["mac-address"] = newDoc.MACAddress
	}
	if existingDoc.IsAutoStart != newDoc.IsAutoStart {
		changes["is-auto-start"] = newDoc.IsAutoStart
	}
	if existingDoc.IsUp != newDoc.IsUp {
		changes["is-up"] = newDoc.IsUp
	}
	if existingDoc.ParentName != newDoc.ParentName {
		changes["parent-name"] = newDoc.ParentName
	}

	var updates bson.D
	if len(changes) > 0 {
		updates = append(updates, bson.DocElem{Name: "$set", Value: changes})
	}

	return txn.Op{
		C:      linkLayerDevicesC,
		Id:     existingDoc.DocID,
		Assert: txn.DocExists,
		Update: updates,
	}, len(updates) > 0
}

// assertLinkLayerDeviceExistsOp returns an operation asserting the document
// matching linkLayerDeviceDocID exists.
func assertLinkLayerDeviceExistsOp(linkLayerDeviceDocID string) txn.Op {
	return txn.Op{
		C:      linkLayerDevicesC,
		Id:     linkLayerDeviceDocID,
		Assert: txn.DocExists,
	}
}

// String returns a human-readable representation of the device.
func (dev *LinkLayerDevice) String() string {
	return fmt.Sprintf("%s device %q on machine %q", dev.doc.Type, dev.doc.Name, dev.doc.MachineID)
}

func (dev *LinkLayerDevice) globalKey() string {
	return linkLayerDeviceGlobalKey(dev.doc.MachineID, dev.doc.Name)
}

func linkLayerDeviceGlobalKey(machineID, deviceName string) string {
	if machineID == "" || deviceName == "" {
		return ""
	}
	return "m#" + machineID + "#d#" + deviceName
}

func parseLinkLayerDeviceGlobalKey(globalKey string) (machineID, deviceName string, canBeGlobalKey bool) {
	if !strings.Contains(globalKey, "#") {
		// Can't be a global key.
		return "", "", false
	}
	keyParts := strings.Split(globalKey, "#")
	if len(keyParts) != 4 || (keyParts[0] != "m" && keyParts[2] != "d") {
		// Invalid global key format.
		return "", "", true
	}
	machineID, deviceName = keyParts[1], keyParts[3]
	return machineID, deviceName, true
}

// IsValidLinkLayerDeviceName returns whether the given name is a valid network
// link-layer device name, depending on the runtime.GOOS value.
func IsValidLinkLayerDeviceName(name string) bool {
	if runtimeGOOS == "linux" {
		return isValidLinuxDeviceName(name)
	}
	hasHash := strings.Contains(name, "#")
	return !hasHash && stringLengthBetween(name, 1, 255)
}

// runtimeGOOS is defined to allow patching in tests.
var runtimeGOOS = runtime.GOOS

// isValidLinuxDeviceName returns whether the given deviceName is valid,
// using the same criteria as dev_valid_name(9) in the Linux kernel:
// - no whitespace allowed
// - length from 1 to 15 ASCII characters
// - literal "." and ".." as names are not allowed.
// Additionally, we don't allow "#" in the name.
func isValidLinuxDeviceName(name string) bool {
	hasWhitespace := whitespaceReplacer.Replace(name) != name
	isDot, isDoubleDot := name == ".", name == ".."
	hasValidLength := stringLengthBetween(name, 1, 15)
	hasHash := strings.Contains(name, "#")

	return hasValidLength && !(hasHash || hasWhitespace || isDot || isDoubleDot)
}

// whitespaceReplacer strips whitespace characters from the input string.
var whitespaceReplacer = strings.NewReplacer(
	" ", "",
	"\t", "",
	"\v", "",
	"\n", "",
	"\r", "",
)

func stringLengthBetween(value string, minLength, maxLength uint) bool {
	length := uint(len(value))
	return length >= minLength && length <= maxLength
}

// Addresses returns all IP addresses assigned to the device.
func (dev *LinkLayerDevice) Addresses() ([]*Address, error) {
	var allAddresses []*Address
	callbackFunc := func(resultDoc *ipAddressDoc) {
		allAddresses = append(allAddresses, newIPAddress(dev.st, *resultDoc))
	}

	findQuery := findAddressesQuery(dev.doc.MachineID, dev.doc.Name)
	if err := dev.st.forEachIPAddressDoc(findQuery, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allAddresses, nil
}

// RemoveAddresses removes all IP addresses assigned to the device.
func (dev *LinkLayerDevice) RemoveAddresses() error {
	findQuery := findAddressesQuery(dev.doc.MachineID, dev.doc.Name)
	ops, err := dev.st.removeMatchingIPAddressesDocOps(findQuery)
	if err != nil {
		return errors.Trace(err)
	}

	return dev.st.db().RunTransaction(ops)
}

// EthernetDeviceForBridge returns LinkLayerDeviceArgs representing an ethernet
// device with the input name and this device as its parent.
// If the device is not a bridge, an error is returned.
func (dev *LinkLayerDevice) EthernetDeviceForBridge(name string) (LinkLayerDeviceArgs, error) {
	if dev.Type() != BridgeDevice {
		return LinkLayerDeviceArgs{}, errors.Errorf("device must be a Bridge Device, receiver has type %q", dev.Type())
	}
	return LinkLayerDeviceArgs{
		Name:        name,
		Type:        EthernetDevice,
		MACAddress:  network.GenerateVirtualMACAddress(),
		MTU:         dev.MTU(),
		IsUp:        true,
		IsAutoStart: true,
		ParentName:  dev.globalKey(),
	}, nil
}
