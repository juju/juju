// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	domainmodel "github.com/juju/juju/domain/model"
	domainmodelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/rpc/params"
)

type statusSuite struct {
	testing.IsolationSuite

	modelUUID        model.UUID
	modelInfoService *MockModelInfoService
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.modelUUID = modeltesting.GenModelUUID(c)
	return ctrl
}

func (s *statusSuite) TestModelStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		UUID:         s.modelUUID,
		Name:         "model-name",
		Type:         model.IAAS,
		Cloud:        "mycloud",
		CloudRegion:  "region",
		AgentVersion: semversion.MustParse("4.0.0"),
	}, nil)
	s.modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(domainmodel.StatusInfo{
		Status:  status.Available,
		Message: "all good now",
		Since:   now,
	}, nil)

	client := &Client{modelInfoService: s.modelInfoService}
	statusInfo, err := client.modelStatus(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(statusInfo, gc.DeepEquals, params.ModelStatusInfo{
		Name:        "model-name",
		Type:        model.IAAS.String(),
		CloudTag:    "cloud-mycloud",
		CloudRegion: "region",
		Version:     "4.0.0",
		ModelStatus: params.DetailedStatus{
			Status: status.Available.String(),
			Info:   "all good now",
			Since:  &now,
		},
	})
}

func (s *statusSuite) TestModelStatusModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		UUID:         s.modelUUID,
		Name:         "model-name",
		Type:         model.IAAS,
		Cloud:        "mycloud",
		CloudRegion:  "region",
		AgentVersion: semversion.MustParse("4.0.0"),
	}, nil)
	s.modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(domainmodel.StatusInfo{}, domainmodelerrors.NotFound)

	client := &Client{modelInfoService: s.modelInfoService}
	_, err := client.modelStatus(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}
