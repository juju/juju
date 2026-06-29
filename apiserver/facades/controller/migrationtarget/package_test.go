// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

//go:generate go run github.com/canonical/gomock/mockgen -package migrationtarget_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/controller/migrationtarget CloudService,ControllerConfigService,ExternalControllerService,UpgradeService,ModelImporter,ModelMigrationService,ModelAgentService,StatusService,MachineService,ModelService,RemovalService,MigrationImportService
//go:generate go run github.com/canonical/gomock/mockgen -package migrationtarget_test -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServicesGetter,DomainServices
//go:generate go run github.com/canonical/gomock/mockgen -package migrationtarget_test -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter
