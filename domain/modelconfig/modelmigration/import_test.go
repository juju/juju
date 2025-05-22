// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator           *MockCoordinator
	service               *MockImportService
	modelDefaultsProvider *MockModelDefaultsProvider
}

func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.modelDefaultsProvider = NewMockModelDefaultsProvider(ctrl)

	return ctrl
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, s.modelDefaultsProvider, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestEmptyModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config, err := config.New(config.NoDefaults, map[string]any{
		"name":             "foo",
		"uuid":             "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":             "sometype",
		"workload-storage": "mystorage",
		"operator-storage": "otherstorage",
	})
	c.Assert(err, tc.ErrorIsNil)
	importedCOnfig := map[string]any{
		"logging-config":   "<root>=INFO",
		"workload-storage": "mystorage",
	}

	s.service.EXPECT().SetModelConfig(gomock.Any(), importedCOnfig).Return(nil)
	s.modelDefaultsProvider.EXPECT().ModelDefaults(gomock.Any()).Return(modeldefaults.Defaults{
		"logging-config":   modeldefaults.DefaultAttributeValue{},
		"workload-storage": modeldefaults.DefaultAttributeValue{},
	}, nil)

	model := description.NewModel(description.ModelArgs{
		Config: config.AllAttrs(),
	})

	op := s.newImportOperation(c)
	err = op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		service:          s.service,
		defaultsProvider: s.modelDefaultsProvider,
		logger:           loggertesting.WrapCheckLog(c),
	}
}
