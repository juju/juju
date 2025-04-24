// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/clock"
	jujutesting "github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	unit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	internalcharm "github.com/juju/juju/internal/charm"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/application/modelmigration ImportService,ExportService
//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination description_mock_test.go github.com/juju/description/v9 CharmMetadata,CharmMetadataRelation,CharmMetadataStorage,CharmMetadataDevice,CharmMetadataResource,CharmMetadataContainer,CharmMetadataContainerMount,CharmManifest,CharmManifestBase,CharmActions,CharmAction,CharmConfigs,CharmConfig

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type exportSuite struct {
	jujutesting.IsolationSuite

	exportService *MockExportService
}

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() exportOperation {
	return exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}
}

func (s *exportSuite) expectApplication(c *gc.C) {
	s.expectApplicationFor(c, "prometheus")
}

func (s *exportSuite) expectApplicationFor(c *gc.C, name string) {
	charmUUID := charmtesting.GenCharmID(c)

	s.exportService.EXPECT().GetApplications(gomock.Any()).Return([]application.ExportApplication{{
		Name:      name,
		ModelType: model.IAAS,
		CharmUUID: charmUUID,
		CharmLocator: charm.CharmLocator{
			Source:       charm.CharmHubSource,
			Name:         name,
			Revision:     42,
			Architecture: architecture.AMD64,
		},
	}}, nil)
	s.exportService.EXPECT().GetApplicationCharmOrigin(gomock.Any(), name).Return(application.CharmOrigin{
		Name:   name,
		Source: charm.CharmHubSource,
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04",
			Architecture: architecture.AMD64,
		},
	}, nil)
}

func (s *exportSuite) expectCharmOriginFor(name string) {
	s.exportService.EXPECT().GetApplicationCharmOrigin(gomock.Any(), name).Return(application.CharmOrigin{
		Name:   name,
		Source: charm.CharmHubSource,
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04",
			Architecture: architecture.AMD64,
		},
	}, nil)
}

func (s *exportSuite) expectMinimalCharm() {
	s.expectMinimalCharmFor("prometheus")
}

func (s *exportSuite) expectMinimalCharmFor(name string) {
	meta := &internalcharm.Meta{
		Name: name,
	}
	cfg := &internalcharm.Config{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type:    "string",
				Default: "baz",
			},
		},
	}
	ch := internalcharm.NewCharmBase(meta, nil, cfg, nil, nil)
	locator := charm.CharmLocator{
		Revision: 1,
	}
	s.exportService.EXPECT().GetCharmByApplicationName(gomock.Any(), name).Return(ch, locator, nil)
}

func (s *exportSuite) expectApplicationConfig() {
	s.expectApplicationConfigFor("prometheus")
}

func (s *exportSuite) expectApplicationConfigFor(name string) {
	config := config.ConfigAttributes{
		"foo": "bar",
	}
	settings := application.ApplicationSettings{
		Trust: true,
	}
	s.exportService.EXPECT().GetApplicationConfigAndSettings(gomock.Any(), name).Return(config, settings, nil)
}

func (s *exportSuite) expectGetApplicationScaleStateFor(name string, scaleState application.ScaleState) {
	exp := s.exportService.EXPECT()
	exp.GetApplicationScaleState(gomock.Any(), name).Return(scaleState, nil)
}

func (s *exportSuite) expectApplicationConstraints(cons constraints.Value) {
	s.expectApplicationConstraintsFor("prometheus", cons)
}

func (s *exportSuite) expectApplicationConstraintsFor(name string, cons constraints.Value) {
	s.exportService.EXPECT().GetApplicationConstraints(gomock.Any(), name).Return(cons, nil)
}

func (s *exportSuite) expectApplicationUnits() {
	s.expectApplicationUnitsFor("prometheus")
}

func (s *exportSuite) expectApplicationUnitsFor(name string) {
	s.exportService.EXPECT().GetApplicationUnits(gomock.Any(), name).Return([]application.ExportUnit{{
		Name: unit.Name(name + "/0"),
	}}, nil)
}
