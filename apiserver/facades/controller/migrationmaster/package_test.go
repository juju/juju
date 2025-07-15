// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/backend.go github.com/juju/juju/apiserver/facades/controller/migrationmaster ModelExporter,UpgradeService,ControllerConfigService,ControllerNodeService,CredentialService,ModelInfoService,ModelService,ApplicationService,RelationService,StatusService,ModelAgentService,ModelMigrationService,MachineService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore.go github.com/juju/juju/core/objectstore ObjectStore
