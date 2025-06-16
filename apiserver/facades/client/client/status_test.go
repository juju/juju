// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	permission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	domainmodelerrors "github.com/juju/juju/domain/model/errors"
	statusservice "github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type statusSuite struct {
	testhelpers.IsolationSuite

	modelUUID model.UUID

	authorizer       *MockAuthorizer
	modelInfoService *MockModelInfoService
	statusService    *MockStatusService
}

func TestStatusSuite(t *testing.T) {
	tc.Run(t, &statusSuite{})
}

func (s *statusSuite) TestModelStatus(c *tc.C) {
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
	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{
		Status:  status.Available,
		Message: "all good now",
		Since:   &now,
	}, nil)

	client := &Client{
		modelInfoService: s.modelInfoService,
		statusService:    s.statusService,
	}
	statusInfo, err := client.modelStatus(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(statusInfo, tc.DeepEquals, params.ModelStatusInfo{
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

func (s *statusSuite) TestModelStatusModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		UUID:         s.modelUUID,
		Name:         "model-name",
		Type:         model.IAAS,
		Cloud:        "mycloud",
		CloudRegion:  "region",
		AgentVersion: semversion.MustParse("4.0.0"),
	}, nil)
	s.statusService.EXPECT().GetModelStatus(gomock.Any()).Return(status.StatusInfo{}, domainmodelerrors.NotFound)

	client := &Client{
		modelInfoService: s.modelInfoService,
		statusService:    s.statusService,
	}
	_, err := client.modelStatus(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *statusSuite) TestStatusHistory(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag, err := names.ParseApplicationTag("application-foo")
	c.Assert(err, tc.ErrorIsNil)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)
	s.statusService.EXPECT().GetStatusHistory(gomock.Any(), statusservice.StatusHistoryRequest{
		Kind: status.KindApplication,
		Tag:  tag.Id(),
	}).Return([]status.DetailedStatus{{
		Kind:   status.KindApplication,
		Status: status.Allocating,
	}}, nil)

	client := &Client{
		statusService: s.statusService,
		auth:          s.authorizer,
	}
	results := client.StatusHistory(c.Context(), params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Kind: "application",
			Tag:  tag.String(),
		}},
	})
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results, tc.DeepEquals, []params.StatusHistoryResult{{
		History: params.History{
			Statuses: []params.DetailedStatus{{
				Kind:   "application",
				Status: status.Allocating.String(),
			}},
		},
	}})
}

func (s *statusSuite) TestStatusHistoryNoBulk(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag, err := names.ParseApplicationTag("application-foo")
	c.Assert(err, tc.ErrorIsNil)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)

	client := &Client{
		statusService: s.statusService,
		auth:          s.authorizer,
	}
	results := client.StatusHistory(c.Context(), params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Kind: "application",
			Tag:  tag.String(),
		}, {
			Kind: "application",
			Tag:  tag.String(),
		}},
	})
	c.Check(results.Results, tc.DeepEquals, []params.StatusHistoryResult{{
		Error: &params.Error{
			Message: "multiple requests are not supported",
		},
	}, {
		Error: &params.Error{
			Message: "multiple requests are not supported",
		},
	}})
}

func (s *statusSuite) TestStatusHistoryInvalidKind(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag, err := names.ParseApplicationTag("application-foo")
	c.Assert(err, tc.ErrorIsNil)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)

	client := &Client{
		statusService: s.statusService,
		auth:          s.authorizer,
	}
	results := client.StatusHistory(c.Context(), params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Kind: "blah",
			Tag:  tag.String(),
		}},
	})
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results, tc.DeepEquals, []params.StatusHistoryResult{{
		Error: &params.Error{
			Message: `invalid status history kind "blah"`,
		},
	}})
}

func (s *statusSuite) TestStatusHistoryInvalidTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)

	client := &Client{
		statusService: s.statusService,
		auth:          s.authorizer,
	}
	results := client.StatusHistory(c.Context(), params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Kind: "application",
			Tag:  "invalid-tag",
		}},
	})
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results, tc.DeepEquals, []params.StatusHistoryResult{{
		Error: &params.Error{
			Message: `"invalid-tag" is not a valid tag`,
		},
	}})
}

func (s *statusSuite) TestStatusHistoryError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag, err := names.ParseApplicationTag("application-foo")
	c.Assert(err, tc.ErrorIsNil)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, gomock.Any()).Return(nil)
	s.statusService.EXPECT().GetStatusHistory(gomock.Any(), statusservice.StatusHistoryRequest{
		Kind: status.KindApplication,
		Tag:  tag.Id(),
	}).Return([]status.DetailedStatus{{
		Kind:   status.KindApplication,
		Status: status.Allocating,
	}}, errors.Errorf("boom"))

	client := &Client{
		statusService: s.statusService,
		auth:          s.authorizer,
	}
	results := client.StatusHistory(c.Context(), params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Kind: "application",
			Tag:  tag.String(),
		}},
	})
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results, tc.DeepEquals, []params.StatusHistoryResult{{
		Error: &params.Error{
			Message: `boom`,
		},
	}})
}

func (s *statusSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)

	s.modelUUID = modeltesting.GenModelUUID(c)

	return ctrl
}
