// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/environs/config"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

func TestExportSuite(t *stdtesting.T) { tc.Run(t, &exportSuite{}) }
func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() *exportOperation {
	return &exportOperation{
		service: s.service,
	}
}

func (s *exportSuite) TestRegisterExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterExport(s.coordinator)
}

func (s *exportSuite) TestNilModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().ModelConfig(gomock.Any()).Return(nil, nil)

	model := description.NewModel(description.ModelArgs{})

	op := s.newExportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *exportSuite) TestEmptyModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := &config.Config{}

	s.service.EXPECT().ModelConfig(gomock.Any()).Return(config, nil)

	model := description.NewModel(description.ModelArgs{})

	op := s.newExportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *exportSuite) TestModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config, err := config.New(config.NoDefaults, map[string]any{
		"name": "foo",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	})
	c.Assert(err, tc.ErrorIsNil)

	s.service.EXPECT().ModelConfig(gomock.Any()).Return(config, nil)

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{},
	})

	op := s.newExportOperation()
	err = op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(model.Config(), tc.DeepEquals, config.AllAttrs())
}
