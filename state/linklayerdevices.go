// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

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

	// Type is the underlying type of the device.
	Type network.LinkLayerDeviceType `bson:"type"`

	// MACAddress is the media access control (MAC) address of the device.
	MACAddress string `bson:"mac-address"`

	// IsAutoStart is true if the device should be activated on boot.
	IsAutoStart bool `bson:"is-auto-start"`

	// IsUp is true when the device is up (enabled).
	IsUp bool `bson:"is-up"`

	// ParentName is the name of the parent device, which may be empty.
	// When set, the parent device must be on the same machine unless the
	// current device is inside a container, in which case it can be a global
	// key of a bridge device on the container host.
	ParentName string `bson:"parent-name"`

	// If this is device is part of a virtual switch, this field indicates
	// the type of switch (e.g. an OVS bridge ) this port belongs to.
	VirtualPortType network.VirtualPortType `bson:"virtual-port-type"`
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

// DocID returns the globally unique ID of the link-layer device,
// including the model UUID as prefix.
func (dev *LinkLayerDevice) DocID() string {
	return dev.st.docID(dev.doc.DocID)
}

// ID returns the unique ID of this device within the model.
func (dev *LinkLayerDevice) ID() string {
	return dev.st.localID(dev.doc.DocID)
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

// Type returns this device's underlying type.
func (dev *LinkLayerDevice) Type() network.LinkLayerDeviceType {
	return dev.doc.Type
}

// IsLoopbackDevice returns whether this is a loopback device.
func (dev *LinkLayerDevice) IsLoopbackDevice() bool {
	return dev.doc.Type == network.LoopbackDevice
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

// ParentName returns the name of this device's parent device if set.
// The parent device is almost always on the same machine as the child device,
// but as a special case a child device on a container machine can have a
// parent bridge device on the container's host machine.
// In this case the global key of the parent device is returned.
func (dev *LinkLayerDevice) ParentName() string {
	return dev.doc.ParentName
}

// VirtualPortType returns the type of virtual port for the device if managed
// by a virtual switch.
func (dev *LinkLayerDevice) VirtualPortType() network.VirtualPortType {
	return dev.doc.VirtualPortType
}

// ParentID uses the rules for ParentName (above) to return
// the ID of this device's parent if it has one.
func (dev *LinkLayerDevice) ParentID() string {
	parent := dev.doc.ParentName
	if parent == "" {
		return ""
	}

	if strings.Contains(parent, "#") {
		return parent
	}

	return strings.Join([]string{"m", dev.doc.MachineID, "d", dev.doc.ParentName}, "#")
}

// ParentDevice returns the LinkLayerDevice corresponding to the parent device
// of this device, if set. When no parent device name is set, it returns nil and
// no error.
func (dev *LinkLayerDevice) ParentDevice() (*LinkLayerDevice, error) {
	if dev.ParentID() == "" {
		return nil, nil
	}

	dev, err := dev.st.LinkLayerDevice(dev.ParentID())
	return dev, errors.Trace(err)
}

// RemoveOps returns transaction operations that will ensure that the
// device is not present in the collection and that if set,
// its provider ID is removed from the global register.
// Note that this method eschews responsibility for removing device
// addresses and for ensuring that this device has no children.
// That responsibility lies with the caller.
func (dev *LinkLayerDevice) RemoveOps() []txn.Op {
	ops := []txn.Op{{
		C:      linkLayerDevicesC,
		Id:     dev.DocID(),
		Remove: true,
	}}

	if dev.ProviderID() != "" {
		ops = append(ops, dev.st.networkEntityGlobalKeyRemoveOp("linklayerdevice", dev.ProviderID()))
	}

	return ops
}

func (st *State) LinkLayerDevice(id string) (*LinkLayerDevice, error) {
	linkLayerDevices, closer := st.db().GetCollection(linkLayerDevicesC)
	defer closer()

	var doc linkLayerDeviceDoc
	err := linkLayerDevices.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("device with ID %q", id)
	} else if err != nil {
		return nil, errors.Annotatef(err, "retrieving %q", id)
	}

	return newLinkLayerDevice(st, doc), nil
}

// String returns a human-readable representation of the device.
func (dev *LinkLayerDevice) String() string {
	return fmt.Sprintf("%s device %q on machine %q", dev.doc.Type, dev.doc.Name, dev.doc.MachineID)
}

func linkLayerDeviceGlobalKey(machineID, deviceName string) string {
	if machineID == "" || deviceName == "" {
		return ""
	}
	return "m#" + machineID + "#d#" + deviceName
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

// EthernetDeviceForBridge returns an InterfaceInfo representing an ethernet
// device with the input name and this device as its parent.
// The detail supplied reflects whether the provider is expected to supply the
// interface's eventual address.
// If the device is not a bridge, an error is returned.
func (dev *LinkLayerDevice) EthernetDeviceForBridge(
	name string, askProviderForAddress bool,
	allSubnets network.SubnetInfos,
) (network.InterfaceInfo, error) {
	var newDev network.InterfaceInfo

	if !dev.isBridge() {
		return newDev, errors.Errorf("device must be a Bridge Device, but is type %q", dev.Type())
	}

	mtu, err := dev.mtuForChild()
	if err != nil {
		return network.InterfaceInfo{}, errors.Annotate(err, "determining child MTU")
	}

	newDev = network.InterfaceInfo{
		InterfaceName:       name,
		MACAddress:          network.GenerateVirtualMACAddress(),
		ConfigType:          network.ConfigDHCP,
		InterfaceType:       network.EthernetDevice,
		MTU:                 int(mtu),
		ParentInterfaceName: dev.Name(),
		VirtualPortType:     dev.VirtualPortType(),
	}

	addrs, err := dev.Addresses()
	if err != nil {
		return network.InterfaceInfo{}, errors.Trace(err)
	}

	// Include a single address without an IP, but with a CIDR
	// to indicate that we know the subnet for this bridge.
	if len(addrs) > 0 {
		addr := addrs[0]
		if askProviderForAddress {
			subnets, err := allSubnets.GetByCIDR(addr.SubnetCIDR())
			if err != nil {
				return newDev, errors.Annotatef(err,
					"retrieving subnet %q used by address %q of host machine device %q",
					addr.SubnetCIDR(), addr.Value(), dev.Name(),
				)
			}
			// Only one network should be returned for the given
			// address, so we can safely get its first element.
			sub := subnets[0]
			newDev.ConfigType = network.ConfigStatic
			newDev.ProviderSubnetId = sub.ProviderId
			newDev.VLANTag = sub.VLANTag
			newDev.IsDefaultGateway = addr.IsDefaultGateway()
			newDev.Addresses = network.ProviderAddresses{
				network.NewMachineAddress("", network.WithCIDR(sub.CIDR)).AsProviderAddress()}
		} else {
			newDev.Addresses = network.ProviderAddresses{
				network.NewMachineAddress("", network.WithCIDR(addr.SubnetCIDR())).AsProviderAddress()}
		}
	}

	return newDev, nil
}

// mtuForChild returns a suitable MTU to use for a child of this device.
// At the time of writing, Fan devices are configured with a static MTU.
// See /usr/sbin/fanctl. It is either 1480 or (usually) 1450, which appears
// to be a lazy 50 less than the common 1500. Using this value can cause
// issues if the underlay has a MTU lower than 1450. If this is a Fan device,
// locate the accompanying VXLAN device instead, and use that MTU.
// This should have the correct value relative to the underlay.
func (dev *LinkLayerDevice) mtuForChild() (uint, error) {
	if !strings.HasPrefix(dev.doc.Name, "fan-") {
		return dev.MTU(), nil
	}

	linkLayerDevs, closer := dev.st.db().GetCollection(linkLayerDevicesC)
	defer closer()

	var resultDoc struct {
		MTU uint `bson:"mtu"`
	}
	err := linkLayerDevs.Find(bson.D{
		{"machine-id", dev.doc.MachineID},
		{"parent-name", dev.doc.Name},
		{"type", network.VXLANDevice},
	}).Select(bson.D{{"mtu", 1}}).One(&resultDoc)

	return resultDoc.MTU, errors.Trace(err)
}

func (dev *LinkLayerDevice) isBridge() bool {
	if dev.Type() == network.BridgeDevice {
		return true
	}

	// OVS bridges expose their internal port as a plain NIC with the
	// same name as the bridge.
	if dev.VirtualPortType() == network.OvsPort {
		return true
	}

	return false
}
