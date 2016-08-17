// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/controller"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/description"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type controllerSuite struct {
	jujutesting.JujuConnSuite

	controller *controller.ControllerAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	controller, err := controller.NewControllerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller

	loggo.GetLogger("juju.apiserver.controller").SetLogLevel(loggo.TRACE)
}

func (s *controllerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("mysql/0"),
	}
	endPoint, err := controller.NewControllerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) TestNewAPIRefusesNonAdmins(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	endPoint, err := controller.NewControllerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *controllerSuite) checkEnvironmentMatches(c *gc.C, env params.Model, expected *state.Model) {
	c.Check(env.Name, gc.Equals, expected.Name())
	c.Check(env.UUID, gc.Equals, expected.UUID())
	c.Check(env.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *controllerSuite) TestAllModels(c *gc.C) {
	admin := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})

	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "owned", Owner: admin.UserTag()}).Close()
	remoteUserTag := names.NewUserTag("user@remote")
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "user", Owner: remoteUserTag})
	defer st.Close()
	st.AddModelUser(state.UserAccessSpec{
		User:        admin.UserTag(),
		CreatedBy:   remoteUserTag,
		DisplayName: "Foo Bar",
		Access:      description.ReadAccess})

	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "no-access", Owner: remoteUserTag}).Close()

	response, err := s.controller.AllModels()
	c.Assert(err, jc.ErrorIsNil)
	// The results are sorted.
	expected := []string{"controller", "no-access", "owned", "user"}
	var obtained []string
	for _, env := range response.UserModels {
		obtained = append(obtained, env.Name)
		stateEnv, err := s.State.GetModel(names.NewModelTag(env.UUID))
		c.Assert(err, jc.ErrorIsNil)
		s.checkEnvironmentMatches(c, env.Model, stateEnv)
	}
	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *controllerSuite) TestListBlockedModels(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	list, err := s.controller.ListBlockedModels()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(list.Models, jc.DeepEquals, []params.ModelBlockInfo{
		params.ModelBlockInfo{
			Name:     "controller",
			UUID:     s.State.ModelUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
		params.ModelBlockInfo{
			Name:     "test",
			UUID:     st.ModelUUID(),
			OwnerTag: s.AdminUserTag(c).String(),
			Blocks: []string{
				"BlockDestroy",
				"BlockChange",
			},
		},
	})

}

func (s *controllerSuite) TestListBlockedModelsNoBlocks(c *gc.C) {
	list, err := s.controller.ListBlockedModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(list.Models, gc.HasLen, 0)
}

func (s *controllerSuite) TestModelConfig(c *gc.C) {
	env, err := s.controller.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Config["name"], jc.DeepEquals, params.ConfigValue{Value: "controller"})
}

func (s *controllerSuite) TestModelConfigFromNonController(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer st.Close()

	authorizer := &apiservertesting.FakeAuthorizer{Tag: s.AdminUserTag(c)}
	controller, err := controller.NewControllerAPI(st, common.NewResources(), authorizer)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := controller.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["name"], jc.DeepEquals, params.ConfigValue{Value: "controller"})
}

func (s *controllerSuite) TestControllerConfig(c *gc.C) {
	cfg, err := s.controller.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	cfgFromDB, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["controller-uuid"], gc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(cfg.Config["state-port"], gc.Equals, cfgFromDB.StatePort())
	c.Assert(cfg.Config["api-port"], gc.Equals, cfgFromDB.APIPort())
}

func (s *controllerSuite) TestControllerConfigFromNonController(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer st.Close()

	authorizer := &apiservertesting.FakeAuthorizer{Tag: s.AdminUserTag(c)}
	controller, err := controller.NewControllerAPI(st, common.NewResources(), authorizer)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := controller.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	cfgFromDB, err := s.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["controller-uuid"], gc.Equals, cfgFromDB.ControllerUUID())
	c.Assert(cfg.Config["state-port"], gc.Equals, cfgFromDB.StatePort())
	c.Assert(cfg.Config["api-port"], gc.Equals, cfgFromDB.APIPort())
}

func (s *controllerSuite) TestRemoveBlocks(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test"})
	defer st.Close()

	s.State.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	s.State.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")
	st.SwitchBlockOn(state.DestroyBlock, "TestBlockDestroyModel")
	st.SwitchBlockOn(state.ChangeBlock, "TestChangeBlock")

	err := s.controller.RemoveBlocks(params.RemoveBlocksArgs{All: true})
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *controllerSuite) TestRemoveBlocksNotAll(c *gc.C) {
	err := s.controller.RemoveBlocks(params.RemoveBlocksArgs{})
	c.Assert(err, gc.ErrorMatches, "not supported")
}

func (s *controllerSuite) TestWatchAllModels(c *gc.C) {
	watcherId, err := s.controller.WatchAllModels()
	c.Assert(err, jc.ErrorIsNil)

	watcherAPI_, err := apiserver.NewAllWatcher(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
		ID_:        watcherId.AllWatcherId,
	})
	c.Assert(err, jc.ErrorIsNil)
	watcherAPI := watcherAPI_.(*apiserver.SrvAllWatcher)
	defer func() {
		err := watcherAPI.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}()

	resultC := make(chan params.AllWatcherNextResults)
	go func() {
		result, err := watcherAPI.Next()
		c.Assert(err, jc.ErrorIsNil)
		resultC <- result
	}()

	select {
	case result := <-resultC:
		// Expect to see the initial environment be reported.
		deltas := result.Deltas
		c.Assert(deltas, gc.HasLen, 1)
		envInfo := deltas[0].Entity.(*multiwatcher.ModelInfo)
		c.Assert(envInfo.ModelUUID, gc.Equals, s.State.ModelUUID())
	case <-time.After(testing.LongWait):
		c.Fatal("timed out")
	}
}

func (s *controllerSuite) TestModelStatus(c *gc.C) {
	otherEnvOwner := s.Factory.MakeModelUser(c, nil)
	otherSt := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:  "dummytoo",
		Owner: otherEnvOwner.UserTag,
		ConfigAttrs: testing.Attrs{
			"controller": false,
		},
	})
	defer otherSt.Close()

	s.Factory.MakeMachine(c, &factory.MachineParams{Jobs: []state.MachineJob{state.JobManageModel}})
	s.Factory.MakeMachine(c, &factory.MachineParams{Jobs: []state.MachineJob{state.JobHostUnits}})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, nil),
	})

	otherFactory := factory.NewFactory(otherSt)
	otherFactory.MakeMachine(c, nil)
	otherFactory.MakeMachine(c, nil)
	otherFactory.MakeApplication(c, &factory.ApplicationParams{
		Charm: otherFactory.MakeCharm(c, nil),
	})

	controllerEnvTag := s.State.ModelTag().String()
	hostedEnvTag := otherSt.ModelTag().String()

	req := params.Entities{
		Entities: []params.Entity{{Tag: controllerEnvTag}, {Tag: hostedEnvTag}},
	}
	results, err := s.controller.ModelStatus(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.ModelStatus{{
		ModelTag:           controllerEnvTag,
		HostedMachineCount: 1,
		ApplicationCount:   1,
		OwnerTag:           "user-admin@local",
		Life:               params.Alive,
	}, {
		ModelTag:           hostedEnvTag,
		HostedMachineCount: 2,
		ApplicationCount:   1,
		OwnerTag:           otherEnvOwner.UserTag.String(),
		Life:               params.Alive,
	}})
}

func (s *controllerSuite) TestInitiateModelMigration(c *gc.C) {
	// Create two hosted models to migrate.
	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	// Kick off the migration.
	args := params.InitiateModelMigrationArgs{
		Specs: []params.ModelMigrationSpec{
			{
				ModelTag: st1.ModelTag().String(),
				TargetInfo: params.ModelMigrationTargetInfo{
					ControllerTag: randomModelTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert1",
					AuthTag:       names.NewUserTag("admin1").String(),
					Password:      "secret1",
				},
			}, {
				ModelTag: st2.ModelTag().String(),
				TargetInfo: params.ModelMigrationTargetInfo{
					ControllerTag: randomModelTag(),
					Addrs:         []string{"3.3.3.3:3333"},
					CACert:        "cert2",
					AuthTag:       names.NewUserTag("admin2").String(),
					Password:      "secret2",
				},
			},
		},
	}
	out, err := s.controller.InitiateModelMigration(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 2)

	states := []*state.State{st1, st2}
	for i, spec := range args.Specs {
		st := states[i]
		result := out.Results[i]

		c.Check(result.Error, gc.IsNil)
		c.Check(result.ModelTag, gc.Equals, spec.ModelTag)
		expectedId := st.ModelUUID() + ":0"
		c.Check(result.MigrationId, gc.Equals, expectedId)

		// Ensure the migration made it into the DB correctly.
		mig, err := st.LatestModelMigration()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(mig.Id(), gc.Equals, expectedId)
		c.Check(mig.ModelUUID(), gc.Equals, st.ModelUUID())
		c.Check(mig.InitiatedBy(), gc.Equals, s.AdminUserTag(c).Id())
		targetInfo, err := mig.TargetInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(targetInfo.ControllerTag.String(), gc.Equals, spec.TargetInfo.ControllerTag)
		c.Check(targetInfo.Addrs, jc.SameContents, spec.TargetInfo.Addrs)
		c.Check(targetInfo.CACert, gc.Equals, spec.TargetInfo.CACert)
		c.Check(targetInfo.AuthTag.String(), gc.Equals, spec.TargetInfo.AuthTag)
		c.Check(targetInfo.Password, gc.Equals, spec.TargetInfo.Password)
	}
}

func (s *controllerSuite) TestInitiateModelMigrationValidationError(c *gc.C) {
	// Create a hosted model to migrate.
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	// Kick off the migration with missing details.
	args := params.InitiateModelMigrationArgs{
		Specs: []params.ModelMigrationSpec{{
			ModelTag: st.ModelTag().String(),
			// TargetInfo missing
		}},
	}
	out, err := s.controller.InitiateModelMigration(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 1)
	result := out.Results[0]
	c.Check(result.ModelTag, gc.Equals, args.Specs[0].ModelTag)
	c.Check(result.MigrationId, gc.Equals, "")
	c.Check(result.Error, gc.ErrorMatches, "controller tag: .+ is not a valid tag")
}

func (s *controllerSuite) TestInitiateModelMigrationPartialFailure(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	args := params.InitiateModelMigrationArgs{
		Specs: []params.ModelMigrationSpec{
			{
				ModelTag: st.ModelTag().String(),
				TargetInfo: params.ModelMigrationTargetInfo{
					ControllerTag: randomModelTag(),
					Addrs:         []string{"1.1.1.1:1111", "2.2.2.2:2222"},
					CACert:        "cert",
					AuthTag:       names.NewUserTag("admin").String(),
					Password:      "secret",
				},
			}, {
				ModelTag: randomModelTag(), // Doesn't exist.
			},
		},
	}
	out, err := s.controller.InitiateModelMigration(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.Results, gc.HasLen, 2)

	c.Check(out.Results[0].ModelTag, gc.Equals, st.ModelTag().String())
	c.Check(out.Results[0].Error, gc.IsNil)

	c.Check(out.Results[1].ModelTag, gc.Equals, args.Specs[1].ModelTag)
	c.Check(out.Results[1].Error, gc.ErrorMatches, "unable to read model: .+")
}

func randomModelTag() string {
	uuid := utils.MustNewUUID().String()
	return names.NewModelTag(uuid).String()
}

func (s *controllerSuite) modifyControllerAccess(c *gc.C, user names.UserTag, action params.ControllerAction, access string) error {
	args := params.ModifyControllerAccessRequest{
		Changes: []params.ModifyControllerAccess{{
			UserTag: user.String(),
			Action:  action,
			Access:  access,
		}}}
	result, err := s.controller.ModifyControllerAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	return result.OneError()
}

func (s *controllerSuite) controllerGrant(c *gc.C, user names.UserTag, access string) error {
	return s.modifyControllerAccess(c, user, params.GrantControllerAccess, access)
}

func (s *controllerSuite) controllerRevoke(c *gc.C, user names.UserTag, access string) error {
	return s.modifyControllerAccess(c, user, params.RevokeControllerAccess, access)
}

func (s *controllerSuite) TestGrantMissingUserFails(c *gc.C) {
	user := names.NewLocalUserTag("foobar")
	err := s.controllerGrant(c, user, string(description.AddModelAccess))
	expectedErr := `could not grant controller access: user "foobar" does not exist locally: user "foobar" not found`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestRevokeSuperuserLeavesAddModelAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	err := s.controllerGrant(c, user.UserTag(), string(description.SuperuserAccess))
	c.Assert(err, gc.IsNil)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, description.SuperuserAccess)

	err = s.controllerRevoke(c, user.UserTag(), string(description.SuperuserAccess))
	c.Assert(err, gc.IsNil)

	controllerUser, err = s.State.UserAccess(user.UserTag(), controllerUser.Object)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, description.AddModelAccess)
}

func (s *controllerSuite) TestRevokeAddModelLeavesLoginAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	err := s.controllerGrant(c, user.UserTag(), string(description.AddModelAccess))
	c.Assert(err, gc.IsNil)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, description.AddModelAccess)

	err = s.controllerRevoke(c, user.UserTag(), string(description.AddModelAccess))
	c.Assert(err, gc.IsNil)

	controllerUser, err = s.State.UserAccess(user.UserTag(), controllerUser.Object)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, description.LoginAccess)
}

func (s *controllerSuite) TestRevokeLoginRemovesControllerUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	err := s.controllerRevoke(c, user.UserTag(), string(description.LoginAccess))
	c.Assert(err, gc.IsNil)

	ctag := names.NewControllerTag(s.State.ControllerUUID())
	_, err = s.State.UserAccess(user.UserTag(), ctag)

	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *controllerSuite) TestRevokeControllerMissingUser(c *gc.C) {
	user := names.NewLocalUserTag("foobar")
	err := s.controllerRevoke(c, user, string(description.AddModelAccess))
	expectedErr := `could not look up controller access for user: controller user "foobar@local" not found`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestGrantOnlyGreaterAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})

	err := s.controllerGrant(c, user.UserTag(), string(description.AddModelAccess))
	c.Assert(err, gc.IsNil)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, description.AddModelAccess)

	err = s.controllerGrant(c, user.UserTag(), string(description.AddModelAccess))
	expectedErr := `could not grant controller access: user already has "addmodel" access or greater`
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestGrantControllerAddRemoteUser(c *gc.C) {
	userTag := names.NewUserTag("foobar@ubuntuone")

	err := s.controllerGrant(c, userTag, string(description.AddModelAccess))
	c.Assert(err, jc.ErrorIsNil)

	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(userTag, ctag)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(controllerUser.Access, gc.Equals, description.AddModelAccess)
}

func (s *controllerSuite) TestGrantControllerInvalidUserTag(c *gc.C) {
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
		errPart := `could not modify controller access: "` + regexp.QuoteMeta(testParam.tag) + `" is not a valid `

		if testParam.validTag {
			// The string is a valid tag, but not a user tag.
			expectedErr = errPart + `user tag`
		} else {
			// The string is not a valid tag of any kind.
			expectedErr = errPart + `tag`
		}

		args := params.ModifyControllerAccessRequest{
			Changes: []params.ModifyControllerAccess{{
				UserTag: testParam.tag,
				Action:  params.GrantControllerAccess,
				Access:  string(description.SuperuserAccess),
			}}}

		result, err := s.controller.ModifyControllerAccess(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
	}
}

func (s *controllerSuite) TestModifyControllerAccessEmptyArgs(c *gc.C) {
	args := params.ModifyControllerAccessRequest{Changes: []params.ModifyControllerAccess{{}}}

	result, err := s.controller.ModifyControllerAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `"" controller access not valid`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}

func (s *controllerSuite) TestModifyControllerAccessInvalidAction(c *gc.C) {
	var dance params.ControllerAction = "dance"
	args := params.ModifyControllerAccessRequest{
		Changes: []params.ModifyControllerAccess{{
			UserTag: "user-user@local",
			Action:  dance,
			Access:  string(description.LoginAccess),
		}}}

	result, err := s.controller.ModifyControllerAccess(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `unknown action "dance"`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}
