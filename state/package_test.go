// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	coretesting "github.com/juju/juju/testing"
)

// TODO (manadart 2020-04-03)
// The following refactoring should occur over time:
// - Move export_test.go contents to this file.
// - Rearrange packages (see state/testing) so that base suites can be
//   implemented here without import cycling.
// - Replace blanket exports with functions in suites here that supply
//   behaviour to parent suites that require them.

//go:generate mockgen -package state -destination migration_import_mock_test.go github.com/juju/juju/state TransactionRunner,StateDocumentFactory,DocModelNamespace
//go:generate mockgen -package state -destination migration_import_input_mock_test.go github.com/juju/juju/state RemoteEntitiesInput,RelationNetworksInput,RemoteApplicationsInput,ApplicationOfferStateDocumentFactory,ApplicationOfferInput,ExternalControllerStateDocumentFactory,ExternalControllersInput,FirewallRulesInput
//go:generate mockgen -package state -destination migration_description_mock_test.go github.com/juju/description ApplicationOffer,ExternalController,FirewallRule,RemoteEntity,RelationNetwork,RemoteApplication,RemoteSpace,Status
//go:generate mockgen -package mocks -destination mocks/operation_mock.go github.com/juju/juju/state ModelOperation

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

	c.Assert(st.db().RunTransaction(ops), jc.ErrorIsNil)
	c.Assert(m.refresh(m.UUID()), jc.ErrorIsNil)
}

// AddTestingApplicationWithEmptyBindings mimics an application
// from an old version of Juju, with no bindings entry.
func AddTestingApplicationWithEmptyBindings(c *gc.C, st *State, name string, ch *Charm) *Application {
	app := addTestingApplication(c, addTestingApplicationParams{
		st:   st,
		name: name,
		ch:   ch,
	})

	c.Assert(st.db().RunTransaction([]txn.Op{removeEndpointBindingsOp(app.globalKey())}), jc.ErrorIsNil)
	return app
}
