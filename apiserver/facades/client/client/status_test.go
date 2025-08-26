// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	permission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	statusservice "github.com/juju/juju/domain/status/service"
	charm0 "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
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
			Message: "multiple requests",
			Code:    "not supported",
		},
	}, {
		Error: &params.Error{
			Message: "multiple requests",
			Code:    "not supported",
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
			Message: `status history kind "blah"`,
			Code:    "not valid",
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

func (s *statusSuite) TestFetchOffers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cmrService := NewMockCrossModelRelationService(ctrl)

	// Arrange
	charmLocator := charm.CharmLocator{
		Name:         "app",
		Revision:     42,
		Source:       charm.CharmHubSource,
		Architecture: architecture.AMD64,
	}
	offerDetails := []*crossmodelrelation.OfferDetail{
		{
			OfferUUID:              uuid.MustNewUUID().String(),
			OfferName:              "one",
			ApplicationName:        "test-app",
			ApplicationDescription: "testing application",
			CharmLocator:           charmLocator,
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{Name: "db"},
			},
			OfferUsers: []crossmodelrelation.OfferUser{{Name: "george", Access: permission.ConsumeAccess}},
		}, {
			OfferUUID:              uuid.MustNewUUID().String(),
			OfferName:              "two",
			ApplicationName:        "test-app",
			ApplicationDescription: "testing application",
			CharmLocator:           charmLocator,
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{Name: "endpoint"},
			},
			OfferUsers: []crossmodelrelation.OfferUser{{Name: "admin", Access: permission.AdminAccess}},
		},
	}
	cmrService.EXPECT().GetOffers(gomock.Any(), []crossmodelrelation.OfferFilter{{}}).Return(offerDetails, nil)

	// Act
	output, err := fetchOffers(c.Context(), cmrService)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(output, tc.HasLen, 2)
	outputOne, ok := output["one"]
	c.Check(ok, tc.IsTrue)

	c.Check(outputOne, tc.DeepEquals, offerStatus{
		ApplicationOffer: crossmodel.ApplicationOffer{
			OfferUUID:              offerDetails[0].OfferUUID,
			OfferName:              offerDetails[0].OfferName,
			ApplicationName:        offerDetails[0].ApplicationName,
			ApplicationDescription: offerDetails[0].ApplicationDescription,
			Endpoints: map[string]charm0.Relation{
				"db": {
					Name: "db",
				},
			},
		},
		err:      nil,
		charmURL: "ch:amd64/app-42",
	})
	outputTwo, ok := output["two"]
	c.Check(ok, tc.IsTrue)
	c.Check(outputTwo, tc.DeepEquals, offerStatus{
		ApplicationOffer: crossmodel.ApplicationOffer{
			OfferUUID:              offerDetails[1].OfferUUID,
			OfferName:              offerDetails[1].OfferName,
			ApplicationName:        offerDetails[1].ApplicationName,
			ApplicationDescription: offerDetails[1].ApplicationDescription,
			Endpoints: map[string]charm0.Relation{
				"endpoint": {
					Name: "endpoint",
				},
			},
		},
		err:      nil,
		charmURL: "ch:amd64/app-42",
	})
}

func (s *statusSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)

	s.modelUUID = modeltesting.GenModelUUID(c)

	c.Cleanup(func() {
		s.authorizer = nil
		s.modelInfoService = nil
		s.statusService = nil
		s.modelUUID = ""
	})

	return ctrl
}
