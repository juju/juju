// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/modelmanager"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
)

type modelmanagerCompatSuite struct {
}

func TestModelmanagerCompatSuite(t *testing.T) {
	tc.Run(t, &modelmanagerCompatSuite{})
}

func (s *modelmanagerCompatSuite) TestListModelSummariesWithOlderFacadeVersion(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	userTag := names.NewUserTag("alice@canonical.com")
	expectedQualifier := string(model.QualifierFromUserTag(userTag))
	testModelInfo := createModelSummaryLegacy()

	args := params.ModelSummariesRequest{
		UserTag: userTag.String(),
		All:     true,
	}

	tests := []struct {
		about        string
		resultValue  params.ModelSummaryResultsLegacy
		assertResult func(*tc.C, []base.UserModelSummary)
	}{{
		about: "model summaries are converted correctly",
		resultValue: params.ModelSummaryResultsLegacy{
			Results: []params.ModelSummaryResultLegacy{
				{Result: testModelInfo},
				{Error: apiservererrors.ServerError(errors.New("model error"))},
			},
		},
		assertResult: func(c *tc.C, results []base.UserModelSummary) {
			c.Assert(results, tc.HasLen, 2)
			c.Assert(results[0], tc.DeepEquals, base.UserModelSummary{Name: testModelInfo.Name,
				UUID:            testModelInfo.UUID,
				Type:            model.IAAS,
				ControllerUUID:  testModelInfo.ControllerUUID,
				ProviderType:    testModelInfo.ProviderType,
				Cloud:           "aws",
				CloudRegion:     "us-east-1",
				CloudCredential: "foo/bob/one",
				Qualifier:       model.Qualifier(expectedQualifier),
				Life:            "alive",
				Status: base.Status{
					Status: status.Active,
					Data:   map[string]interface{}{},
				},
				ModelUserAccess: "admin",
				Counts:          []base.EntityCount{},
			})
			c.Assert(errors.Cause(results[1].Error), tc.ErrorMatches, "model error")
		},
	}, {
		about: "no summaries",
		resultValue: params.ModelSummaryResultsLegacy{
			Results: []params.ModelSummaryResultLegacy{},
		},
		assertResult: func(c *tc.C, results []base.UserModelSummary) {
			c.Assert(results, tc.HasLen, 0)
		},
	}}

	for _, test := range tests {
		result := new(params.ModelSummaryResultsLegacy)

		mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
		mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListModelSummaries", args, result).SetArg(3, test.resultValue).Return(nil)
		client := modelmanager.NewLegacyClientFromCaller(mockFacadeCaller)

		results, err := client.ListModelSummaries(c.Context(), userTag.Id(), true)
		c.Assert(err, tc.ErrorIsNil)
		test.assertResult(c, results)
	}
}

func createModelSummaryLegacy() *params.ModelSummaryLegacy {
	return &params.ModelSummaryLegacy{
		Name:               "name",
		UUID:               "uuid",
		OwnerTag:           "user-alice@canonical.com",
		Type:               "iaas",
		ControllerUUID:     "controllerUUID",
		ProviderType:       "aws",
		CloudTag:           "cloud-aws",
		CloudRegion:        "us-east-1",
		CloudCredentialTag: "cloudcred-foo_bob_one",
		Life:               life.Alive,
		Status:             params.EntityStatus{Status: status.Status("active")},
		UserAccess:         params.ModelAdminAccess,
		Counts:             []params.ModelEntityCount{},
	}
}
