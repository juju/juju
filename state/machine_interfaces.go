// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// Interface returns the interface matching the given name. An error satisfying
// errors.IsNotFound() is returned when no such interface exists on the machine.
func (m *Machine) Interface(name string) (*Interface, error) {
	interfaces, closer := m.st.getCollection(interfacesC)
	defer closer()

	var doc interfaceDoc
	globalKey := interfaceGlobalKey(m.doc.Id, name)
	err := interfaces.FindId(globalKey).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("interface %q on machine %q", name, m.doc.Id)
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot get interface %q on machine %q", name, m.doc.Id)
	}
	return newInterface(m.st, doc), nil
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
// provider, but when set must be unique within the model. Errors are returned
// in the following cases:
// - ProviderID not unique (when set);
// - Machine is no longer alive or it's missing;
// - errors.NotValidError, when any of the fields in args contain invalid values;
// - errors.NotFoundError, when ParentName is set but cannot be found;
// - errors.AlreadyExistsError, when Name is set to an existing interface.
func (m *Machine) AddInterface(args AddInterfaceArgs) (_ *Interface, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add interface %q to machine %q", args.Name, m.doc.Id)

	if err := validateAddInterfaceArgs(args); err != nil {
		return nil, err
	}

	newInterfaceDoc := m.newInterfaceDocFromArgs(args)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := checkModeLife(m.st); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.ensureStillAlive(); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.ensureParentInterfaceExistsWhenSet(args.ParentName); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.ensureInterfaceDoesNotExistYet(args.Name); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops := []txn.Op{
			assertModelAliveOp(m.st.ModelUUID()),
			m.assertAliveOp(),
			insertInterfaceDocOp(newInterfaceDoc),
		}
		return m.maybeAssertParentInterfaceExistsOp(args.ParentName, ops), nil
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
	globalKey := interfaceGlobalKey(m.doc.Id, args.Name)
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

func (m *Machine) ensureStillAlive() error {
	if machineAlive, err := isAlive(m.st, machinesC, m.doc.Id); err != nil {
		return errors.Trace(err)
	} else if !machineAlive {
		return errors.Errorf("machine not found or not alive")
	}
	return nil
}

func (m *Machine) ensureParentInterfaceExistsWhenSet(parentInterfaceName string) error {
	if parentInterfaceName == "" {
		return nil
	}
	if _, err := m.Interface(parentInterfaceName); errors.IsNotFound(err) {
		return errors.NotFoundf("parent interface %q", parentInterfaceName)
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *Machine) ensureInterfaceDoesNotExistYet(interfaceName string) error {
	if _, err := m.Interface(interfaceName); err == nil {
		return errors.AlreadyExistsf("interface")
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

func insertInterfaceDocOp(newInterfaceDoc interfaceDoc) txn.Op {
	return txn.Op{
		C:      interfacesC,
		Id:     newInterfaceDoc.DocID,
		Assert: txn.DocMissing,
		Insert: newInterfaceDoc,
	}
}

func (m *Machine) maybeAssertParentInterfaceExistsOp(parentInterfaceName string, ops []txn.Op) []txn.Op {
	if parentInterfaceName == "" {
		return ops
	}
	parentGlobalKey := interfaceGlobalKey(m.doc.Id, parentInterfaceName)
	parentDocID := m.st.docID(parentGlobalKey)
	return append(ops, txn.Op{
		C:      interfacesC,
		Id:     parentDocID,
		Assert: txn.DocExists,
	})
}
