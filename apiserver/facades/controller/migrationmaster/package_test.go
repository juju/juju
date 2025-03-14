// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/backend.go github.com/juju/juju/apiserver/facades/controller/migrationmaster Backend,ControllerState,ModelExporter,UpgradeService,ControllerConfigService,CredentialService,ModelConfigService,ModelInfoService,ModelService,ApplicationService,StatusService,ModelAgentService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/precheckbackend.go github.com/juju/juju/internal/migration PrecheckBackend
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state.go github.com/juju/juju/state ModelMigration,NotifyWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore.go github.com/juju/juju/core/objectstore ObjectStore

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
