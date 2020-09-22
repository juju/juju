// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
)

// applicationPortRangesDoc represents the state of ports opened for an application.
type applicationPortRangesDoc struct {
	DocID           string                    `bson:"_id"`
	ModelUUID       string                    `bson:"model-uuid"`
	ApplicationName string                    `bson:"application-name"`
	PortRanges      network.GroupedPortRanges `bson:"port-ranges"`
}

func newApplicationPortRangesDoc(modelUUID, appName string, pgs network.GroupedPortRanges) applicationPortRangesDoc {
	return applicationPortRangesDoc{
		DocID:           applicationGlobalKey(appName),
		ModelUUID:       modelUUID,
		ApplicationName: appName,
		PortRanges:      pgs,
	}
}

// applicationPortRanges is a view on the machinePortRanges type that provides
// application-level information about the set of opened port ranges for various
// application endpoints.
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
	return &applicationPortRangesOperation{
		pr: p,
	}
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

type applicationPortRangesOperation struct {
	pr *applicationPortRanges

	updatedPortRanges network.GroupedPortRanges
}

func (op *applicationPortRangesOperation) Build(attempt int) ([]txn.Op, error) {
	// TODO: openClosePortRangesOperation.Build
	return nil, nil
}

// Done implements ModelOperation.
func (op *applicationPortRangesOperation) Done(err error) error {
	if err != nil {
		return err
	}
	// TODO: openClosePortRangesOperation.Done
	return nil
}
