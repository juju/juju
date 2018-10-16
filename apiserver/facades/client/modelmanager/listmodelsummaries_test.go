// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/permission"
	_ "github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/state"
	jujuversion "github.com/juju/juju/version"
)

type ListModelsWithInfoSuite struct {
	gitjujutesting.IsolationSuite

	st *mockState

	authoriser apiservertesting.FakeAuthorizer
	adminUser  names.UserTag

	api         *modelmanager.ModelManagerAPI
	callContext context.ProviderCallContext
}

var _ = gc.Suite(&ListModelsWithInfoSuite{})

func (s *ListModelsWithInfoSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	adminUser := "admin"
	s.adminUser = names.NewUserTag(adminUser)

	s.st = &mockState{
		model: s.createModel(c, s.adminUser),
	}
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		return []state.ModelSummary{s.st.model.getModelDetails()}, s.st.NextErr()
	}

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.adminUser,
	}

	s.callContext = context.NewCloudCallContext()
	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *ListModelsWithInfoSuite) createModel(c *gc.C, user names.UserTag) *mockModel {
	attrs := dummy.SampleConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return &mockModel{
		owner:               user,
		cfg:                 cfg,
		setCloudCredentialF: func(tag names.CloudCredentialTag) (bool, error) { return false, nil },
	}
}

func (s *ListModelsWithInfoSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	modelmanager, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
	s.api = modelmanager
}

// TODO (anastasiamac 2017-11-24) add test with migration and SLA

func (s *ListModelsWithInfoSuite) TestListModelSummaries(c *gc.C) {
	result, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelSummaryResults{
		Results: []params.ModelSummaryResult{
			{
				Result: &params.ModelSummary{
					Name:               "only",
					OwnerTag:           s.adminUser.String(),
					UUID:               s.st.ModelUUID(),
					Type:               string(state.ModelTypeIAAS),
					CloudTag:           "some-cloud",
					CloudRegion:        "some-region",
					CloudCredentialTag: "cloudcred-some-cloud_bob_some-credential",
					Life:               "alive",
					Status:             params.EntityStatus{},
					Counts:             []params.ModelEntityCount{},
					SLA:                &params.ModelSLAInfo{"essential", "admin"},
				},
			},
		},
	})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithUserAccess(c *gc.C) {
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.Access = permission.AdminAccess
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.UserAccess, jc.DeepEquals, params.ModelAdminAccess)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithLastConnected(c *gc.C) {
	now := time.Now()
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.UserLastConnection = &now
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.UserLastConnection, jc.DeepEquals, &now)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithMachineCount(c *gc.C) {
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.MachineCount = int64(64)
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.Counts[0], jc.DeepEquals, params.ModelEntityCount{params.Machines, 64})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithCoreCount(c *gc.C) {
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.CoreCount = int64(43)
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Result.Counts[0], jc.DeepEquals, params.ModelEntityCount{params.Cores, 43})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesWithMachineAndUserDetails(c *gc.C) {
	now := time.Now()
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		summary := s.st.model.getModelDetails()
		summary.Access = permission.AdminAccess
		summary.UserLastConnection = &now
		summary.MachineCount = int64(10)
		summary.CoreCount = int64(42)
		return []state.ModelSummary{summary}, nil
	}
	result, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelSummaryResults{
		Results: []params.ModelSummaryResult{
			{
				Result: &params.ModelSummary{
					Name:               "only",
					OwnerTag:           s.adminUser.String(),
					UUID:               s.st.ModelUUID(),
					Type:               string(state.ModelTypeIAAS),
					CloudTag:           "some-cloud",
					CloudRegion:        "some-region",
					CloudCredentialTag: "cloudcred-some-cloud_bob_some-credential",
					Life:               "alive",
					Status:             params.EntityStatus{},
					SLA:                &params.ModelSLAInfo{"essential", "admin"},
					UserAccess:         params.ModelAdminAccess,
					UserLastConnection: &now,
					Counts: []params.ModelEntityCount{
						{params.Machines, 10},
						{params.Cores, 42},
					},
				},
			},
		},
	})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesDenied(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: other.String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesInvalidUser(c *gc.C) {
	_, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: "invalid"})
	c.Assert(err, gc.ErrorMatches, `"invalid" is not a valid tag`)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesStateError(c *gc.C) {
	errMsg := "captain error for ModelSummariesForUser"
	s.st.Stub.SetErrors(errors.New(errMsg))
	_, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, gc.ErrorMatches, errMsg)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesNoModelsForUser(c *gc.C) {
	s.st.modelDetailsForUser = func() ([]state.ModelSummary, error) {
		return []state.ModelSummary{}, nil
	}
	results, err := s.api.ListModelSummaries(params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}
