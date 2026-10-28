// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

//go:generate go run github.com/canonical/gomock/mockgen -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/network/modelmigration K8sServiceMigrationService,LinkLayerDevicesMigrationService,SpaceImportService,SubnetImportService
