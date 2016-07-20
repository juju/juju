// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"regexp"
	"runtime"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/modelmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/status"
	jujuversion "github.com/juju/juju/version"
	// Register the providers for the field check test
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/controller"
	_ "github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type modelManagerBaseSuite struct {
}

type modelManagerSuite struct {
	gitjujutesting.IsolationSuite
	st         mockState
	authoriser apiservertesting.FakeAuthorizer
	api        *modelmanager.ModelManagerAPI
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	attrs := dummy.SampleConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	s.st = mockState{
		uuid: coretesting.ModelTag.Id(),
		cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		controllerModel: &mockModel{
			owner: names.NewUserTag("admin@local"),
			life:  state.Alive,
			cfg:   cfg,
			status: status.StatusInfo{
				Status: status.StatusAvailable,
				Since:  &time.Time{},
			},
			users: []*mockModelUser{{
				userName: "admin",
				access:   description.AdminAccess,
			}},
		},
		model: &mockModel{
			owner: names.NewUserTag("admin@local"),
			life:  state.Alive,
			cfg:   cfg,
			status: status.StatusInfo{
				Status: status.StatusAvailable,
				Since:  &time.Time{},
			},
			users: []*mockModelUser{{
				userName: "admin",
				access:   description.AdminAccess,
			}},
		},
		creds: map[string]cloud.Credential{
			"some-credential": cloud.NewEmptyCredential(),
		},
	}
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin@local"),
	}
	api, err := modelmanager.NewModelManagerAPI(&s.st, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *modelManagerSuite) TestCreateModelArgs(c *gc.C) {
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin@local",
		Config: map[string]interface{}{
			"bar": "baz",
		},
		CloudRegion:     "qux",
		CloudCredential: "some-credential",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c,
		"IsControllerAdministrator",
		"ModelUUID",
		"ControllerModel",
		"CloudCredentials",
		"ControllerConfig",
		"NewModel",
		"ForModel",
		"Model",
		"ControllerConfig",
		"Close", // close new model's state
		"Close", // close controller model's state
	)

	// We cannot predict the UUID, because it's generated,
	// so we just extract it and ensure that it's not the
	// same as the controller UUID.
	newModelArgs := s.st.Calls()[5].Args[0].(state.ModelArgs)
	uuid := newModelArgs.Config.UUID()
	c.Assert(uuid, gc.Not(gc.Equals), s.st.controllerModel.cfg.UUID())

	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":          "foo",
		"type":          "dummy",
		"uuid":          uuid,
		"agent-version": jujuversion.Current.String(),
		"bar":           "baz",
		"controller":    false,
		"broken":        "",
		"secret":        "pork",
	})
	c.Assert(err, jc.ErrorIsNil)
	// TODO(wallyworld) - we need to separate controller and model schemas
	// Remove any remaining controller attributes from the env config.
	cfg, err = cfg.Remove(controller.ControllerOnlyConfigAttributes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newModelArgs, jc.DeepEquals, state.ModelArgs{
		Owner:           names.NewUserTag("admin@local"),
		CloudName:       "some-cloud",
		CloudRegion:     "qux",
		CloudCredential: "some-credential",
		Config:          cfg,
	})
}

func (s *modelManagerSuite) TestCreateModelDefaultRegion(c *gc.C) {
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin@local",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.st.Calls()[5].Args[0].(state.ModelArgs)
	c.Assert(newModelArgs.CloudRegion, gc.Equals, "some-region")
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdmin(c *gc.C) {
	s.testCreateModelDefaultCredentialAdmin(c, "user-admin@local")
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdminNoDomain(c *gc.C) {
	s.testCreateModelDefaultCredentialAdmin(c, "user-admin")
}

func (s *modelManagerSuite) testCreateModelDefaultCredentialAdmin(c *gc.C, ownerTag string) {
	s.st.cloud.AuthTypes = []cloud.AuthType{"userpass"}
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: ownerTag,
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.st.Calls()[5].Args[0].(state.ModelArgs)
	c.Assert(newModelArgs.CloudCredential, gc.Equals, "some-credential")
}

func (s *modelManagerSuite) TestCreateModelEmptyCredentialNonAdmin(c *gc.C) {
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-bob@local",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.st.Calls()[5].Args[0].(state.ModelArgs)
	c.Assert(newModelArgs.CloudCredential, gc.Equals, "")
}

func (s *modelManagerSuite) TestCreateModelNoDefaultCredentialNonAdmin(c *gc.C) {
	s.st.cloud.AuthTypes = nil
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-bob@local",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, gc.ErrorMatches, "no credential specified")
}

func (s *modelManagerSuite) TestCreateModelUnknownCredential(c *gc.C) {
	args := params.ModelCreateArgs{
		Name:            "foo",
		OwnerTag:        "user-admin@local",
		CloudCredential: "bar",
	}
	_, err := s.api.CreateModel(args)
	c.Assert(err, gc.ErrorMatches, `no such credential "bar"`)
}

// modelManagerStateSuite contains end-to-end tests.
// Prefer adding tests to modelManagerSuite above.
type modelManagerStateSuite struct {
	jujutesting.JujuConnSuite
	modelmanager *modelmanager.ModelManagerAPI
	authoriser   apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&modelManagerStateSuite{})

func (s *modelManagerStateSuite) SetUpSuite(c *gc.C) {
	// TODO(anastasiamac 2016-07-19): Fix this on windows
	if runtime.GOOS != "linux" {
		c.Skip("bug 1603585: Skipping this on windows for now")
	}
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *modelManagerStateSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	loggo.GetLogger("juju.apiserver.modelmanager").SetLogLevel(loggo.TRACE)
}

func (s *modelManagerStateSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	modelmanager, err := modelmanager.NewModelManagerAPI(
		common.NewModelManagerBackend(s.State), s.authoriser,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.modelmanager = modelmanager
}

func (s *modelManagerStateSuite) TestNewAPIAcceptsClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("external@remote")
	endPoint, err := modelmanager.NewModelManagerAPI(
		common.NewModelManagerBackend(s.State), anAuthoriser,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *modelManagerStateSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	endPoint, err := modelmanager.NewModelManagerAPI(
		common.NewModelManagerBackend(s.State), anAuthoriser,
	)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) createArgs(c *gc.C, owner names.UserTag) params.ModelCreateArgs {
	return params.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: owner.String(),
		Config: map[string]interface{}{
			"authorized-keys": "ssh-key",
			// And to make it a valid dummy config
			"controller": false,
		},
	}
}

func (s *modelManagerStateSuite) createArgsForVersion(c *gc.C, owner names.UserTag, ver interface{}) params.ModelCreateArgs {
	params := s.createArgs(c, owner)
	params.Config["agent-version"] = ver
	return params
}

func (s *modelManagerStateSuite) TestUserCanCreateModel(c *gc.C) {
	owner := names.NewUserTag("admin@local")
	s.setAPIUser(c, owner)
	model, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
}

func (s *modelManagerStateSuite) TestAdminCanCreateModelForSomeoneElse(c *gc.C) {
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

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSomeoneElse(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))
	owner := names.NewUserTag("external@remote")
	_, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSelf(c *gc.C) {
	owner := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, owner)
	_, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestCreateModelValidatesConfig(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgs(c, admin)
	args.Config["controller"] = "maybe"
	_, err := s.modelmanager.CreateModel(args)
	c.Assert(err, gc.ErrorMatches,
		"failed to create config: provider validation failed: controller: expected bool, got string\\(\"maybe\"\\)",
	)
}

func (s *modelManagerStateSuite) TestCreateModelBadConfig(c *gc.C) {
	owner := names.NewUserTag("admin@local")
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
		},
	} {
		c.Logf("%d: %s", i, test.key)
		args := s.createArgs(c, owner)
		args.Config[test.key] = test.value
		_, err := s.modelmanager.CreateModel(args)
		c.Assert(err, gc.ErrorMatches, test.errMatch)

	}
}

func (s *modelManagerStateSuite) TestCreateModelSameAgentVersion(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgsForVersion(c, admin, jujuversion.Current.String())
	_, err := s.modelmanager.CreateModel(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerStateSuite) TestCreateModelBadAgentVersion(c *gc.C) {
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

func (s *modelManagerStateSuite) TestListModelsForSelf(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModels(params.Entity{Tag: user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerStateSuite) TestListModelsForSelfLocalUser(c *gc.C) {
	// When the user's credentials cache stores the simple name, but the
	// api server converts it to a fully qualified name.
	user := names.NewUserTag("local-user")
	s.setAPIUser(c, names.NewUserTag("local-user@local"))
	result, err := s.modelmanager.ListModels(params.Entity{Tag: user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerStateSuite) checkModelMatches(c *gc.C, env params.Model, expected *state.Model) {
	c.Check(env.Name, gc.Equals, expected.Name())
	c.Check(env.UUID, gc.Equals, expected.UUID())
	c.Check(env.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *modelManagerStateSuite) TestListModelsAdminSelf(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModels(params.Entity{Tag: user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 1)
	expected, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.checkModelMatches(c, result.UserModels[0].Model, expected)
}

func (s *modelManagerStateSuite) TestListModelsAdminListsOther(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	other := names.NewUserTag("external@remote")
	result, err := s.modelmanager.ListModels(params.Entity{Tag: other.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerStateSuite) TestListModelsDenied(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.modelmanager.ListModels(params.Entity{Tag: other.String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestAdminModelManager(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestNonAdminModelManager(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), jc.IsFalse)
}

func (s *modelManagerStateSuite) TestDestroyOwnModel(c *gc.C) {
	// TODO(perrito666) this test is not valid until we have
	// proper controller permission since the only users that
	// can create models are controller admins.
	owner := names.NewUserTag("admin@local")
	s.setAPIUser(c, owner)
	m, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	st, err := s.State.ForModel(names.NewModelTag(m.UUID))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		common.NewModelManagerBackend(st), s.authoriser,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.modelmanager.DestroyModel()
	c.Assert(err, jc.ErrorIsNil)

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Not(gc.Equals), state.Alive)
}

func (s *modelManagerStateSuite) TestAdminDestroysOtherModel(c *gc.C) {
	// TODO(perrito666) Both users are admins in this case, this tesst is of dubious
	// usefulness until proper controller permissions are in place.
	owner := names.NewUserTag("admin@local")
	s.setAPIUser(c, owner)
	m, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	st, err := s.State.ForModel(names.NewModelTag(m.UUID))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		common.NewModelManagerBackend(st), s.authoriser,
	)
	c.Assert(err, jc.ErrorIsNil)

	other := s.AdminUserTag(c)
	s.setAPIUser(c, other)
	err = s.modelmanager.DestroyModel()
	c.Assert(err, jc.ErrorIsNil)

	s.setAPIUser(c, owner)
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Not(gc.Equals), state.Alive)
}

func (s *modelManagerStateSuite) TestUserDestroysOtherModelDenied(c *gc.C) {
	owner := names.NewUserTag("admin@local")
	s.setAPIUser(c, owner)
	m, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	st, err := s.State.ForModel(names.NewModelTag(m.UUID))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		common.NewModelManagerBackend(st), s.authoriser,
	)
	c.Assert(err, jc.ErrorIsNil)

	user := names.NewUserTag("other@remote")
	s.setAPIUser(c, user)
	err = s.modelmanager.DestroyModel()
	c.Assert(err, gc.ErrorMatches, "permission denied")

	s.setAPIUser(c, owner)
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Alive)
}

func (s *modelManagerStateSuite) modifyAccess(c *gc.C, user names.UserTag, action params.ModelAction, access params.ModelAccessPermission, model names.ModelTag) error {
	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  user.String(),
			Action:   action,
			Access:   access,
			ModelTag: model.String(),
		}}}
	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	return result.OneError()
}

func (s *modelManagerStateSuite) grant(c *gc.C, user names.UserTag, access params.ModelAccessPermission, model names.ModelTag) error {
	return s.modifyAccess(c, user, params.GrantModelAccess, access, model)
}

func (s *modelManagerStateSuite) revoke(c *gc.C, user names.UserTag, access params.ModelAccessPermission, model names.ModelTag) error {
	return s.modifyAccess(c, user, params.RevokeModelAccess, access, model)
}

func (s *modelManagerStateSuite) TestGrantMissingUserFails(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	user := names.NewLocalUserTag("foobar")
	err := s.grant(c, user, params.ModelReadAccess, st.ModelTag())
	expectedErr := `could not grant model access: user "foobar" does not exist locally: user "foobar" not found`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestGrantMissingModelFails(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, nil)
	model := names.NewModelTag("17e4bd2d-3e08-4f3d-b945-087be7ebdce4")
	err := s.grant(c, user.UserTag(), params.ModelReadAccess, model)
	expectedErr := `.*model not found`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestRevokeAdminLeavesReadAccess(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, &factory.ModelUserParams{Access: description.WriteAccess})

	err := s.revoke(c, user.UserTag(), params.ModelWriteAccess, user.ModelTag())
	c.Assert(err, gc.IsNil)

	modelUser, err := s.State.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.IsReadOnly(), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestRevokeReadRemovesModelUser(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	user := s.Factory.MakeModelUser(c, nil)

	err := s.revoke(c, user.UserTag(), params.ModelReadAccess, user.ModelTag())
	c.Assert(err, gc.IsNil)

	_, err = s.State.ModelUser(user.UserTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestRevokeModelMissingUser(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	user := names.NewUserTag("bob")
	err := s.revoke(c, user, params.ModelReadAccess, st.ModelTag())
	c.Assert(err, gc.ErrorMatches, `could not revoke model access: model user "bob@local" does not exist`)

	_, err = st.ModelUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestGrantOnlyGreaterAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := s.grant(c, user.UserTag(), params.ModelReadAccess, st.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.grant(c, user.UserTag(), params.ModelReadAccess, st.ModelTag())
	c.Assert(err, gc.ErrorMatches, `user already has "read" access or greater`)
}

func (s *modelManagerStateSuite) assertNewUser(c *gc.C, modelUser *state.ModelUser, userTag, creatorTag names.UserTag) {
	c.Assert(modelUser.UserTag(), gc.Equals, userTag)
	c.Assert(modelUser.CreatedBy(), gc.Equals, creatorTag.Canonical())
	_, err := modelUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
}

func (s *modelManagerStateSuite) TestGrantModelAddLocalUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	apiUser := s.AdminUserTag(c)
	s.setAPIUser(c, apiUser)
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := s.grant(c, user.UserTag(), params.ModelReadAccess, st.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	modelUser, err := st.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	s.assertNewUser(c, modelUser, user.UserTag(), apiUser)
	c.Assert(modelUser.IsReadOnly(), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestGrantModelAddRemoteUser(c *gc.C) {
	userTag := names.NewUserTag("foobar@ubuntuone")
	apiUser := s.AdminUserTag(c)
	s.setAPIUser(c, apiUser)
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := s.grant(c, userTag, params.ModelReadAccess, st.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	modelUser, err := st.ModelUser(userTag)
	c.Assert(err, jc.ErrorIsNil)

	s.assertNewUser(c, modelUser, userTag, apiUser)
	c.Assert(modelUser.IsReadOnly(), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestGrantModelAddAdminUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", NoModelUser: true})
	apiUser := s.AdminUserTag(c)
	s.setAPIUser(c, apiUser)
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	err := s.grant(c, user.UserTag(), params.ModelWriteAccess, st.ModelTag())

	modelUser, err := st.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	s.assertNewUser(c, modelUser, user.UserTag(), apiUser)
	c.Assert(modelUser.IsReadOnly(), jc.IsFalse)
}

func (s *modelManagerStateSuite) TestGrantModelIncreaseAccess(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	stFactory := factory.NewFactory(st)
	user := stFactory.MakeModelUser(c, &factory.ModelUserParams{Access: description.ReadAccess})

	err := s.grant(c, user.UserTag(), params.ModelWriteAccess, st.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	modelUser, err := st.ModelUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.IsReadWrite(), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestGrantToModelNoAccess(c *gc.C) {
	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	other := names.NewUserTag("other@remote")
	err := s.grant(c, other, params.ModelReadAccess, st.ModelTag())
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestGrantToModelReadAccess(c *gc.C) {
	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	stFactory := factory.NewFactory(st)
	stFactory.MakeModelUser(c, &factory.ModelUserParams{
		User: apiUser.Canonical(), Access: description.ReadAccess})

	other := names.NewUserTag("other@remote")
	err := s.grant(c, other, params.ModelReadAccess, st.ModelTag())
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestGrantToModelWriteAccess(c *gc.C) {
	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	stFactory := factory.NewFactory(st)
	stFactory.MakeModelUser(c, &factory.ModelUserParams{
		User: apiUser.Canonical(), Access: description.AdminAccess})

	other := names.NewUserTag("other@remote")
	err := s.grant(c, other, params.ModelReadAccess, st.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	modelUser, err := st.ModelUser(other)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNewUser(c, modelUser, other, apiUser)
	c.Assert(modelUser.IsReadOnly(), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestGrantModelInvalidUserTag(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	for _, testParam := range []struct {
		tag      string
		validTag bool
	}{{
		tag:      "unit-foo/0",
		validTag: true,
	}, {
		tag:      "application-foo",
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
	}
}

func (s *modelManagerStateSuite) TestModifyModelAccessEmptyArgs(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	args := params.ModifyModelAccessRequest{Changes: []params.ModifyModelAccess{{}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not modify model access: invalid model access permission ""`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestModifyModelAccessInvalidAction(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	var dance params.ModelAction = "dance"
	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{{
			UserTag:  "user-user@local",
			Action:   dance,
			Access:   params.ModelReadAccess,
			ModelTag: s.State.ModelTag().String(),
		}}}

	result, err := s.modelmanager.ModifyModelAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `unknown action "dance"`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}

type fakeProvider struct {
	environs.EnvironProvider
}

func (*fakeProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	return cfg, nil
}

func (*fakeProvider) PrepareForCreateEnvironment(controllerUUID string, cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

func init() {
	environs.RegisterProvider("fake", &fakeProvider{})
}
