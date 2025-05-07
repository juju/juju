// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = tc.Suite(&exportSuite{})

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

func (s *exportSuite) TestExportExternalController(c *tc.C) {
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
	c.Assert(dst.ExternalControllers(), tc.HasLen, 0)
	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)
	// Assert that the destination description model has one external
	// controller after the migration:
	c.Check(dst.ExternalControllers(), tc.HasLen, 1)
	c.Assert(dst.ExternalControllers()[0].ID(), tc.Equals, ctrlUUID)
	c.Assert(dst.ExternalControllers()[0].Addrs(), jc.SameContents, []string{"192.168.1.1:8080"})
	c.Assert(dst.ExternalControllers()[0].Alias(), tc.Equals, "external ctrl1")
	c.Assert(dst.ExternalControllers()[0].CACert(), tc.Equals, "ca-cert-1")
	c.Assert(dst.ExternalControllers()[0].Models(), jc.SameContents, []string{"model1", "model2"})
}

func (s *exportSuite) TestExportExternalControllerRequestsExternalControllerOnceWithSameUUID(c *tc.C) {
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
	c.Assert(dst.ExternalControllers(), tc.HasLen, 0)
	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)
	// Assert that the destination description model has one external
	// controller after the migration:
	c.Assert(dst.ExternalControllers(), tc.HasLen, 1)
	c.Assert(dst.ExternalControllers()[0].ID(), tc.Equals, ctrlUUID)
	c.Assert(dst.ExternalControllers()[0].Addrs(), jc.SameContents, []string{"192.168.1.1:8080"})
	c.Assert(dst.ExternalControllers()[0].Alias(), tc.Equals, "external ctrl1")
	c.Assert(dst.ExternalControllers()[0].CACert(), tc.Equals, "ca-cert-1")
	c.Assert(dst.ExternalControllers()[0].Models(), jc.SameContents, []string{"model1", "model2"})
}

func (s *exportSuite) TestExportExternalControllerRequestsExternalControllerOnceWithSameController(c *tc.C) {
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
	c.Assert(dst.ExternalControllers(), tc.HasLen, 0)
	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)
	// Assert that the destination description model has one external
	// controller after the migration:
	c.Assert(dst.ExternalControllers(), tc.HasLen, 1)
	c.Assert(dst.ExternalControllers()[0].ID(), tc.Equals, ctrlUUID)
	c.Assert(dst.ExternalControllers()[0].Addrs(), jc.SameContents, []string{"192.168.1.1:8080"})
	c.Assert(dst.ExternalControllers()[0].Alias(), tc.Equals, "external ctrl1")
	c.Assert(dst.ExternalControllers()[0].CACert(), tc.Equals, "ca-cert-1")
	c.Assert(dst.ExternalControllers()[0].Models(), jc.SameContents, []string{"model1", "model2"})
}

func (s *exportSuite) TestExportExternalControllerWithNoControllerNotFound(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, "test-external-controller not found")
}

func (s *exportSuite) TestExportExternalControllerFailsGettingExternalControllerEntities(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, "fail")
}
