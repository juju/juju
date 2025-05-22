// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type exportSuite struct {
	testhelpers.IsolationSuite

	coordinator *MockCoordinator
	service     *MockExportService
}

func TestExportSuite(t *stdtesting.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) TestRegisterExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterExport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *exportSuite) TestExportLeader(c *tc.C) {
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

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	leader := application.Leader()
	c.Assert(leader, tc.Equals, "prometheus/0")
}

func (s *exportSuite) TestExportLeaderNoModel(c *tc.C) {
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

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, `getting application leadership: boom`)
}

func (s *exportSuite) TestExportLeaderNoApplications(c *tc.C) {
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

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}
