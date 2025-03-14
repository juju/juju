// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package migrationtarget_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/controller/migrationtarget ControllerConfigService,ExternalControllerService,UpgradeService,ModelImporter,ModelMigrationService,ApplicationService,ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package migrationtarget_test -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServicesGetter,DomainServices
//go:generate go run go.uber.org/mock/mockgen -typed -package migrationtarget_test -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
