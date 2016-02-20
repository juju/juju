// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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

// AllInterfaces returns all exiting interfaces of the machine.
func (m *Machine) AllInterfaces() ([]*Interface, error) {
	var allInterfaces []*Interface
	callbackFunc := func(resultDoc *interfaceDoc) {
		allInterfaces = append(allInterfaces, newInterface(m.st, *resultDoc))
	}

	if err := m.forEachInterfaceDoc(nil, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allInterfaces, nil
}

func (m *Machine) forEachInterfaceDoc(selectOnly bson.D, callbackFunc func(resultDoc *interfaceDoc)) error {
	interfaces, closer := m.st.getCollection(interfacesC)
	defer closer()

	query := interfaces.Find(bson.D{{"machine-id", m.doc.Id}})
	if selectOnly != nil {
		query = query.Select(selectOnly)
	}
	iter := query.Iter()

	var resultDoc interfaceDoc
	for iter.Next(&resultDoc) {
		callbackFunc(&resultDoc)
	}

	return errors.Trace(iter.Close())
}

// RemoveAllInterfaces deletes all existing interfaces of the machine in a
// single transaction. No error is returned when some or all of the interfaces
// were already removed.
func (m *Machine) RemoveAllInterfaces() error {
	var removeAllInterfacesOps []txn.Op
	callbackFunc := func(resultDoc *interfaceDoc) {
		removeOp := removeInterfaceOp(resultDoc.DocID)
		removeAllInterfacesOps = append(removeAllInterfacesOps, removeOp)
	}

	selectDocIDOnly := bson.D{{"_id", 1}}
	if err := m.forEachInterfaceDoc(selectDocIDOnly, callbackFunc); err != nil {
		return errors.Trace(err)
	}

	return m.st.runTransaction(removeAllInterfacesOps)
}

// InterfaceArgs contains the arguments accepted by Machine.AddInterfaces().
type InterfaceArgs struct {
	// Name is the device name of the interface as it appears on the machine.
	Name string

	// Index is the zero-based device index of the interface as it appears on
	// the machine.
	Index uint

	// MTU is the maximum transmission unit the interface can handle.
	MTU uint

	// ProviderID is a provider-specific ID of the interface. Empty when not
	// supported by the provider. Cannot be unset with UpdateInterface once set.
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

// AddInterfaces creates one or more interfaces on the machine, each initialized
// from the items in the given interfacesArgs, and using a single transaction
// for all. ProviderID field can be empty if not supported by the provider, but
// when set must be unique within the model. Errors are returned and no changes
// are applied, in the following cases:
// - ProviderID not unique (when set);
// - Machine is no longer alive or it's missing;
// - errors.NotValidError, when any of the fields in args contain invalid values;
// - errors.NotFoundError, when ParentName is set but cannot be found on the
//   machine or in interfacesArgs;
// - errors.AlreadyExistsError, when Name is set to an existing interface.
func (m *Machine) AddInterfaces(interfacesArgs ...InterfaceArgs) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add interfaces to machine %q", m.doc.Id)

	if len(interfacesArgs) == 0 {
		return errors.Errorf("no interfaces to add")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		existingProviderIDs, err := m.st.allProviderIDsForModelInterfaces()
		if err != nil {
			return nil, errors.Trace(err)
		}
		newDocs, pendingNames, err := m.prepareToAddInterfaces(interfacesArgs, existingProviderIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if attempt > 0 {
			if err := checkModeLife(m.st); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.ensureStillAlive(); err != nil {
				return nil, errors.Trace(err)
			}
			if err := m.ensureInterfaceDocsStillValid(newDocs, pendingNames); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops := []txn.Op{
			assertModelAliveOp(m.st.ModelUUID()),
			m.assertAliveOp(),
		}

		for _, newDoc := range newDocs {
			ops = append(ops, insertInterfaceDocOp(&newDoc))
			ops = m.maybeAssertParentInterfaceExists(newDoc.ParentName, pendingNames, ops)
		}
		return ops, nil
	}
	if err := m.st.run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	// Even without an error, we still need to verify if any of the newDocs was
	// not inserted due to ProviderID unique index violation.
	return m.rollbackUnlessAllInterfacesInArgsInserted(interfacesArgs)
}

func (st *State) allProviderIDsForModelInterfaces() (_ set.Strings, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot get ProviderIDs of all interfaces")

	interfaces, closer := st.getCollection(interfacesC)
	defer closer()

	allProviderIDs := set.NewStrings()
	var doc struct {
		ProviderID string `bson:"providerid"`
	}

	pattern := fmt.Sprintf("^%s:.+$", st.ModelUUID())
	selector := bson.D{{"providerid", bson.D{{"$regex", pattern}}}}

	iter := interfaces.Find(selector).Select(bson.D{{"providerid", 1}}).Iter()
	for iter.Next(&doc) {
		localProviderID := st.localID(doc.ProviderID)
		allProviderIDs.Add(localProviderID)
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return allProviderIDs, nil
}

func (m *Machine) prepareToAddInterfaces(interfacesArgs []InterfaceArgs, existingProviderIDs set.Strings) ([]interfaceDoc, set.Strings, error) {
	var pendingDocs []interfaceDoc
	pendingNames := set.NewStrings()
	allProviderIDs := set.NewStrings(existingProviderIDs.Values()...)

	for _, args := range interfacesArgs {
		newDoc, err := m.prepareOneAddInterfacesArgs(&args, pendingNames, allProviderIDs)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		pendingNames.Add(args.Name)
		pendingDocs = append(pendingDocs, *newDoc)
		if args.ProviderID != "" {
			allProviderIDs.Add(string(args.ProviderID))
		}
	}
	return pendingDocs, pendingNames, nil
}

func (m *Machine) prepareOneAddInterfacesArgs(args *InterfaceArgs, pendingNames, allProviderIDs set.Strings) (_ *interfaceDoc, err error) {
	defer errors.DeferredAnnotatef(&err, "invalid interface %q", args.Name)

	if err := validateAddInterfaceArgs(args); err != nil {
		return nil, errors.Trace(err)
	}

	if pendingNames.Contains(args.Name) {
		return nil, errors.NewNotValid(nil, "Name specified more than once")
	}

	if allProviderIDs.Contains(string(args.ProviderID)) {
		errorMessage := fmt.Sprintf("ProviderID %q not unique", args.ProviderID)
		return nil, errors.NewNotValid(nil, errorMessage)
	}

	return m.newInterfaceDocFromArgs(args), nil
}

func validateAddInterfaceArgs(args *InterfaceArgs) error {
	if args.Name == "" {
		return errors.NotValidf("empty Name")
	}
	if !IsValidInterfaceName(args.Name) {
		return errors.NotValidf("Name %q", args.Name)
	}

	if args.ParentName != "" {
		if !IsValidInterfaceName(args.ParentName) {
			return errors.NotValidf("ParentName %q", args.ParentName)
		}
		if args.Name == args.ParentName {
			return errors.NewNotValid(nil, "Name and ParentName must be different")
		}
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

func (m *Machine) newInterfaceDocFromArgs(args *InterfaceArgs) *interfaceDoc {
	globalKey := interfaceGlobalKey(m.doc.Id, args.Name)
	interfaceDocID := m.st.docID(globalKey)

	providerID := string(args.ProviderID)
	if providerID != "" {
		providerID = m.st.docID(providerID)
	}

	modelUUID := m.st.ModelUUID()

	return &interfaceDoc{
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

func (m *Machine) ensureInterfaceDocsStillValid(newDocs []interfaceDoc, pendingNames set.Strings) error {
	for _, newDoc := range newDocs {
		if err := m.maybeEnsureParentInterfaceExists(newDoc.Name, newDoc.ParentName, pendingNames); err != nil {
			return errors.Trace(err)
		}
		if err := m.ensureInterfaceDoesNotExistYet(newDoc.Name); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (m *Machine) maybeEnsureParentInterfaceExists(name, parentName string, pendingNames set.Strings) error {
	if parentName == "" || pendingNames.Contains(parentName) {
		return nil
	}
	if _, err := m.Interface(parentName); errors.IsNotFound(err) {
		return errors.NotFoundf("parent interface %q of interface %q", parentName, name)
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (m *Machine) ensureInterfaceDoesNotExistYet(interfaceName string) error {
	if _, err := m.Interface(interfaceName); err == nil {
		return errors.AlreadyExistsf("interface %q", interfaceName)
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

func (m *Machine) maybeAssertParentInterfaceExists(parentName string, pendingNames set.Strings, ops []txn.Op) []txn.Op {
	if parentName == "" || pendingNames.Contains(parentName) {
		return ops
	}

	parentGlobalKey := interfaceGlobalKey(m.doc.Id, parentName)
	parentDocID := m.st.docID(parentGlobalKey)
	return append(ops, assertInterfaceExistsOp(parentDocID))
}

func (m *Machine) rollbackUnlessAllInterfacesInArgsInserted(interfacesArgs []InterfaceArgs) error {
	usedProviderIDs := set.NewStrings()
	assertExistsOps := make([]txn.Op, len(interfacesArgs))
	removeOps := make([]txn.Op, len(interfacesArgs))

	for i, args := range interfacesArgs {
		interfaceDocID := interfaceGlobalKey(m.doc.Id, args.Name)
		assertExistsOps[i] = assertInterfaceExistsOp(interfaceDocID)
		removeOps[i] = removeInterfaceOp(interfaceDocID)
		if args.ProviderID != "" {
			usedProviderIDs.Add(string(args.ProviderID))
		}
	}

	var wasAborted bool
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// One or more documents were not inserted due to ProviderID unique
			// index violation, so collect the duplicates and remove all
			// inserted documents.
			wasAborted = true
			return removeOps, nil
		}
		return assertExistsOps, nil
	}
	err := m.st.run(buildTxn)
	if err == nil && wasAborted {
		idList := strings.Join(usedProviderIDs.SortedValues(), ", ")
		return errors.Errorf("one or more non-unique ProviderIDs specified: %s", idList)
	}
	return errors.Trace(err)
}
