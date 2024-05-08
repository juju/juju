// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/testing"
)

// TODO (manadart 2020-04-03)
// The following refactoring should occur over time:
// - Move export_test.go contents to this file.
// - Rearrange packages (see state/testing) so that base suites can be
//   implemented here without import cycling.
// - Replace blanket exports with functions in suites here that supply
//   behaviour to parent suites that require them.

//go:generate go run go.uber.org/mock/mockgen -typed -package state -destination migration_import_mock_test.go github.com/juju/juju/state TransactionRunner,StateDocumentFactory,DocModelNamespace
//go:generate go run go.uber.org/mock/mockgen -typed -package state -destination migration_import_input_mock_test.go github.com/juju/juju/state RemoteEntitiesInput,RelationNetworksInput,RemoteApplicationsInput,ApplicationOfferStateDocumentFactory,ApplicationOfferInput,FirewallRulesInput,FirewallRulesOutput
//go:generate go run go.uber.org/mock/mockgen -typed -package state -destination migration_description_mock_test.go github.com/juju/description/v6 ApplicationOffer,FirewallRule,RemoteEntity,RelationNetwork,RemoteApplication,RemoteSpace,Status
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/operation_mock.go github.com/juju/juju/state ModelOperation
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/application_ports_mock.go github.com/juju/juju/state ApplicationPortRanges
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/upgrader_mock.go github.com/juju/juju/state Upgrader

func TestPackage(t *testing.T) {
	if !runStateTests {
		t.Skip("skipping state tests since the skip_state_tests build tag was set")
	}
	coretesting.MgoTestPackage(t)
}

// SetModelTypeToCAAS can be called after SetUpTest for state suites.
// It crudely just sets the model type to CAAS so that certain functionality
// relying on the model type can be tested.
func SetModelTypeToCAAS(c *gc.C, st *State, m *Model) {
	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.UUID(),
		Update: bson.D{{"$set", bson.D{{"type", ModelTypeCAAS}}}},
	}}

	RunTransaction(c, st, ops)
	c.Assert(m.refresh(m.UUID()), jc.ErrorIsNil)
}

// AddTestingApplicationWithEmptyBindings mimics an application
// from an old version of Juju, with no bindings entry.
func AddTestingApplicationWithEmptyBindings(c *gc.C, st *State, objectStore objectstore.ObjectStore, name string, ch *Charm) *Application {
	app := addTestingApplication(c, objectStore, addTestingApplicationParams{
		st:   st,
		name: name,
		ch:   ch,
	})

	RunTransaction(c, st, []txn.Op{removeEndpointBindingsOp(app.globalKey())})
	return app
}

// RunTransaction exposes the transaction running capability of State.
func RunTransaction(c *gc.C, st *State, ops []txn.Op) {
	c.Assert(st.db().RunTransaction(ops), jc.ErrorIsNil)
}

// MustOpenUnitPortRange ensures that the provided port range is opened
// for the specified unit and endpoint combination on the provided machine.
func MustOpenUnitPortRange(c *gc.C, st *State, machine *Machine, unitName, endpointName string, portRange network.PortRange) {
	MustOpenUnitPortRanges(c, st, machine, unitName, endpointName, []network.PortRange{portRange})
}

// MustOpenUnitPortRanges ensures that the provided port ranges are opened
// for the specified unit and endpoint combination on the provided machine.
func MustOpenUnitPortRanges(c *gc.C, st *State, machine *Machine, unitName, endpointName string, portRanges []network.PortRange) {
	machPortRanges, err := machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	unitPortRanges := machPortRanges.ForUnit(unitName)
	for _, pr := range portRanges {
		unitPortRanges.Open(endpointName, pr)
	}
	c.Assert(st.ApplyOperation(machPortRanges.Changes()), jc.ErrorIsNil)
}

// MustCloseUnitPortRange ensures that the provided port range is closed
// for the specified unit and endpoint combination on the provided machine.
func MustCloseUnitPortRange(c *gc.C, st *State, machine *Machine, unitName, endpointName string, portRange network.PortRange) {
	MustCloseUnitPortRanges(c, st, machine, unitName, endpointName, []network.PortRange{portRange})
}

// MustCloseUnitPortRanges ensures that the provided port ranges are closed
// for the specified unit and endpoint combination on the provided machine.
func MustCloseUnitPortRanges(c *gc.C, st *State, machine *Machine, unitName, endpointName string, portRanges []network.PortRange) {
	machPortRanges, err := machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	unitPortRanges := machPortRanges.ForUnit(unitName)
	for _, pr := range portRanges {
		unitPortRanges.Close(endpointName, pr)
	}
	c.Assert(st.ApplyOperation(machPortRanges.Changes()), jc.ErrorIsNil)
}

func (st *State) ReadBackendRefCount(backendID string) (int, error) {
	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()

	key := secretBackendRefCountKey(backendID)
	return nsRefcounts.read(refCountCollection, key)
}

func (st *State) IsSecretRevisionObsolete(c *gc.C, uri *secrets.URI, rev int) bool {
	col, closer := st.db().GetCollection(secretRevisionsC)
	defer closer()
	var doc secretRevisionDoc
	err := col.FindId(secretRevisionKey(uri, rev)).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	return doc.Obsolete
}
