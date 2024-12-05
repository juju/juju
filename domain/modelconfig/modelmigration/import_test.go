// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/environs/config"
)

type importSuite struct {
	coordinator           *MockCoordinator
	service               *MockImportService
	modelDefaultsProvider *MockModelDefaultsProvider
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.modelDefaultsProvider = NewMockModelDefaultsProvider(ctrl)

	return ctrl
}

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, s.modelDefaultsProvider)
}

func (s *importSuite) TestEmptyModelConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestModelConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config, err := config.New(config.NoDefaults, map[string]any{
		"name": "foo",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.service.EXPECT().SetModelConfig(gomock.Any(), config.AllAttrs()).Return(nil)

	model := description.NewModel(description.ModelArgs{
		Config: config.AllAttrs(),
	})

	op := s.newImportOperation()
	err = op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service:          s.service,
		defaultsProvider: s.modelDefaultsProvider,
	}
}
