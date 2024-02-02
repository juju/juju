// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package migrationtarget_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/controller/migrationtarget ControllerConfigService,ExternalControllerService,UpgradeService,ModelImporter,ModelManagerService
//go:generate go run go.uber.org/mock/mockgen -package migrationtarget_test -destination credential_mock_test.go github.com/juju/juju/domain/credential/service CredentialValidator
//go:generate go run go.uber.org/mock/mockgen -package migrationtarget_test -destination servicefactory_mock_test.go github.com/juju/juju/internal/servicefactory ServiceFactoryGetter,ServiceFactory

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
