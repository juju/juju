// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

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
