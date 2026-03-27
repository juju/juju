// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type ModelStatusSuite struct {
	coretesting.BaseSuite

	facade *mocks.MockFacadeCaller
}

func TestModelStatusSuite(t *testing.T) {
	tc.Run(t, &ModelStatusSuite{})
}

func (s *ModelStatusSuite) setup(c *tc.C, legacy bool) (*gomock.Controller, *common.ModelStatusAPI) {
	ctrl := gomock.NewController(c)

	s.facade = mocks.NewMockFacadeCaller(ctrl)

	return ctrl, common.NewModelStatusAPI(s.facade, legacy)
}

func (s *ModelStatusSuite) TestModelStatusLegacy(c *tc.C) {
	ctrl, api := s.setup(c, true)
	defer ctrl.Finish()

	s.facade.EXPECT().FacadeCall(gomock.Any(), "ModelStatus", params.Entities{
		Entities: []params.Entity{
			{Tag: coretesting.ModelTag.String()},
		},
	}, gomock.Any()).SetArg(3, params.ModelStatusResultsLegacy{
		Results: []params.ModelStatusLegacy{
			{
				ModelTag:           coretesting.ModelTag.String(),
				OwnerTag:           names.NewUserTag("alice").String(),
				ApplicationCount:   3,
				HostedMachineCount: 2,
				Life:               "alive",
				Machines: []params.ModelMachineInfo{{
					Id:         "0",
					InstanceId: "inst-ance",
					Status:     "pending",
				}},
			},
		},
	})

	results, err := api.ModelStatus(c.Context(), coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].UUID, tc.Equals, coretesting.ModelTag.Id())
	c.Check(results[0].HostedMachineCount, tc.Equals, 2)
	c.Check(results[0].ApplicationCount, tc.Equals, 3)
	// We expect the returned Qualifier (after the compatibility layer) to be
	// the username instead of the user tag.
	c.Check(results[0].Qualifier, tc.Equals, model.Qualifier("alice"))
	c.Check(results[0].Life, tc.Equals, life.Alive)
}

func (s *ModelStatusSuite) TestModelStatusLegacyError(c *tc.C) {
	ctrl, api := s.setup(c, true)
	defer ctrl.Finish()

	s.facade.EXPECT().FacadeCall(gomock.Any(), "ModelStatus", gomock.Any(), gomock.Any()).SetArg(3, params.ModelStatusResultsLegacy{
		Results: []params.ModelStatusLegacy{
			{Error: apiservererrors.ServerError(errors.New("model error"))},
		},
	})

	results, err := api.ModelStatus(c.Context(), coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, "model error")
}

func (s *ModelStatusSuite) TestModelStatusLegacyEmpty(c *tc.C) {
	ctrl, api := s.setup(c, true)
	defer ctrl.Finish()

	s.facade.EXPECT().FacadeCall(gomock.Any(), "ModelStatus", gomock.Any(), gomock.Any()).SetArg(3, params.ModelStatusResultsLegacy{})

	results, err := api.ModelStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, []base.ModelStatus{})
}

func (s *ModelStatusSuite) TestModelStatusLegacyParseUserTagError(c *tc.C) {
	ctrl, api := s.setup(c, true)
	defer ctrl.Finish()

	s.facade.EXPECT().FacadeCall(gomock.Any(), "ModelStatus", gomock.Any(), gomock.Any()).SetArg(3, params.ModelStatusResultsLegacy{
		Results: []params.ModelStatusLegacy{
			{
				ModelTag: coretesting.ModelTag.String(),
				OwnerTag: "invalid-tag",
			},
		},
	})

	results, err := api.ModelStatus(c.Context(), coretesting.ModelTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0].Error, tc.ErrorMatches, ".*not a valid tag.*")
}
