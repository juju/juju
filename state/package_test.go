// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate mockgen -package state -destination migration_import_mock_test.go github.com/juju/juju/state TransactionRunner,StateDocumentFactory,DocModelNamespace,RemoteEntitiesDescription,RelationNetworksDescription,RemoteApplicationsDescription,ApplicationOfferStateDocumentFactory,ApplicationOfferDescription,ExternalControllersDescription
//go:generate mockgen -package state -destination migration_description_mock_test.go github.com/juju/description ExternalController,ApplicationOffer,RemoteEntity,RelationNetwork,RemoteApplication,RemoteSpace,Status

func TestPackage(t *testing.T) {
	if !runStateTests {
		t.Skip("skipping state tests since the skip_state_tests build tag was set")
	}
	coretesting.MgoTestPackage(t)
}
