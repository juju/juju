// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"errors"

	"github.com/juju/description/v8"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator)
}

func (s *importSuite) TestEmptyExternalControllers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	// No import executed.
	s.service.EXPECT().ImportExternalControllers(gomock.All(), gomock.Any()).Times(0)
}

func (s *importSuite) TestExecuteMultipleExternalControllers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.AddExternalController(
		description.ExternalControllerArgs{
			Tag:    names.NewControllerTag("ctrl1"),
			Addrs:  []string{"192.168.1.1:8080"},
			Alias:  "external ctrl1",
			CACert: "ca-cert-1",
			Models: []string{"model1", "model2"},
		},
	)
	model.AddExternalController(
		description.ExternalControllerArgs{
			Tag:    names.NewControllerTag("ctrl2"),
			Addrs:  []string{"192.168.1.1:8080"},
			Alias:  "external ctrl2",
			CACert: "ca-cert-2",
			Models: []string{"model3", "model4"},
		},
	)

	expectedCtrls := []crossmodel.ControllerInfo{
		{
			ControllerTag: names.NewControllerTag("ctrl1"),
			Addrs:         []string{"192.168.1.1:8080"},
			Alias:         "external ctrl1",
			CACert:        "ca-cert-1",
			ModelUUIDs:    []string{"model1", "model2"},
		},
		{
			ControllerTag: names.NewControllerTag("ctrl2"),
			Addrs:         []string{"192.168.1.1:8080"},
			Alias:         "external ctrl2",
			CACert:        "ca-cert-2",
			ModelUUIDs:    []string{"model3", "model4"},
		},
	}
	s.service.EXPECT().ImportExternalControllers(gomock.All(), expectedCtrls).Times(1)

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestExecuteReturnsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Model with 2 external controllers.
	model := description.NewModel(description.ModelArgs{})
	model.AddExternalController(
		description.ExternalControllerArgs{
			Tag:    names.NewControllerTag("ctrl1"),
			Addrs:  []string{"192.168.1.1:8080"},
			Alias:  "external ctrl1",
			CACert: "ca-cert-1",
			Models: []string{"model1", "model2"},
		},
	)

	expectedCtrls := []crossmodel.ControllerInfo{
		{
			ControllerTag: names.NewControllerTag("ctrl1"),
			Addrs:         []string{"192.168.1.1:8080"},
			Alias:         "external ctrl1",
			CACert:        "ca-cert-1",
			ModelUUIDs:    []string{"model1", "model2"},
		},
	}
	s.service.EXPECT().ImportExternalControllers(gomock.All(), expectedCtrls).
		Times(1).
		Return(errors.New("fail on test"))

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, "cannot import external controllers: fail on test")
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service: s.service,
	}
}
