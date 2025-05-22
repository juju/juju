// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/internal/errors"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator)
}

func (s *importSuite) TestEmptyExternalControllers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
	// No import executed.
	s.service.EXPECT().ImportExternalControllers(gomock.All(), gomock.Any()).Times(0)
}

func (s *importSuite) TestExecuteMultipleExternalControllers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.AddExternalController(
		description.ExternalControllerArgs{
			ID:     "ctrl1",
			Addrs:  []string{"192.168.1.1:8080"},
			Alias:  "external ctrl1",
			CACert: "ca-cert-1",
			Models: []string{"model1", "model2"},
		},
	)
	model.AddExternalController(
		description.ExternalControllerArgs{
			ID:     "ctrl2",
			Addrs:  []string{"192.168.1.1:8080"},
			Alias:  "external ctrl2",
			CACert: "ca-cert-2",
			Models: []string{"model3", "model4"},
		},
	)

	expectedCtrls := []crossmodel.ControllerInfo{
		{
			ControllerUUID: "ctrl1",
			Addrs:          []string{"192.168.1.1:8080"},
			Alias:          "external ctrl1",
			CACert:         "ca-cert-1",
			ModelUUIDs:     []string{"model1", "model2"},
		},
		{
			ControllerUUID: "ctrl2",
			Addrs:          []string{"192.168.1.1:8080"},
			Alias:          "external ctrl2",
			CACert:         "ca-cert-2",
			ModelUUIDs:     []string{"model3", "model4"},
		},
	}
	s.service.EXPECT().ImportExternalControllers(gomock.All(), expectedCtrls).Times(1)

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestExecuteReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.AddExternalController(
		description.ExternalControllerArgs{
			ID:     "ctrl1",
			Addrs:  []string{"192.168.1.1:8080"},
			Alias:  "external ctrl1",
			CACert: "ca-cert-1",
			Models: []string{"model1", "model2"},
		},
	)

	expectedCtrls := []crossmodel.ControllerInfo{
		{
			ControllerUUID: "ctrl1",
			Addrs:          []string{"192.168.1.1:8080"},
			Alias:          "external ctrl1",
			CACert:         "ca-cert-1",
			ModelUUIDs:     []string{"model1", "model2"},
		},
	}
	s.service.EXPECT().ImportExternalControllers(gomock.All(), expectedCtrls).
		Times(1).
		Return(errors.New("fail on test"))

	op := s.newImportOperation()
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, "cannot import external controllers: fail on test")
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service: s.service,
	}
}
