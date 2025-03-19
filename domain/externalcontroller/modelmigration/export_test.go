// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
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

func (s *exportSuite) TestExportExternalController(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := "model-uuid-1"
	dst := description.NewModel(description.ModelArgs{})
	dst.AddRemoteApplication(description.RemoteApplicationArgs{
		SourceModelUUID: modelUUID,
	})
	ctrlUUID := "ctrl-uuid-1"
	extCtrlModel := []crossmodel.ControllerInfo{
		{
			ControllerUUID: ctrlUUID,
			Addrs:          []string{"192.168.1.1:8080"},
			Alias:          "external ctrl1",
			CACert:         "ca-cert-1",
			ModelUUIDs:     []string{"model1", "model2"},
		},
	}
	s.service.EXPECT().ControllersForModels(gomock.Any(), []string{modelUUID}).
		Times(1).
		Return(extCtrlModel, nil)

	// Assert that the destination description model has no external
	// controllers before the migration:
	c.Assert(dst.ExternalControllers(), gc.HasLen, 0)
	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)
	// Assert that the destination description model has one external
	// controller after the migration:
	c.Check(dst.ExternalControllers(), gc.HasLen, 1)
	c.Assert(dst.ExternalControllers()[0].ID(), gc.Equals, ctrlUUID)
	c.Assert(dst.ExternalControllers()[0].Addrs(), jc.SameContents, []string{"192.168.1.1:8080"})
	c.Assert(dst.ExternalControllers()[0].Alias(), gc.Equals, "external ctrl1")
	c.Assert(dst.ExternalControllers()[0].CACert(), gc.Equals, "ca-cert-1")
	c.Assert(dst.ExternalControllers()[0].Models(), jc.SameContents, []string{"model1", "model2"})
}

func (s *exportSuite) TestExportExternalControllerRequestsExternalControllerOnceWithSameUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := "model-uuid-1"
	dst := description.NewModel(description.ModelArgs{})
	// We add two remote applications with the same source model:
	dst.AddRemoteApplication(description.RemoteApplicationArgs{
		SourceModelUUID: modelUUID,
	})
	dst.AddRemoteApplication(description.RemoteApplicationArgs{
		SourceModelUUID: modelUUID,
	})
	ctrlUUID := "ctrl-uuid-1"
	extCtrlModel := []crossmodel.ControllerInfo{
		{
			ControllerUUID: ctrlUUID,
			Addrs:          []string{"192.168.1.1:8080"},
			Alias:          "external ctrl1",
			CACert:         "ca-cert-1",
			ModelUUIDs:     []string{"model1", "model2"},
		},
	}
	// But only once controller should be returned since the model is
	// the same for both remote applications.
	s.service.EXPECT().ControllersForModels(gomock.Any(), []string{modelUUID, modelUUID}).
		Times(1).
		Return(extCtrlModel, nil)

	// Assert that the destination description model has no external
	// controllers before the migration:
	c.Assert(dst.ExternalControllers(), gc.HasLen, 0)
	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)
	// Assert that the destination description model has one external
	// controller after the migration:
	c.Assert(dst.ExternalControllers(), gc.HasLen, 1)
	c.Assert(dst.ExternalControllers()[0].ID(), gc.Equals, ctrlUUID)
	c.Assert(dst.ExternalControllers()[0].Addrs(), jc.SameContents, []string{"192.168.1.1:8080"})
	c.Assert(dst.ExternalControllers()[0].Alias(), gc.Equals, "external ctrl1")
	c.Assert(dst.ExternalControllers()[0].CACert(), gc.Equals, "ca-cert-1")
	c.Assert(dst.ExternalControllers()[0].Models(), jc.SameContents, []string{"model1", "model2"})
}

func (s *exportSuite) TestExportExternalControllerRequestsExternalControllerOnceWithSameController(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID1 := "model-uuid-1"
	modelUUID2 := "model-uuid-2"
	dst := description.NewModel(description.ModelArgs{})
	// We add two remote applications with the same source model:
	dst.AddRemoteApplication(description.RemoteApplicationArgs{
		SourceModelUUID: modelUUID1,
	})
	dst.AddRemoteApplication(description.RemoteApplicationArgs{
		SourceModelUUID: modelUUID2,
	})
	ctrlUUID := "ctrl-uuid-1"
	extCtrlModel := []crossmodel.ControllerInfo{
		{
			ControllerUUID: ctrlUUID,
			Addrs:          []string{"192.168.1.1:8080"},
			Alias:          "external ctrl1",
			CACert:         "ca-cert-1",
			ModelUUIDs:     []string{"model1", "model2"},
		},
	}
	// But only once controller should be returned since the model is
	// the same for both remote applications.
	s.service.EXPECT().ControllersForModels(gomock.Any(), []string{modelUUID1, modelUUID2}).
		Times(1).
		Return(extCtrlModel, nil)

	// Assert that the destination description model has no external
	// controllers before the migration:
	c.Assert(dst.ExternalControllers(), gc.HasLen, 0)
	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)
	// Assert that the destination description model has one external
	// controller after the migration:
	c.Assert(dst.ExternalControllers(), gc.HasLen, 1)
	c.Assert(dst.ExternalControllers()[0].ID(), gc.Equals, ctrlUUID)
	c.Assert(dst.ExternalControllers()[0].Addrs(), jc.SameContents, []string{"192.168.1.1:8080"})
	c.Assert(dst.ExternalControllers()[0].Alias(), gc.Equals, "external ctrl1")
	c.Assert(dst.ExternalControllers()[0].CACert(), gc.Equals, "ca-cert-1")
	c.Assert(dst.ExternalControllers()[0].Models(), jc.SameContents, []string{"model1", "model2"})
}

func (s *exportSuite) TestExportExternalControllerWithNoControllerNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := "model-uuid-1"
	dst := description.NewModel(description.ModelArgs{})
	dst.AddRemoteApplication(description.RemoteApplicationArgs{
		SourceModelUUID: modelUUID,
	})

	s.service.EXPECT().ControllersForModels(gomock.Any(), []string{modelUUID}).
		Times(1).
		Return(nil, errors.Errorf("test-external-controller %w", coreerrors.NotFound))

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.ErrorMatches, "test-external-controller not found")
}

func (s *exportSuite) TestExportExternalControllerFailsGettingExternalControllerEntities(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := "model-uuid-1"
	dst := description.NewModel(description.ModelArgs{})
	dst.AddRemoteApplication(description.RemoteApplicationArgs{
		SourceModelUUID: modelUUID,
	})

	s.service.EXPECT().ControllersForModels(gomock.Any(), []string{modelUUID}).
		Times(1).
		Return(nil, errors.New("fail"))

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.ErrorMatches, "fail")
}
