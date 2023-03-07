// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/network"
)

// applicationPortRangesDoc represents the state of ports opened for an application.
type applicationPortRangesDoc struct {
	DocID           string `bson:"_id"`
	ModelUUID       string `bson:"model-uuid"`
	ApplicationName string `bson:"application-name"`

	// PortRanges is the application port ranges that are open for all units.
	PortRanges network.GroupedPortRanges            `bson:"port-ranges"`
	UnitRanges map[string]network.GroupedPortRanges `bson:"unit-port-ranges"`
	TxnRevno   int64                                `bson:"txn-revno"`
}

func newApplicationPortRangesDoc(docID, modelUUID, appName string) applicationPortRangesDoc {
	return applicationPortRangesDoc{
		DocID:           docID,
		ModelUUID:       modelUUID,
		ApplicationName: appName,
		UnitRanges:      make(map[string]network.GroupedPortRanges),
	}
}

var _ ApplicationPortRanges = (*applicationPortRanges)(nil)

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

// Changes returns a ModelOperation for applying any changes that were made to
// this port range instance.
func (p *applicationPortRanges) Changes() ModelOperation {
	// The application scope opened port range is not implemented yet.
	// We manage(open/close) ports by units using "unitPortRanges.Open|Close|Changes()".
	return nil
}

// Persisted returns true if the underlying document for this instance exists
// in the database.
func (p *applicationPortRanges) Persisted() bool {
	return p.docExists
}

// ForUnit returns the set of port ranges opened by the specified unit.
func (p *applicationPortRanges) ForUnit(unitName string) UnitPortRanges {
	return &unitPortRanges{
		unitName: unitName,
		apg:      p,
	}
}

// ByUnit returns the set of port ranges opened by each unit grouped by unit name.
func (p *applicationPortRanges) ByUnit() map[string]UnitPortRanges {
	if len(p.doc.UnitRanges) == 0 {
		return nil
	}
	res := make(map[string]UnitPortRanges)
	for unitName := range p.doc.UnitRanges {
		res[unitName] = newUnitPortRanges(unitName, p)
	}
	return res
}

// ByEndpoint returns the list of open port ranges grouped by endpoint.
func (p *applicationPortRanges) ByEndpoint() network.GroupedPortRanges {
	out := make(network.GroupedPortRanges)
	for _, gpg := range p.doc.UnitRanges {
		for endpoint, prs := range gpg {
			out[endpoint] = append(out[endpoint], prs...)
		}
	}
	return out
}

// UniquePortRanges returns a slice of unique open PortRanges all units.
func (p *applicationPortRanges) UniquePortRanges() []network.PortRange {
	allRanges := make(network.GroupedPortRanges)
	for _, unitRanges := range p.ByUnit() {
		allRanges[""] = append(allRanges[""], unitRanges.UniquePortRanges()...)
	}
	return allRanges.UniquePortRanges()
}

func (p *applicationPortRanges) clearPendingRecords() {
	p.pendingOpenRanges = make(network.GroupedPortRanges)
	p.pendingCloseRanges = make(network.GroupedPortRanges)
}

// ApplicationName returns the application name associated with this set of port ranges.
func (p *applicationPortRanges) ApplicationName() string {
	return p.doc.ApplicationName
}

func (p *applicationPortRanges) Remove() error {
	doc := &applicationPortRanges{st: p.st, doc: p.doc, docExists: p.docExists}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := doc.Refresh()
			if errors.Is(err, errors.NotFound) {
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
		p.docExists = false
		return errors.NotFoundf("open port ranges for application %q", p.ApplicationName())
	}
	if err != nil {
		return errors.Annotatef(err, "refresh open port ranges for application %q", p.ApplicationName())
	}
	p.docExists = true
	return nil
}

func (p *applicationPortRanges) removeOps() []txn.Op {
	if !p.docExists {
		return nil
	}
	return []txn.Op{{
		C:      openedPortsC,
		Id:     p.doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}}
}

var _ UnitPortRanges = (*unitPortRanges)(nil)

type unitPortRanges struct {
	unitName string
	apg      *applicationPortRanges
}

func newUnitPortRanges(unitName string, apg *applicationPortRanges) UnitPortRanges {
	return &unitPortRanges{
		unitName: unitName,
		apg:      apg,
	}
}

// UnitName returns the unit name associated with this set of ports.
func (p *unitPortRanges) UnitName() string {
	return p.unitName
}

// Open records a request for opening a particular port range for the specified
// endpoint.
func (p *unitPortRanges) Open(endpoint string, portRange network.PortRange) {
	if p.apg.pendingOpenRanges == nil {
		p.apg.pendingOpenRanges = make(network.GroupedPortRanges)
	}

	p.apg.pendingOpenRanges[endpoint] = append(p.apg.pendingOpenRanges[endpoint], portRange)
}

// Close records a request for closing a particular port range for the
// specified endpoint.
func (p *unitPortRanges) Close(endpoint string, portRange network.PortRange) {
	if p.apg.pendingCloseRanges == nil {
		p.apg.pendingCloseRanges = make(network.GroupedPortRanges)
	}

	p.apg.pendingCloseRanges[endpoint] = append(p.apg.pendingCloseRanges[endpoint], portRange)
}

// Changes returns a ModelOperation for applying any changes that were made to
// this port range instance.
func (p *unitPortRanges) Changes() ModelOperation {
	return newApplicationPortRangesOperation(p.apg, p.unitName)
}

// UniquePortRanges returns a slice of unique open PortRanges for the unit.
func (p *unitPortRanges) UniquePortRanges() []network.PortRange {
	allRanges := p.apg.doc.UnitRanges[p.unitName].UniquePortRanges()
	network.SortPortRanges(allRanges)
	return allRanges
}

// ByEndpoint returns the list of open port ranges grouped by endpoint.
func (p *unitPortRanges) ByEndpoint() network.GroupedPortRanges {
	return p.apg.doc.UnitRanges[p.unitName]
}

// ForEndpoint returns a list of port ranges that the unit has opened for the
// specified endpoint.
func (p *unitPortRanges) ForEndpoint(endpointName string) []network.PortRange {
	unitPortRange := p.apg.doc.UnitRanges[p.unitName]
	if len(unitPortRange) == 0 || len(unitPortRange[endpointName]) == 0 {
		return nil
	}
	res := append([]network.PortRange(nil), unitPortRange[endpointName]...)
	network.SortPortRanges(res)
	return res
}

var _ ModelOperation = (*applicationPortRangesOperation)(nil)

type applicationPortRangesOperation struct {
	unitName              string
	apr                   *applicationPortRanges
	updatedUnitPortRanges map[string]network.GroupedPortRanges
}

func newApplicationPortRangesOperation(apr *applicationPortRanges, unitName string) ModelOperation {
	op := &applicationPortRangesOperation{apr: apr, unitName: unitName}
	op.cloneExistingUnitPortRanges()
	return op
}

func (op *applicationPortRangesOperation) cloneExistingUnitPortRanges() {
	op.updatedUnitPortRanges = make(map[string]network.GroupedPortRanges)
	for unitName, existingDoc := range op.apr.doc.UnitRanges {
		newDoc := make(network.GroupedPortRanges)
		for endpointName, portRanges := range existingDoc {
			newDoc[endpointName] = append([]network.PortRange(nil), portRanges...)
		}
		op.updatedUnitPortRanges[unitName] = newDoc
	}
}

func (op *applicationPortRangesOperation) Build(attempt int) ([]txn.Op, error) {
	defer op.apr.clearPendingRecords()

	if op.unitName == "" {
		return nil, errors.NotValidf("empty unit name")
	}

	if err := checkModelNotDead(op.apr.st); err != nil {
		return nil, errors.Annotate(err, "cannot open/close ports")
	}

	var createDoc = !op.apr.docExists
	if attempt > 0 {
		if err := op.apr.Refresh(); err != nil {
			if !errors.Is(err, errors.NotFound) {
				return nil, errors.Annotate(err, "cannot open/close ports")
			}

			// Doc not found; we need to create it.
			createDoc = true
		}
	}

	op.cloneExistingUnitPortRanges()

	if err := op.validatePendingChanges(); err != nil {
		return nil, errors.Annotate(err, "cannot open/close ports")
	}

	ops := []txn.Op{
		assertModelNotDeadOp(op.apr.st.ModelUUID()),
		assertApplicationAliveOp(op.apr.st.docID(op.apr.ApplicationName())),
		assertUnitNotDeadOp(op.apr.st, op.unitName),
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

	if !portListModified || (createDoc && len(op.updatedUnitPortRanges) == 0) {
		return nil, jujutxn.ErrNoOperations
	}

	if createDoc {
		assert := txn.DocMissing
		ops = append(ops, insertAppPortRangesDocOps(op.apr.st, &op.apr.doc, assert, op.updatedUnitPortRanges)...)
	} else if len(op.updatedUnitPortRanges) == 0 {
		// Port list is empty; get rid of ports document.
		ops = append(ops, op.apr.removeOps()...)
	} else {
		assert := bson.D{
			{Name: "txn-revno", Value: op.apr.doc.TxnRevno},
		}
		ops = append(ops, updateAppPortRangesDocOps(op.apr.st, &op.apr.doc, assert, op.updatedUnitPortRanges)...)
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
			op.addPortRanges(endpointName, true, pendingRange)
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
			modified = op.removePortRange(endpointName, pendingRange)
		}
	}
	return modified, nil
}

func (op *applicationPortRangesOperation) addPortRanges(endpointName string, merge bool, portRanges ...network.PortRange) {
	if op.updatedUnitPortRanges[op.unitName] == nil {
		op.updatedUnitPortRanges[op.unitName] = make(network.GroupedPortRanges)
	}
	if !merge {
		op.updatedUnitPortRanges[op.unitName][endpointName] = portRanges
		return
	}
	op.updatedUnitPortRanges[op.unitName][endpointName] = append(op.updatedUnitPortRanges[op.unitName][endpointName], portRanges...)
}

func (op *applicationPortRangesOperation) removePortRange(endpointName string, portRange network.PortRange) bool {
	if op.updatedUnitPortRanges[op.unitName] == nil || op.updatedUnitPortRanges[op.unitName][endpointName] == nil {
		return false
	}
	var modified bool
	existingRanges := op.updatedUnitPortRanges[op.unitName][endpointName]
	for i, v := range existingRanges {
		if v != portRange {
			continue
		}
		existingRanges = append(existingRanges[:i], existingRanges[i+1:]...)
		if len(existingRanges) == 0 {
			delete(op.updatedUnitPortRanges[op.unitName], endpointName)
		} else {
			op.addPortRanges(endpointName, false, existingRanges...)
		}
		modified = true
	}
	return modified
}

func (op *applicationPortRangesOperation) rangeExistsForEndpoint(endpointName string, portRange network.PortRange) bool {
	// For k8s applications, no endpoint level portrange supported currently.
	// There is only one endpoint(which is empty string - "").
	if len(op.updatedUnitPortRanges[op.unitName][endpointName]) == 0 {
		return false
	}

	for _, existingRange := range op.updatedUnitPortRanges[op.unitName][endpointName] {
		if existingRange == portRange {
			return true
		}
	}
	return false
}

func (op *applicationPortRangesOperation) getEndpointBindings() (set.Strings, error) {
	appEndpoints := set.NewStrings()
	endpointToSpaceIDMap, _, err := readEndpointBindings(op.apr.st, applicationGlobalKey(op.apr.ApplicationName()))
	if errors.Is(err, errors.NotFound) {
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
	op.apr.doc.UnitRanges = op.updatedUnitPortRanges

	op.apr.pendingOpenRanges = nil
	op.apr.pendingCloseRanges = nil
	return nil
}

func insertAppPortRangesDocOps(
	st *State, doc *applicationPortRangesDoc, asserts interface{}, unitRanges map[string]network.GroupedPortRanges,
) (o []txn.Op) {
	// As the following insert operation might be rolled back, we should
	// not mutate our internal doc but instead work on a copy of the
	// applicationPortRangesDoc.
	docCopy := new(applicationPortRangesDoc)
	*docCopy = *doc
	docCopy.UnitRanges = unitRanges
	return []txn.Op{
		{
			C:      openedPortsC,
			Id:     docCopy.DocID,
			Assert: asserts,
			Insert: docCopy,
		},
	}
}

func updateAppPortRangesDocOps(
	st *State, doc *applicationPortRangesDoc, asserts interface{}, unitRanges map[string]network.GroupedPortRanges,
) (o []txn.Op) {
	return []txn.Op{
		{
			C:      openedPortsC,
			Id:     doc.DocID,
			Assert: asserts,
			Update: bson.D{{Name: "$set", Value: bson.D{{Name: "unit-port-ranges", Value: unitRanges}}}},
		},
	}
}

func removeApplicationPortsForUnitOps(st *State, unit *Unit) ([]txn.Op, error) {
	unitName := unit.Name()
	appPortRanges, err := getApplicationPortRanges(st, unit.ApplicationName())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !appPortRanges.docExists {
		return nil, nil
	}
	if appPortRanges.doc.UnitRanges == nil || appPortRanges.doc.UnitRanges[unitName] == nil {
		// No entry for the unit; nothing to do here.
		return nil, nil
	}
	// Drop unit rules and write the doc back if non-empty or remove it if empty.
	delete(appPortRanges.doc.UnitRanges, unitName)
	if len(appPortRanges.doc.UnitRanges) != 0 {
		assert := bson.D{{"txn-revno", appPortRanges.doc.TxnRevno}}
		return updateAppPortRangesDocOps(st, &appPortRanges.doc, assert, appPortRanges.doc.UnitRanges), nil
	}
	return appPortRanges.removeOps(), nil
}

// getApplicationPortRanges attempts to retrieve the set of opened ports for
// a particular embedded k8s application. If the underlying document does not exist, a blank
// applicationPortRanges instance with the docExists flag set to false will be
// returned instead.
func getApplicationPortRanges(st *State, appName string) (*applicationPortRanges, error) {
	openedPorts, closer := st.db().GetCollection(openedPortsC)
	defer closer()

	docID := st.docID(applicationGlobalKey(appName))
	var doc applicationPortRangesDoc
	if err := openedPorts.FindId(docID).One(&doc); err != nil {
		if err != mgo.ErrNotFound {
			return nil, errors.Annotatef(err, "cannot get opened port ranges for application %q", appName)
		}
		return &applicationPortRanges{
			st:        st,
			doc:       newApplicationPortRangesDoc(docID, st.ModelUUID(), appName),
			docExists: false,
		}, nil
	}

	return &applicationPortRanges{
		st:        st,
		doc:       doc,
		docExists: true,
	}, nil
}

// getOpenedApplicationPortRanges attempts to retrieve the set of opened ports for
// a particular embedded k8s unit. If the underlying document does not exist, a blank
// unitPortRanges instance with the docExists flag set to false will be
// returned instead.
func getUnitPortRanges(st *State, appName, unitName string) (*unitPortRanges, error) {
	apg, err := getApplicationPortRanges(st, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &unitPortRanges{unitName: unitName, apg: apg}, nil
}

// OpenedPortRangesForAllApplications returns a slice of opened port ranges for all
// applications managed by this model.
func (m *Model) OpenedPortRangesForAllApplications() ([]ApplicationPortRanges, error) {
	mprResults, err := getOpenedApplicationPortRangesForAllApplications(m.st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return transform.Slice(mprResults, func(agr *applicationPortRanges) ApplicationPortRanges {
		return agr
	}), nil
}

// getOpenedApplicationPortRangesForAllApplications is used for migration export.
func getOpenedApplicationPortRangesForAllApplications(st *State) ([]*applicationPortRanges, error) {
	apps, err := st.AllApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var appNames []string
	for _, app := range apps {
		appNames = append(appNames, app.Name())
	}
	openedPorts, closer := st.db().GetCollection(openedPortsC)
	defer closer()
	docs := []applicationPortRangesDoc{}
	err = openedPorts.Find(bson.D{{"application-name", bson.D{{"$in", appNames}}}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]*applicationPortRanges, len(docs))
	for i, doc := range docs {
		results[i] = &applicationPortRanges{
			st:        st,
			doc:       doc,
			docExists: true,
		}
	}
	return results, nil
}
