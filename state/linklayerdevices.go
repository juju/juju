// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
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
	return network.Id(dev.localProviderID())
}

func (dev *LinkLayerDevice) localProviderID() string {
	return dev.st.localID(dev.doc.ProviderID)
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

// ParentDevice returns the LinkLayerDevice corresponding to the parent device
// of this device, if set. When no parent device name is set, it returns nil and
// no error.
func (dev *LinkLayerDevice) ParentDevice() (*LinkLayerDevice, error) {
	if dev.doc.ParentName == "" {
		return nil, nil
	}

	hostMachineID, parentDeviceName, err := parseParentNameAsGlobalKey(dev.doc.ParentName)
	if err != nil {
		return nil, errors.Trace(err)
	} else if hostMachineID != "" {
		// parent device is on the host machine.
		return dev.machineProxy(hostMachineID).LinkLayerDevice(parentDeviceName)
	}

	// parent device is on the same machine.
	return dev.machineProxy(dev.doc.MachineID).LinkLayerDevice(dev.doc.ParentName)
}

func (dev *LinkLayerDevice) parentDocID() string {
	parentGlobalKey := dev.parentGlobalKey()
	if parentGlobalKey == "" {
		return ""
	}
	return dev.st.docID(parentGlobalKey)
}

func (dev *LinkLayerDevice) parentGlobalKey() string {
	return linkLayerDeviceGlobalKey(dev.doc.MachineID, dev.doc.ParentName)
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
		return removeLinkLayerDeviceOps(dev.st, dev.DocID(), dev.parentDocID())
	}
	return dev.st.run(buildTxn)
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

	if numChildren > 0 {
		deviceName := linkLayerDeviceNameFromDocID(linkLayerDeviceDocID)
		return nil, newParentDeviceHasChildrenError(deviceName, numChildren)
	}

	var ops []txn.Op
	if parentDeviceDocID != "" {
		ops = append(ops, decrementDeviceNumChildrenOp(parentDeviceDocID))
	}
	return append(ops,
		removeLinkLayerDeviceDocOp(linkLayerDeviceDocID),
		removeLinkLayerDevicesRefsOp(linkLayerDeviceDocID),
	), nil
}

// linkLayerDeviceNameFromDocID extracts the last part of linkLayerDeviceDocID - the name.
func linkLayerDeviceNameFromDocID(linkLayerDeviceDocID string) string {
	lastHash := strings.LastIndex(linkLayerDeviceDocID, "#")
	deviceName := linkLayerDeviceDocID[lastHash+1:]
	return deviceName
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
