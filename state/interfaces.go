// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// interfaceDoc describes the persistent state of a machine network interface.
type interfaceDoc struct {
	// DocID is the interface global key, prefixed by ModelUUID.
	DocID string `bson:"_id"`

	// Name is the device name of the interface as it appears on the machine.
	Name string `bson:"name"`

	// ModelUUID is the UUID of the model this interface is part of.
	ModelUUID string `bson:"model-uuid"`

	// Index is the zero-based device index of the interface as it appears on
	// the machine.
	Index uint `bson:"index"`

	// MTU is the maximum transmission unit the interface can handle.
	MTU uint `bson:"mtu"`

	// ProviderID is a provider-specific ID of the interface, prefixed by
	// ModelUUID. Empty when not supported by the provider.
	ProviderID string `bson:"providerid,omitempty"`

	// MachineID is the ID of the machine where this interface is located.
	MachineID string `bson:"machine-id"`

	// Type is the type of the interface related to the underlying device.
	Type InterfaceType `bson:"type"`

	// HardwareAddress is the hardware address for the interface, usually a MAC
	// address.
	HardwareAddress string `bson:"hardware-address"`

	// IsAutoStart is true if the interface should be activated on boot.
	IsAutoStart bool `bson:"is-auto-start"`

	// IsUp is true when the interface is up (enabled).
	IsUp bool `bson:"is-up"`

	// ParentName is the name of the parent interface, which may be empty.
	ParentName string `bson:"parent-name"`

	// DNSServers is an optional list of DNS nameservers that apply to this
	// interface.
	DNSServers []string `bson:"dns-servers,omitempty"`

	// DNSSearchDomains is an optional list of DNS domain names used to
	// construct a fully-qualified hostname.
	DNSSearchDomains []string `bson:"dns-search-domains,omitempty"`

	// GatewayAddress is the gateway to use for this interface, which may be
	// empty.
	GatewayAddress string `bson:"gateway-address,omitempty"`
}

// InterfaceType defines the type of a machine network interface.
type InterfaceType string

const (
	// UnknownInterface is used for interfaces with unknown type.
	UnknownInterface InterfaceType = "unknown"

	// LoopbackInterface is used for loopback interfaces.
	LoopbackInterface InterfaceType = "loopback"

	// EthernetInterface is used for interfaces representing Ethernet devices.
	EthernetInterface InterfaceType = "ethernet"

	// VLANInterface is used for interfaces representing IEEE 802.11Q VLAN
	// devices.
	VLANInterface InterfaceType = "vlan"

	// BondInterface is used for interfaces representing bonding devices.
	BondInterface InterfaceType = "bond"

	// BridgeInterface is used for interfaces representing an OSI layer-2 bridge
	// device.
	BridgeInterface InterfaceType = "bridge"
)

// IsValidInterfaceType returns whether the given value is a valid interface
// type.
func IsValidInterfaceType(value string) bool {
	switch InterfaceType(value) {
	case UnknownInterface, LoopbackInterface, EthernetInterface, VLANInterface, BondInterface, BridgeInterface:
		return true
	}
	return false
}

// Interface represents the state of a machine network interface.
type Interface struct {
	st  *State
	doc interfaceDoc
}

func newInterface(st *State, doc interfaceDoc) *Interface {
	return &Interface{st: st, doc: doc}
}

// DocID returns the globally unique interface ID, including the model UUID as
// prefix.
func (nic *Interface) DocID() string {
	return nic.st.docID(nic.doc.DocID)
}

// Name returns the interface name as it appears on its machine.
func (nic *Interface) Name() string {
	return nic.doc.Name
}

// Index returns the interface's device index as it appears on its machine.
func (nic *Interface) Index() uint {
	return nic.doc.Index
}

// MTU returns the maximum transmission unit the interface can handle.
func (nic *Interface) MTU() uint {
	return nic.doc.MTU
}

// ProviderID returns the provider-specific interface ID, if set.
func (nic *Interface) ProviderID() network.Id {
	localProviderID := nic.st.localID(nic.doc.ProviderID)
	return network.Id(localProviderID)
}

// MachineID returns the ID of the machine this interface is on.
func (nic *Interface) MachineID() string {
	return nic.doc.MachineID
}

// Type returns the type of the interface.
func (nic *Interface) Type() InterfaceType {
	return nic.doc.Type
}

// HardwareAddress returns the hardware address of the interface, usually a MAC
// address.
func (nic *Interface) HardwareAddress() string {
	return nic.doc.HardwareAddress
}

// IsAutoStart returns whether the interface is configured to automatically
// start on boot.
func (nic *Interface) IsAutoStart() bool {
	return nic.doc.IsAutoStart
}

// IsUp returns whether the interface is currently up.
func (nic *Interface) IsUp() bool {
	return nic.doc.IsUp
}

// ParentName returns the name of this interface's parent interface, if set.
func (nic *Interface) ParentName() string {
	return nic.doc.ParentName
}

// ParentInterface returns the Interface corresponding to this interface's
// parent (if specified). When no parent interface name is set, it returns nil
// and no error.
func (nic *Interface) ParentInterface() (*Interface, error) {
	if nic.doc.ParentName == "" {
		return nil, nil
	}

	parentGlobalKey := InterfaceGlobalKey(nic.doc.MachineID, nic.doc.ParentName)
	return nic.st.Interface(parentGlobalKey)
}

// Refresh refreshes the contents of the inteface from the underlying state. It
// returns an error that satisfies errors.IsNotFound if the interface has been
// removed.
func (nic *Interface) Refresh() error {
	freshCopy, err := nic.st.Interface(nic.globalKey())
	if errors.IsNotFound(err) {
		return err
	} else if err != nil {
		return errors.Annotatef(err, "cannot refresh interface %s", nic)
	}
	nic.doc = freshCopy.doc
	return nil
}

// Interface returns the interface matching the given globalKey. An error
// satisfying errors.IsNotFound() is returned when no such interface exists.
func (st *State) Interface(globalKey string) (*Interface, error) {
	interfaces, closer := st.getCollection(interfacesC)
	defer closer()

	var doc interfaceDoc
	err := interfaces.FindId(globalKey).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("interface %q", globalKey)
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot get interface %q", globalKey)
	}
	return newInterface(st, doc), nil
}

// AddInterfaceArgs contains the arguments accepted by Machine.AddInterface().
type AddInterfaceArgs struct {
	// Name is the device name of the interface as it appears on the machine.
	Name string

	// Index is the zero-based device index of the interface as it appears on
	// the machine.
	Index uint

	// MTU is the maximum transmission unit the interface can handle.
	MTU uint

	// ProviderID is a provider-specific ID of the interface. Empty when not
	// supported by the provider.
	ProviderID network.Id

	// Type is the type of the interface related to the underlying device.
	Type InterfaceType

	// HardwareAddress is the hardware address for the interface, usually a MAC
	// address.
	HardwareAddress string

	// IsAutoStart is true if the interface should be activated on boot.
	IsAutoStart bool

	// IsUp is true when the interface is up (enabled).
	IsUp bool

	// ParentName is the name of the parent interface, which may be empty. If
	// set, it needs to be an existing interface on the same machine. Traffic
	// originating from an interface egresses from the parent interface.
	ParentName string

	// DNSServers is an optional list of DNS nameservers that apply for this
	// interface.
	DNSServers []string

	// DNSSearchDomains is an optional DNS domain names to use for qualifying
	// hostnames.
	DNSSearchDomains []string

	// GatewayAddress is the gateway to use for this interface, which may be
	// empty.
	GatewayAddress string
}

// AddInterface creates a new interface on the machine, initialized from the
// given args. ProviderID from args can be empty if not supported by the
// provider, but when set must be unique within the model or an error is
// returned. If the machine is not found or not Alive, an error is returned. If
// an interface with the same name already exists, an error satisfying
// errors.IsAlreadyExists() is returned. If any of the fields in args contain an
// invalid value, an error satisfying errors.IsNotValid() is returned. When
// ParentName is not empty, it must refer to an existing interface on the same
// machine, otherwise an error satisfying errors.IsNotFound() is returned.
func (m *Machine) AddInterface(args AddInterfaceArgs) (_ *Interface, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add interface %q to machine %q", args.Name, m.doc.Id)

	if err := validateAddInterfaceArgs(args); err != nil {
		return nil, err
	}

	newInterfaceDoc := m.newInterfaceDocFromArgs(args)
	parentGlobalKey := InterfaceGlobalKey(m.doc.Id, args.ParentName)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := checkModeLife(m.st); err != nil {
				return nil, errors.Trace(err)
			}

			if machineAlive, err := isAlive(m.st, machinesC, m.doc.Id); err != nil {
				return nil, errors.Trace(err)
			} else if !machineAlive {
				return nil, errors.Errorf("machine not found or not alive")
			}

			if parentGlobalKey != "" {
				if _, err := m.st.Interface(parentGlobalKey); errors.IsNotFound(err) {
					return nil, errors.NotFoundf("parent interface %q", args.ParentName)
				} else if err != nil {
					return nil, errors.Trace(err)
				}
			}

			globalKey := InterfaceGlobalKey(m.doc.Id, args.Name)
			if _, err := m.st.Interface(globalKey); err == nil {
				return nil, errors.AlreadyExistsf("interface")
			} else if !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
		}

		ops := []txn.Op{
			assertModelAliveOp(m.st.ModelUUID()),
			{
				C:      machinesC,
				Id:     m.doc.Id,
				Assert: isAliveDoc,
			}, {
				C:      interfacesC,
				Id:     newInterfaceDoc.DocID,
				Assert: txn.DocMissing,
				Insert: newInterfaceDoc,
			},
		}

		if parentGlobalKey != "" {
			parentDocID := m.st.docID(parentGlobalKey)
			ops = append(ops, txn.Op{
				C:      interfacesC,
				Id:     parentDocID,
				Assert: txn.DocExists,
			})
		}

		return ops, nil
	}
	err = m.st.run(buildTxn)
	if err == nil {
		addedInterface := newInterface(m.st, newInterfaceDoc)
		// If the ProviderID was not unique adding the interface can fail
		// without an error. Refreshing catches this by returning NotFoundError.
		if err := addedInterface.Refresh(); errors.IsNotFound(err) {
			return nil, errors.Errorf("ProviderID %q not unique", args.ProviderID)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return addedInterface, nil
	}
	return nil, errors.Trace(err)
}

// validateAddInterfaceArgs performs a quick sanity check on args before trying
// to add the interface.
func validateAddInterfaceArgs(args AddInterfaceArgs) error {
	if args.Name == "" {
		return errors.NotValidf("empty Name")
	}
	if !IsValidInterfaceName(args.Name) {
		return errors.NotValidf("Name %q", args.Name)
	}

	if args.ParentName != "" && !IsValidInterfaceName(args.ParentName) {
		return errors.NotValidf("ParentName %q", args.ParentName)
	}

	if !IsValidInterfaceType(string(args.Type)) {
		return errors.NotValidf("Type %q", args.Type)
	}

	if args.HardwareAddress != "" {
		if _, err := net.ParseMAC(args.HardwareAddress); err != nil {
			return errors.NotValidf("HardwareAddress %q", args.HardwareAddress)
		}
	}
	if args.GatewayAddress != "" && net.ParseIP(args.GatewayAddress) == nil {
		return errors.NotValidf("GatewayAddress %q", args.GatewayAddress)
	}
	return nil
}

// newInterfaceDocFromArgs returns an interfaceDoc populated from args for the
// machine.
func (m *Machine) newInterfaceDocFromArgs(args AddInterfaceArgs) interfaceDoc {
	globalKey := InterfaceGlobalKey(m.doc.Id, args.Name)
	interfaceDocID := m.st.docID(globalKey)

	providerID := string(args.ProviderID)
	if providerID != "" {
		providerID = m.st.docID(providerID)
	}

	modelUUID := m.st.ModelUUID()

	return interfaceDoc{
		DocID:            interfaceDocID,
		Name:             args.Name,
		ModelUUID:        modelUUID,
		Index:            args.Index,
		MTU:              args.MTU,
		ProviderID:       providerID,
		MachineID:        m.doc.Id,
		Type:             args.Type,
		HardwareAddress:  args.HardwareAddress,
		IsAutoStart:      args.IsAutoStart,
		IsUp:             args.IsUp,
		ParentName:       args.ParentName,
		DNSServers:       args.DNSServers,
		DNSSearchDomains: args.DNSSearchDomains,
		GatewayAddress:   args.GatewayAddress,
	}
}

// DNSServers returns the list of DNS nameservers that apply to this interface,
// if they are known, or an empty slice otherwise.
func (nic *Interface) DNSServers() []string {
	return nic.doc.DNSServers
}

// DNSSearchDomains returns the list of DNS domain names used to qualify
// hostnames. Can be empty when not set.
func (nic *Interface) DNSSearchDomains() []string {
	return nic.doc.DNSSearchDomains
}

// GatewayAddress returns the address of the gateway to use for this interface,
// if set.
func (nic *Interface) GatewayAddress() string {
	return nic.doc.GatewayAddress
}

func (nic *Interface) globalKey() string {
	return InterfaceGlobalKey(nic.doc.MachineID, nic.doc.Name)
}

// String returns the interface as a human-readable string.
func (nic *Interface) String() string {
	return fmt.Sprintf("%s interface %q on machine %q", nic.doc.Type, nic.doc.Name, nic.doc.MachineID)
}

// InterfaceGlobalKey returns a (model-unique) global key for an interface. If
// either argument is empty, the result is empty.
func InterfaceGlobalKey(machineID, interfaceName string) string {
	if machineID == "" || interfaceName == "" {
		return ""
	}
	return "m#" + machineID + "i#" + interfaceName
}

// IsValidInterfaceName returns whether the given interfaceName is a valid
// network device name, depending on the runtime.GOOS value.
func IsValidInterfaceName(interfaceName string) bool {
	if runtimeGOOS == "linux" {
		return isValidLinuxDeviceName(interfaceName)
	}
	hasHash := strings.Contains(interfaceName, "#")
	return !hasHash && stringLengthBetween(interfaceName, 1, 255)
}

// runtimeGOOS is defined to allow patching in tests.
var runtimeGOOS = runtime.GOOS

// isValidLinuxDeviceName returns whether the given deviceName is valid,
// using the same criteria as dev_valid_name(9) in the Linux kernel:
// - no whitespace allowed
// - length from 1 to 15 ASCII characters
// - literal "." and ".." as names are not allowed.
// Additionally, we don't allow "#" in the name.
func isValidLinuxDeviceName(deviceName string) bool {
	hasWhitespace := whitespaceReplacer.Replace(deviceName) != deviceName
	isDot, isDoubleDot := deviceName == ".", deviceName == ".."
	hasValidLength := stringLengthBetween(deviceName, 1, 15)
	hasHash := strings.Contains(deviceName, "#")

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
