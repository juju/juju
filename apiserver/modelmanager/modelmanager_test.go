// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/modelmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	// Register the providers for the field check test
	_ "github.com/juju/juju/provider/azure"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type modelManagerBaseSuite struct {
	jujutesting.JujuConnSuite

	modelmanager *modelmanager.ModelManagerAPI
	resources    *common.Resources
	authoriser   apiservertesting.FakeAuthorizer
}

func (s *modelManagerBaseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	loggo.GetLogger("juju.apiserver.modelmanager").SetLogLevel(loggo.TRACE)
}

func (s *modelManagerBaseSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	modelmanager, err := modelmanager.NewModelManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	s.modelmanager = modelmanager
}

type modelManagerSuite struct {
	modelManagerBaseSuite
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) TestNewAPIAcceptsClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("external@remote")
	endPoint, err := modelmanager.NewModelManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *modelManagerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	endPoint, err := modelmanager.NewModelManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerSuite) createArgs(c *gc.C, owner names.UserTag) params.ModelCreateArgs {
	return params.ModelCreateArgs{
		OwnerTag: owner.String(),
		Account:  make(map[string]interface{}),
		Config: map[string]interface{}{
			"name":            "test-model",
			"authorized-keys": "ssh-key",
			// And to make it a valid dummy config
			"controller": false,
		},
	}
}

func (s *modelManagerSuite) createArgsForVersion(c *gc.C, owner names.UserTag, ver interface{}) params.ModelCreateArgs {
	params := s.createArgs(c, owner)
	params.Config["agent-version"] = ver
	return params
}

func (s *modelManagerSuite) TestUserCanCreateModel(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	s.setAPIUser(c, owner)
	model, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
}

func (s *modelManagerSuite) TestAdminCanCreateModelForSomeoneElse(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	owner := names.NewUserTag("external@remote")
	model, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
	// Make sure that the environment created does actually have the correct
	// owner, and that owner is actually allowed to use the environment.
	newState, err := s.State.ForModel(names.NewModelTag(model.UUID))
	c.Assert(err, jc.ErrorIsNil)
	defer newState.Close()

	newModel, err := newState.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newModel.Owner(), gc.Equals, owner)
	_, err = newState.ModelUser(owner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestNonAdminCannotCreateModelForSomeoneElse(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))
	owner := names.NewUserTag("external@remote")
	_, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerSuite) TestConfigSkeleton(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))

	_, err := s.modelmanager.ConfigSkeleton(
		params.ModelSkeletonConfigArgs{Provider: "ec2"})
	c.Check(err, gc.ErrorMatches, `cannot create new model with credentials for provider type "ec2" on controller with provider type "dummy"`)
	_, err = s.modelmanager.ConfigSkeleton(
		params.ModelSkeletonConfigArgs{Region: "the sun"})
	c.Check(err, gc.ErrorMatches, `region value "the sun" not valid`)

	skeleton, err := s.modelmanager.ConfigSkeleton(params.ModelSkeletonConfigArgs{})
	c.Assert(err, jc.ErrorIsNil)

	// The apiPort changes every test run as the dummy provider
	// looks for a random open port.
	apiPort := s.Environ.Config().APIPort()

	c.Assert(skeleton.Config, jc.DeepEquals, params.ModelConfig{
		"type":            "dummy",
		"controller-uuid": coretesting.ModelTag.Id(),
		"ca-cert":         coretesting.CACert,
		"state-port":      1234,
		"api-port":        apiPort,
	})
}

func (s *modelManagerSuite) TestCreateModelValidatesConfig(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgs(c, admin)
	args.Config["controller"] = "maybe"
	_, err := s.modelmanager.CreateModel(args)
	c.Assert(err, gc.ErrorMatches,
		"failed to create config: provider validation failed: controller: expected bool, got string\\(\"maybe\"\\)",
	)
}

func (s *modelManagerSuite) TestCreateModelBadConfig(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	s.setAPIUser(c, owner)
	for i, test := range []struct {
		key      string
		value    interface{}
		errMatch string
	}{
		{
			key:      "uuid",
			value:    "anything",
			errMatch: `failed to create config: uuid is generated, you cannot specify one`,
		}, {
			key:      "type",
			value:    "fake",
			errMatch: `failed to create config: specified type "fake" does not match controller "dummy"`,
		}, {
			key:      "ca-cert",
			value:    coretesting.OtherCACert,
			errMatch: `failed to create config: (?s)specified ca-cert ".*" does not match controller ".*"`,
		}, {
			key:      "state-port",
			value:    9876,
			errMatch: `failed to create config: specified state-port "9876" does not match controller "1234"`,
		}, {
			// The api-port is dynamic, but always in user-space, so > 1024.
			key:      "api-port",
			value:    123,
			errMatch: `failed to create config: specified api-port "123" does not match controller ".*"`,
		},
	} {
		c.Logf("%d: %s", i, test.key)
		args := s.createArgs(c, owner)
		args.Config[test.key] = test.value
		_, err := s.modelmanager.CreateModel(args)
		c.Assert(err, gc.ErrorMatches, test.errMatch)

	}
}

func (s *modelManagerSuite) TestCreateModelSameAgentVersion(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgsForVersion(c, admin, jujuversion.Current.String())
	_, err := s.modelmanager.CreateModel(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestCreateModelBadAgentVersion(c *gc.C) {
	err := s.BackingState.SetModelAgentVersion(coretesting.FakeVersionNumber)
	c.Assert(err, jc.ErrorIsNil)

	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)

	bigger := coretesting.FakeVersionNumber
	bigger.Minor += 1

	smaller := coretesting.FakeVersionNumber
	smaller.Minor -= 1

	for i, test := range []struct {
		value    interface{}
		errMatch string
	}{
		{
			value:    42,
			errMatch: `failed to create config: agent-version must be a string but has type 'int'`,
		}, {
			value:    "not a number",
			errMatch: `failed to create config: invalid version \"not a number\"`,
		}, {
			value:    bigger.String(),
			errMatch: "failed to create config: agent-version .* cannot be greater than the controller .*",
		}, {
			value:    smaller.String(),
			errMatch: "failed to create config: no tools found for version .*",
		},
	} {
		c.Logf("test %d", i)
		args := s.createArgsForVersion(c, admin, test.value)
		_, err := s.modelmanager.CreateModel(args)
		c.Check(err, gc.ErrorMatches, test.errMatch)
	}
}

func (s *modelManagerSuite) TestListModelsForSelf(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModels(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerSuite) TestListModelsForSelfLocalUser(c *gc.C) {
	// When the user's credentials cache stores the simple name, but the
	// api server converts it to a fully qualified name.
	user := names.NewUserTag("local-user")
	s.setAPIUser(c, names.NewUserTag("local-user@local"))
	result, err := s.modelmanager.ListModels(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerSuite) checkModelMatches(c *gc.C, env params.Model, expected *state.Model) {
	c.Check(env.Name, gc.Equals, expected.Name())
	c.Check(env.UUID, gc.Equals, expected.UUID())
	c.Check(env.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *modelManagerSuite) TestListModelsAdminSelf(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModels(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 1)
	expected, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.checkModelMatches(c, result.UserModels[0].Model, expected)
}

func (s *modelManagerSuite) TestListModelsAdminListsOther(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	other := names.NewUserTag("external@remote")
	result, err := s.modelmanager.ListModels(params.Entity{other.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerSuite) TestListModelsDenied(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.modelmanager.ListModels(params.Entity{other.String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerSuite) TestAdminModelManager(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), jc.IsTrue)
}

func (s *modelManagerSuite) TestNonAdminModelManager(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), jc.IsFalse)
}

func (s *modelManagerSuite) TestGrantMissingUserFails(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  names.NewLocalUserTag("foobar").String(),
			Action:   params.GrantModelAccess,
			Access:   params.ModelReadAccess,
			ModelTag: names.NewModelTag(model.UUID()).String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not grant model access: user "foobar" does not exist locally: user "foobar" not found`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *modelManagerSuite) TestGrantMissingModelFails(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, nil)
	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.UserTag().String(),
			Action:   params.GrantModelAccess,
			Access:   params.ModelReadAccess,
			ModelTag: names.NewModelTag("17e4bd2d-3e08-4f3d-b945-087be7ebdce4").String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `model not found`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *modelManagerSuite) TestRevokeAdminLeavesReadAccess(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: state.ModelAdminAccess})
	modelUser, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.UserTag().String(),
			Action:   params.RevokeModelAccess,
			Access:   params.ModelWriteAccess,
			ModelTag: modelUser.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	modelUser, err = s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.ReadOnly(), jc.IsTrue)
}

func (s *modelManagerSuite) TestRevokeReadRemovesModelUser(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, nil)
	modelUser, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.UserTag().String(),
			Action:   params.RevokeModelAccess,
			Access:   params.ModelReadAccess,
			ModelTag: modelUser.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	_, err = s.State.ModelUser(user.UserTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *modelManagerSuite) TestRevokeModelMissingUser(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	user := names.NewUserTag("bob")
	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.String(),
			Action:   params.RevokeModelAccess,
			Access:   params.ModelReadAccess,
			ModelTag: model.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, `could not revoke model access: model user "bob@local" does not exist`)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)

	_, err = st.ModelUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *modelManagerSuite) TestGrantOnlyGreaterAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.UserTag().String(),
			Action:   params.GrantModelAccess,
			Access:   params.ModelReadAccess,
			ModelTag: model.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	result, err = s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, `user already has "read" access`)
}

func (s *modelManagerSuite) TestGrantModelAddLocalUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.UserTag().String(),
			Action:   params.GrantModelAccess,
			Access:   params.ModelReadAccess,
			ModelTag: model.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	modelUser, err := st.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName(), gc.Equals, user.UserTag().Canonical())
	c.Assert(modelUser.CreatedBy(), gc.Equals, "admin@local")
	c.Assert(modelUser.ReadOnly(), jc.IsTrue)
	lastConn, err := modelUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn, gc.Equals, time.Time{})
}

func (s *modelManagerSuite) TestGrantModelAddRemoteUser(c *gc.C) {
	user := names.NewUserTag("foobar@ubuntuone")
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.String(),
			Action:   params.GrantModelAccess,
			Access:   params.ModelReadAccess,
			ModelTag: model.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	modelUser, err := st.ModelUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName(), gc.Equals, user.Canonical())
	c.Assert(modelUser.CreatedBy(), gc.Equals, "admin@local")
	c.Assert(modelUser.ReadOnly(), jc.IsTrue)
	lastConn, err := modelUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *modelManagerSuite) TestGrantModelAddAdminUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.Tag().String(),
			Action:   params.GrantModelAccess,
			Access:   params.ModelWriteAccess,
			ModelTag: model.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	modelUser, err := st.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName(), gc.Equals, user.UserTag().Canonical())
	c.Assert(modelUser.CreatedBy(), gc.Equals, "admin@local")
	c.Assert(modelUser.ReadOnly(), jc.IsFalse)
	lastConn, err := modelUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn, gc.Equals, time.Time{})
}

func (s *modelManagerSuite) TestGrantModelIncreaseAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.Tag().String(),
			Action:   params.GrantModelAccess,
			Access:   params.ModelReadAccess,
			ModelTag: model.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	args = params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.Tag().String(),
			Action:   params.GrantModelAccess,
			Access:   params.ModelWriteAccess,
			ModelTag: model.ModelTag().String(),
		}}}

	result, err = s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	modelUser, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName(), gc.Equals, user.UserTag().Canonical())
	c.Assert(modelUser.Access(), gc.Equals, state.ModelAdminAccess)
}

func (s *modelManagerSuite) TestGrantModelInvalidUserTag(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	for _, testParam := range []struct {
		tag      string
		validTag bool
	}{{
		tag:      "unit-foo/0",
		validTag: true,
	}, {
		tag:      "service-foo",
		validTag: true,
	}, {
		tag:      "relation-wordpress:db mysql:db",
		validTag: true,
	}, {
		tag:      "machine-0",
		validTag: true,
	}, {
		tag:      "user@local",
		validTag: false,
	}, {
		tag:      "user-Mua^h^h^h^arh",
		validTag: true,
	}, {
		tag:      "user@",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "user@ubuntuone",
		validTag: false,
	}, {
		tag:      "@ubuntuone",
		validTag: false,
	}, {
		tag:      "in^valid.",
		validTag: false,
	}, {
		tag:      "",
		validTag: false,
	},
	} {
		var expectedErr string
		errPart := `could not modify model access: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {
			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {
			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyModelAccessRequest{
			Changes: []params.ModifyModelAccess{{
				UserTag: testParam.tag,
				Action:  params.GrantModelAccess,
				Access:  params.ModelReadAccess,
			}}}

		result, err := s.modelmanager.ModifyModelAccess(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
		c.Assert(result.Results, gc.HasLen, 1)
		c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
	}
}

func (s *modelManagerSuite) TestModifyModelAccessEmptyArgs(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	args := params.ModifyModelAccessRequest{Changes: []params.ModifyModelAccess{{}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not modify model access: invalid model access permission ""`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *modelManagerSuite) TestModifyModelAccessInvalidAction(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	var dance params.ModelAction = "dance"
	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  "user-user@local",
			Action:   dance,
			Access:   params.ModelReadAccess,
			ModelTag: names.NewModelTag(model.UUID()).String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `unknown action "dance"`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, expectedErr)
}

type fakeProvider struct {
	environs.EnvironProvider
}

func (*fakeProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	return cfg, nil
}

func (*fakeProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

func init() {
	environs.RegisterProvider("fake", &fakeProvider{})
}
