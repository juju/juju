// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/application/modelmigration ImportService,ExportService
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination description_mock_test.go github.com/juju/description/v9 CharmMetadata,CharmMetadataRelation,CharmMetadataStorage,CharmMetadataDevice,CharmMetadataResource,CharmMetadataContainer,CharmMetadataContainerMount,CharmManifest,CharmManifestBase,CharmActions,CharmAction,CharmConfigs,CharmConfig

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
