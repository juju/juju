// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
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

	api *modelmanager.ModelManagerAPI
}

var _ = gc.Suite(&ListModelsWithInfoSuite{})

func (s *ListModelsWithInfoSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	adminUser := "admin"
	s.adminUser = names.NewUserTag(adminUser)

	s.st = &mockState{
		model: s.createModel(c, s.adminUser),
	}
	s.st.modelDetailsForUser = func() ([]state.ModelDetails, error) {
		return []state.ModelDetails{s.st.model.getModelDetails()}, s.st.NextErr()
	}

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.adminUser,
	}
	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, nil, s.authoriser, s.st.model)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *ListModelsWithInfoSuite) createModel(c *gc.C, user names.UserTag) *mockModel {
	attrs := dummy.SampleConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	return &mockModel{
		owner: user,
		cfg:   cfg,
	}
}

func (s *ListModelsWithInfoSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	modelmanager, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, nil, s.authoriser, s.st.model)
	c.Assert(err, jc.ErrorIsNil)
	s.api = modelmanager
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoForSelf(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	s.st.model = s.createModel(c, user)
	result, err := s.api.ListModelsWithInfo(params.Entity{Tag: user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoForSelfLocalUser(c *gc.C) {
	// When the user's credentials cache stores the simple name, but the
	// api server converts it to a fully qualified name.
	user := names.NewUserTag("local-user")
	s.setAPIUser(c, user)
	s.st.model = s.createModel(c, user)
	result, err := s.api.ListModelsWithInfo(params.Entity{Tag: user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoAdminSelf(c *gc.C) {
	result, err := s.api.ListModelsWithInfo(params.Entity{Tag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)

	model := result.Results[0].Result
	c.Check(model.Name, gc.Equals, "only")
	c.Check(model.OwnerTag, gc.Equals, s.adminUser.String())
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoAdminListsOther(c *gc.C) {
	otherTag := names.NewUserTag("someotheruser")
	s.st.model = s.createModel(c, otherTag)
	result, err := s.api.ListModelsWithInfo(params.Entity{Tag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Result.OwnerTag, gc.DeepEquals, otherTag.String())
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoDenied(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.api.ListModelsWithInfo(params.Entity{Tag: other.String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoInvalidUser(c *gc.C) {
	_, err := s.api.ListModelsWithInfo(params.Entity{Tag: "invalid"})
	c.Assert(err, gc.ErrorMatches, `"invalid" is not a valid tag`)
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoStateError(c *gc.C) {
	errMsg := "captain error for ModelDetailsForUser"
	s.st.Stub.SetErrors(errors.New(errMsg))
	_, err := s.api.ListModelsWithInfo(params.Entity{Tag: s.adminUser.String()})
	c.Assert(err, gc.ErrorMatches, errMsg)
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoNoModelsForUser(c *gc.C) {
	s.st.modelDetailsForUser = func() ([]state.ModelDetails, error) {
		return []state.ModelDetails{}, nil
	}
	results, err := s.api.ListModelsWithInfo(params.Entity{Tag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *ListModelsWithInfoSuite) TestListModelsWithInfoForJustAModelUser(c *gc.C) {
	other := "other"
	otherTag := names.NewUserTag(other)
	s.setAPIUser(c, otherTag)
	s.st.modelDetailsForUser = func() ([]state.ModelDetails, error) {
		details := s.st.model.getModelDetails()
		details.Users = map[string]state.UserAccessInfo{
			"admin": state.UserAccessInfo{},
			"other": state.UserAccessInfo{
				UserAccess: permission.UserAccess{
					UserTag:  names.NewUserTag(other),
					Access:   permission.ReadAccess,
					UserName: other,
				},
			},
		}
		return []state.ModelDetails{details}, s.st.NextErr()
	}

	results, err := s.api.ListModelsWithInfo(params.Entity{Tag: otherTag.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result.Users, jc.DeepEquals, []params.ModelUserInfo{
		params.ModelUserInfo{
			UserName: other,
			Access:   "read",
		},
	})
}
