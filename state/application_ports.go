// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
)

// applicationPortRangesDoc represents the state of ports opened for an application.
type applicationPortRangesDoc struct {
	DocID           string                    `bson:"_id"`
	ModelUUID       string                    `bson:"model-uuid"`
	ApplicationName string                    `bson:"application-name"`
	PortRanges      network.GroupedPortRanges `bson:"port-ranges"`
	TxnRevno        int64                     `bson:"txn-revno"`
}

func newApplicationPortRangesDoc(modelUUID, appName string, pgs network.GroupedPortRanges) applicationPortRangesDoc {
	return applicationPortRangesDoc{
		DocID:           applicationGlobalKey(appName),
		ModelUUID:       modelUUID,
		ApplicationName: appName,
		PortRanges:      pgs,
	}
}

// ApplicationPortRanges is implemented by types that can query and/or
// manipulate the set of port ranges opened by an application.
type ApplicationPortRanges interface {
	// ApplicationName returns the name of the application these ranges apply to.
	ApplicationName() string

	// Open records a request for opening the specified port range for the
	// specified endpoint.
	Open(endpoint string, portRange network.PortRange)

	// Close records a request for closing a particular port range for the
	// specified endpoint.
	Close(endpoint string, portRange network.PortRange)

	Persisted() bool

	// Changes returns a ModelOperation for applying any changes that were
	// made to this port range instance for an application.
	Changes() ModelOperation

	Remove() error
	Refresh() error

	// UniquePortRanges returns a slice of unique open PortRanges for
	// an application.
	UniquePortRanges() []network.PortRange
}

type applicationPortRanges struct {
	st  *State
	doc applicationPortRangesDoc

	// docExists is false if the port range doc has not yet been persisted
	// to the backing store.
	docExists bool

	// The set of pending port ranges that have not yet been persisted.
	pendingOpenRanges  network.GroupedPortRanges
	pendingCloseRanges network.GroupedPortRanges
}

// ApplicationName returns the application name associated with this set of port ranges.
func (p *applicationPortRanges) ApplicationName() string {
	return p.doc.ApplicationName
}

// Open records a request for opening a particular port range for the specified
// endpoint.
func (p *applicationPortRanges) Open(endpoint string, portRange network.PortRange) {
	if p.pendingOpenRanges == nil {
		p.pendingOpenRanges = make(network.GroupedPortRanges)
	}

	p.pendingOpenRanges[endpoint] = append(p.pendingOpenRanges[endpoint], portRange)
}

// Close records a request for closing a particular port range for the
// specified endpoint.
func (p *applicationPortRanges) Close(endpoint string, portRange network.PortRange) {
	if p.pendingCloseRanges == nil {
		p.pendingCloseRanges = make(network.GroupedPortRanges)
	}

	p.pendingCloseRanges[endpoint] = append(p.pendingCloseRanges[endpoint], portRange)
}

// Persisted returns true if the underlying document for this instance exists
// in the database.
func (p *applicationPortRanges) Persisted() bool {
	return p.docExists
}

// Changes returns a ModelOperation for applying any changes that were made to
// this port range instance for all machine units.
func (p *applicationPortRanges) Changes() ModelOperation {
	return newApplicationPortRangesOperation(p)
}

func (p *applicationPortRanges) Remove() error {
	doc := &applicationPortRanges{st: p.st, doc: p.doc, docExists: p.docExists}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := doc.Refresh()
			if errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}
		return doc.removeOps(), nil
	}
	if err := p.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}

	p.docExists = false
	return nil
}

// Refresh refreshes the port document from state.
func (p *applicationPortRanges) Refresh() error {
	openedPorts, closer := p.st.db().GetCollection(openedPortsC)
	defer closer()

	err := openedPorts.FindId(p.doc.DocID).One(&p.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("open port ranges for application %q", p.ApplicationName())
	}
	if err != nil {
		return errors.Annotatef(err, "refresh open port ranges for application %q", p.ApplicationName())
	}
	return nil
}

// UniquePortRanges returns a slice of unique open PortRanges for the application.
func (p *applicationPortRanges) UniquePortRanges() []network.PortRange {
	allRanges := p.doc.PortRanges.UniquePortRanges()
	network.SortPortRanges(allRanges)
	return allRanges
}

func (p *applicationPortRanges) removeOps() []txn.Op {
	return []txn.Op{{
		C:      openedPortsC,
		Id:     p.doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}
}

var _ ModelOperation = (*applicationPortRangesOperation)(nil)

type applicationPortRangesOperation struct {
	apr *applicationPortRanges

	updatedPortRanges network.GroupedPortRanges
}

func newApplicationPortRangesOperation(apr *applicationPortRanges) ModelOperation {
	return &applicationPortRangesOperation{
		apr:               apr,
		updatedPortRanges: apr.doc.PortRanges.Clone(),
	}
}

func (op *applicationPortRangesOperation) Build(attempt int) ([]txn.Op, error) {
	if err := checkModelNotDead(op.apr.st); err != nil {
		return nil, errors.Annotate(err, "cannot open/close ports")
	}

	var createDoc = !op.apr.docExists
	if attempt > 0 {
		if err := op.apr.Refresh(); err != nil {
			if !errors.IsNotFound(err) {
				return nil, errors.Annotate(err, "cannot open/close ports")
			}

			// Doc not found; we need to create it.
			createDoc = true
		}
	}

	if err := op.validatePendingChanges(); err != nil {
		return nil, errors.Annotate(err, "cannot open/close ports")
	}

	ops := []txn.Op{
		assertModelNotDeadOp(op.apr.st.ModelUUID()),
		assertApplicationAliveOp(op.apr.ApplicationName()),
	}

	portListModified, err := op.mergePendingOpenPortRanges()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modified, err := op.mergePendingClosePortRanges()
	if err != nil {
		return nil, errors.Trace(err)
	}
	portListModified = portListModified || modified

	if !portListModified || (createDoc && len(op.updatedPortRanges) == 0) {
		return nil, jujutxn.ErrNoOperations
	}

	if createDoc {
		assert := txn.DocMissing
		ops = append(ops, insertAppPortRangesDocOps(op.apr.st, &op.apr.doc, assert, op.updatedPortRanges)...)
	} else if len(op.updatedPortRanges) == 0 {
		// Port list is empty; get rid of ports document.
		ops = append(ops, op.apr.removeOps()...)
	} else {
		assert := bson.D{{Name: "txn-revno", Value: op.apr.doc.TxnRevno}}
		ops = append(ops, updateAppPortRangesDocOps(op.apr.st, &op.apr.doc, assert, op.updatedPortRanges)...)
	}

	return ops, nil
}

func (op *applicationPortRangesOperation) mergePendingOpenPortRanges() (bool, error) {
	var modified bool
	for endpointName, pendingRanges := range op.apr.pendingOpenRanges {
		for _, pendingRange := range pendingRanges {
			if op.rangeExistsForEndpoint(endpointName, pendingRange) {
				// Exists, no op for opening.
				continue
			}
			op.updatedPortRanges[endpointName] = append(op.updatedPortRanges[endpointName], pendingRange)
			modified = true
		}
	}
	return modified, nil
}

func (op *applicationPortRangesOperation) mergePendingClosePortRanges() (bool, error) {
	var modified bool
	for endpointName, pendingRanges := range op.apr.pendingCloseRanges {
		for _, pendingRange := range pendingRanges {
			if !op.rangeExistsForEndpoint(endpointName, pendingRange) {
				// Not exists, no op for closing.
				continue
			}
			modified = op.removePortRange(endpointName, pendingRange) || modified
		}
	}
	return modified, nil
}

func (op *applicationPortRangesOperation) removePortRange(endpointName string, portRange network.PortRange) bool {
	var modified bool
	existingRanges := op.updatedPortRanges[endpointName]
	for i, existingRange := range existingRanges {
		if existingRange != portRange {
			continue
		}
		existingRanges = append(existingRanges[:i], existingRanges[i+1:]...)
		op.updatedPortRanges[endpointName] = existingRanges
		modified = true
	}
	return modified
}

func (op *applicationPortRangesOperation) rangeExistsForEndpoint(endpointName string, portRange network.PortRange) bool {
	// For k8s applications, no endpoint level portrange supported currently.
	// There is only one endpoint(which is empty string - "").
	if len(op.updatedPortRanges[endpointName]) == 0 {
		return false
	}

	for _, existingRange := range op.updatedPortRanges[endpointName] {
		if existingRange == portRange {
			return true
		}
	}
	return false
}

func (op *applicationPortRangesOperation) getEndpointBindings() (set.Strings, error) {
	appEndpoints := set.NewStrings()
	endpointToSpaceIDMap, _, err := readEndpointBindings(op.apr.st, applicationGlobalKey(op.apr.ApplicationName()))
	if errors.IsNotFound(err) {
		return appEndpoints, nil
	}
	if err != nil {
		return appEndpoints, errors.Trace(err)
	}
	for endpointName := range endpointToSpaceIDMap {
		if endpointName == "" {
			continue
		}
		appEndpoints.Add(endpointName)
	}
	return appEndpoints, nil
}

func (op *applicationPortRangesOperation) validatePendingChanges() error {
	endpointsNames, err := op.getEndpointBindings()
	if err != nil {
		return errors.Trace(err)
	}

	for endpointName := range op.apr.pendingOpenRanges {
		if len(endpointName) > 0 && !endpointsNames.Contains(endpointName) {
			return errors.NotFoundf("open port range: endpoint %q for application %q", endpointName, op.apr.ApplicationName())
		}
	}
	for endpointName := range op.apr.pendingCloseRanges {
		if len(endpointName) > 0 && !endpointsNames.Contains(endpointName) {
			return errors.NotFoundf("close port range: endpoint %q for application %q", endpointName, op.apr.ApplicationName())
		}
	}
	return nil
}

// Done implements ModelOperation.
func (op *applicationPortRangesOperation) Done(err error) error {
	if err != nil {
		return err
	}
	// Document has been persisted to state.
	op.apr.docExists = true
	op.apr.doc.PortRanges = op.updatedPortRanges

	op.apr.pendingOpenRanges = nil
	op.apr.pendingCloseRanges = nil
	return nil
}

func insertAppPortRangesDocOps(st *State, doc *applicationPortRangesDoc, asserts interface{}, portRanges network.GroupedPortRanges) []txn.Op {
	// As the following insert operation might be rolled back, we should
	// not mutate our internal doc but instead work on a copy of the
	// applicationPortRangesDoc.
	docCopy := new(applicationPortRangesDoc)
	*docCopy = *doc
	docCopy.PortRanges = portRanges

	return []txn.Op{
		{
			C:      openedPortsC,
			Id:     docCopy.DocID,
			Assert: asserts,
			Insert: docCopy,
		},
	}
}

func updateAppPortRangesDocOps(st *State, doc *applicationPortRangesDoc, asserts interface{}, portRanges network.GroupedPortRanges) []txn.Op {
	return []txn.Op{
		{
			C:      openedPortsC,
			Id:     doc.DocID,
			Assert: asserts,
			Update: bson.D{{Name: "$set", Value: bson.D{{Name: "port-ranges", Value: portRanges}}}},
		},
	}
}

// getOpenedApplicationPortRanges attempts to retrieve the set of opened ports for
// a particular embedded k8s application. If the underlying document does not exist, a blank
// applicationPortRanges instance with the docExists flag set to false will be
// returned instead.
func getOpenedApplicationPortRanges(st *State, appName string) (*applicationPortRanges, error) {
	openedPorts, closer := st.db().GetCollection(openedPortsC)
	defer closer()

	var doc applicationPortRangesDoc
	if err := openedPorts.FindId(applicationGlobalKey(appName)).One(&doc); err != nil {
		if err != mgo.ErrNotFound {
			return nil, errors.Annotatef(err, "cannot get opened port ranges for application %q", appName)
		}
		return &applicationPortRanges{
			st:        st,
			doc:       newApplicationPortRangesDoc(st.ModelUUID(), appName, nil),
			docExists: false,
		}, nil
	}

	return &applicationPortRanges{
		st:        st,
		doc:       doc,
		docExists: true,
	}, nil
}
