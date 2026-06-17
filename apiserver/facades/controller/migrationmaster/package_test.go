// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/backend.go github.com/juju/juju/apiserver/facades/controller/migrationmaster UpgradeService,ControllerConfigService,CredentialService,ModelInfoService,ModelService,ApplicationService,RelationService,StatusService,ModelAgentService,ModelMigrationService,MachineService
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/objectstore.go github.com/juju/juju/core/objectstore ObjectStore
