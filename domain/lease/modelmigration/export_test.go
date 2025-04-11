// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type exportSuite struct {
	testing.IsolationSuite

	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) TestRegisterExport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterExport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *exportSuite) TestExportLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetApplicationLeadershipForModel(gomock.Any(), model.UUID("model-uuid")).Return(map[string]string{
		"prometheus": "prometheus/0",
	}, nil)

	op := exportOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"uuid": "model-uuid",
		},
	})
	application := model.AddApplication(description.ApplicationArgs{
		Name: "prometheus",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	leader := application.Leader()
	c.Assert(leader, gc.Equals, "prometheus/0")
}

func (s *exportSuite) TestExportLeaderNoModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetApplicationLeadershipForModel(gomock.Any(), model.UUID("model-uuid")).Return(nil, errors.Errorf("boom"))

	op := exportOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"uuid": "model-uuid",
		},
	})
	model.AddApplication(description.ApplicationArgs{
		Name: "prometheus",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, `getting application leadership: boom`)
}

func (s *exportSuite) TestExportLeaderNoApplications(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetApplicationLeadershipForModel(gomock.Any(), model.UUID("model-uuid")).Return(map[string]string{}, nil)

	op := exportOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}

	model := description.NewModel(description.ModelArgs{
		Config: map[string]any{
			"uuid": "model-uuid",
		},
	})
	model.AddApplication(description.ApplicationArgs{
		Name: "prometheus",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}
