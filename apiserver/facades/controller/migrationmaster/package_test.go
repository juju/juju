// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/backend.go github.com/juju/juju/apiserver/facades/controller/migrationmaster Backend,ControllerState,ModelExporter,UpgradeService,ControllerConfigService,CredentialService,ModelInfoService,ModelService,ApplicationService,RelationService,StatusService,ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/precheckbackend.go github.com/juju/juju/internal/migration PrecheckBackend
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state.go github.com/juju/juju/state ModelMigration,NotifyWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore.go github.com/juju/juju/core/objectstore ObjectStore

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
