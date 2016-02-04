// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type ModelSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ModelSuite{})

func (s *ModelSuite) TestModel(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	expectedTag := names.NewModelTag(model.UUID())
	c.Assert(model.Tag(), gc.Equals, expectedTag)
	c.Assert(model.ControllerTag(), gc.Equals, expectedTag)
	c.Assert(model.Name(), gc.Equals, "testenv")
	c.Assert(model.Owner(), gc.Equals, s.Owner)
	c.Assert(model.Life(), gc.Equals, state.Alive)
	c.Assert(model.TimeOfDying().IsZero(), jc.IsTrue)
	c.Assert(model.TimeOfDeath().IsZero(), jc.IsTrue)
}

func (s *ModelSuite) TestModelDestroy(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	now := state.NowToTheSecond()
	s.PatchValue(&state.NowToTheSecond, func() time.Time {
		return now
	})

	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)
	c.Assert(env.TimeOfDying().UTC(), gc.Equals, now.UTC())
	c.Assert(env.TimeOfDeath().IsZero(), jc.IsTrue)
}

func (s *ModelSuite) TestNewModelNonExistentLocalUser(c *gc.C) {
	cfg, _ := s.createTestEnvConfig(c)
	owner := names.NewUserTag("non-existent@local")

	_, _, err := s.State.NewModel(cfg, owner)
	c.Assert(err, gc.ErrorMatches, `cannot create model: user "non-existent" not found`)
}

func (s *ModelSuite) TestNewModelSameUserSameNameFails(c *gc.C) {
	cfg, _ := s.createTestEnvConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first model.
	_, st1, err := s.State.NewModel(cfg, owner)
	c.Assert(err, jc.ErrorIsNil)
	defer st1.Close()

	// Attempt to create another model with a different UUID but the
	// same owner and name as the first.
	newUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg2 := testing.CustomModelConfig(c, testing.Attrs{
		"name": cfg.Name(),
		"uuid": newUUID.String(),
	})
	_, _, err = s.State.NewModel(cfg2, owner)
	errMsg := fmt.Sprintf("model %q for %s already exists", cfg2.Name(), owner.Canonical())
	c.Assert(err, gc.ErrorMatches, errMsg)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)

	// Remove the first model.
	env1, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = env1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Destroy only sets the model to dying and RemoveAllModelDocs can
	// only be called on a dead model. Normally, the environ's lifecycle
	// would be set to dead after machines and services have been cleaned up.
	err = state.SetModelLifeDead(st1, env1.ModelTag().Id())
	c.Assert(err, jc.ErrorIsNil)
	err = st1.RemoveAllModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	// We should now be able to create the other model.
	env2, st2, err := s.State.NewModel(cfg2, owner)
	c.Assert(err, jc.ErrorIsNil)
	defer st2.Close()
	c.Assert(env2, gc.NotNil)
	c.Assert(st2, gc.NotNil)
}

func (s *ModelSuite) TestNewModel(c *gc.C) {
	cfg, uuid := s.createTestEnvConfig(c)
	owner := names.NewUserTag("test@remote")

	env, st, err := s.State.NewModel(cfg, owner)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	modelTag := names.NewModelTag(uuid)
	assertEnvMatches := func(env *state.Model) {
		c.Assert(env.UUID(), gc.Equals, modelTag.Id())
		c.Assert(env.Tag(), gc.Equals, modelTag)
		c.Assert(env.ControllerTag(), gc.Equals, s.modelTag)
		c.Assert(env.Owner(), gc.Equals, owner)
		c.Assert(env.Name(), gc.Equals, "testing")
		c.Assert(env.Life(), gc.Equals, state.Alive)
	}
	assertEnvMatches(env)

	// Since the model tag for the State connection is different,
	// asking for this model through FindEntity returns a not found error.
	env, err = s.State.GetModel(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	assertEnvMatches(env)

	env, err = st.Model()
	c.Assert(err, jc.ErrorIsNil)
	assertEnvMatches(env)

	_, err = s.State.FindEntity(modelTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	entity, err := st.FindEntity(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, modelTag)

	// Ensure the model is functional by adding a machine
	_, err = st.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSuite) TestControllerModel(c *gc.C) {
	env, err := s.State.ControllerModel()
	c.Assert(err, jc.ErrorIsNil)

	expectedTag := names.NewModelTag(env.UUID())
	c.Assert(env.Tag(), gc.Equals, expectedTag)
	c.Assert(env.ControllerTag(), gc.Equals, expectedTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.Owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *ModelSuite) TestControllerModelAccessibleFromOtherModels(c *gc.C) {
	cfg, _ := s.createTestEnvConfig(c)
	_, st, err := s.State.NewModel(cfg, names.NewUserTag("test@remote"))
	defer st.Close()

	env, err := st.ControllerModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Tag(), gc.Equals, s.modelTag)
	c.Assert(env.Name(), gc.Equals, "testenv")
	c.Assert(env.Owner(), gc.Equals, s.Owner)
	c.Assert(env.Life(), gc.Equals, state.Alive)
}

func (s *ModelSuite) TestConfigForControllerEnv(c *gc.C) {
	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Name: "other"})
	defer otherState.Close()

	env, err := otherState.GetModel(s.modelTag)
	c.Assert(err, jc.ErrorIsNil)

	conf, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.Name(), gc.Equals, "testenv")
	uuid, ok := conf.UUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, gc.Equals, s.modelTag.Id())
}

func (s *ModelSuite) TestConfigForOtherEnv(c *gc.C) {
	otherState := s.Factory.MakeModel(c, &factory.ModelParams{Name: "other"})
	defer otherState.Close()
	otherEnv, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)

	// By getting the model through a different state connection,
	// the underlying state pointer in the *state.Model struct has
	// a different model tag.
	env, err := s.State.GetModel(otherEnv.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	conf, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.Name(), gc.Equals, "other")
	uuid, ok := conf.UUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(uuid, gc.Equals, otherEnv.UUID())
}

// createTestEnvConfig returns a new model config and its UUID for testing.
func (s *ModelSuite) createTestEnvConfig(c *gc.C) (*config.Config, string) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return testing.CustomModelConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	}), uuid.String()
}

func (s *ModelSuite) TestModelConfigSameEnvAsState(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	uuid, exists := cfg.UUID()
	c.Assert(exists, jc.IsTrue)
	c.Assert(uuid, gc.Equals, s.State.ModelUUID())
}

func (s *ModelSuite) TestModelConfigDifferentEnvThanState(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	env, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	uuid, exists := cfg.UUID()
	c.Assert(exists, jc.IsTrue)
	c.Assert(uuid, gc.Equals, env.UUID())
	c.Assert(uuid, gc.Not(gc.Equals), s.State.ModelUUID())
}

func (s *ModelSuite) TestDestroyControllerModel(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyOtherModel(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	env, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyControllerModelFails(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), gc.ErrorMatches, "failed to destroy model: hosting 1 other models")
}

func (s *ModelSuite) TestDestroyControllerAndHostedModels(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	controllerEnv, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerEnv.DestroyIncludingHosted(), jc.ErrorIsNil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	env2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env2.Life(), gc.Equals, state.Dying)

	c.Assert(st2.ProcessDyingModel(), jc.ErrorIsNil)

	c.Assert(env2.Refresh(), jc.ErrorIsNil)
	c.Assert(env2.Life(), gc.Equals, state.Dead)

	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env2.Life(), gc.Equals, state.Dead)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModelsWithResources(c *gc.C) {
	otherSt := s.Factory.MakeModel(c, nil)
	defer otherSt.Close()

	assertEnv := func(env *state.Model, st *state.State, life state.Life, expectedMachines, expectedServices int) {
		c.Assert(env.Refresh(), jc.ErrorIsNil)
		c.Assert(env.Life(), gc.Equals, life)

		machines, err := st.AllMachines()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machines, gc.HasLen, expectedMachines)

		services, err := st.AllServices()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(services, gc.HasLen, expectedServices)
	}

	// add some machines and services
	otherEnv, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherSt.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	service := s.Factory.MakeService(c, &factory.ServiceParams{Creator: otherEnv.Owner()})
	ch, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)

	args := state.AddServiceArgs{
		Name:  service.Name(),
		Owner: service.GetOwnerTag(),
		Charm: ch,
	}
	service, err = otherSt.AddService(args)
	c.Assert(err, jc.ErrorIsNil)

	controllerEnv, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerEnv.DestroyIncludingHosted(), jc.ErrorIsNil)

	assertCleanupRuns(c, s.State)
	assertDoesNotNeedCleanup(c, s.State)
	assertAllMachinesDeadAndRemove(c, s.State)
	assertEnv(controllerEnv, s.State, state.Dying, 0, 0)

	err = s.State.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, `one or more hosted models are not yet dead`)

	assertCleanupCount(c, otherSt, 3)
	assertAllMachinesDeadAndRemove(c, otherSt)
	assertEnv(otherEnv, otherSt, state.Dying, 0, 0)
	c.Assert(otherSt.ProcessDyingModel(), jc.ErrorIsNil)

	c.Assert(otherEnv.Refresh(), jc.ErrorIsNil)
	c.Assert(otherEnv.Life(), gc.Equals, state.Dead)

	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(controllerEnv.Refresh(), jc.ErrorIsNil)
	c.Assert(controllerEnv.Life(), gc.Equals, state.Dead)
}

func (s *ModelSuite) TestDestroyControllerModelRace(c *gc.C) {
	// Simulate an model being added just before the remove txn is
	// called.
	defer state.SetBeforeHooks(c, s.State, func() {
		blocker := s.Factory.MakeModel(c, nil)
		err := blocker.Close()
		c.Check(err, jc.ErrorIsNil)
	}).Check()

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), gc.ErrorMatches, "failed to destroy model: hosting 1 other models")
}

func (s *ModelSuite) TestDestroyControllerAlreadyDyingRaceNoOp(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Simulate an model being destroyed by another client just before
	// the remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(env.Destroy(), jc.ErrorIsNil)
	}).Check()

	c.Assert(env.Destroy(), jc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyControllerAlreadyDyingNoOp(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(env.Destroy(), jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)
}

func (s *ModelSuite) TestProcessDyingServerEnvironTransitionDyingToDead(c *gc.C) {
	s.assertDyingEnvironTransitionDyingToDead(c, s.State)
}

func (s *ModelSuite) TestProcessDyingHostedEnvironTransitionDyingToDead(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	s.assertDyingEnvironTransitionDyingToDead(c, st)
}

func (s *ModelSuite) assertDyingEnvironTransitionDyingToDead(c *gc.C, st *state.State) {
	env, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	// ProcessDyingModel is called by a worker after Destroy is called. To
	// avoid a race, we jump the gun here and test immediately after the
	// environement was set to dead.
	defer state.SetAfterHooks(c, st, func() {
		c.Assert(env.Refresh(), jc.ErrorIsNil)
		c.Assert(env.Life(), gc.Equals, state.Dying)

		c.Assert(st.ProcessDyingModel(), jc.ErrorIsNil)

		c.Assert(env.Refresh(), jc.ErrorIsNil)
		c.Assert(env.Life(), gc.Equals, state.Dead)
	}).Check()

	c.Assert(env.Destroy(), jc.ErrorIsNil)
}

func (s *ModelSuite) TestProcessDyingEnvironWithMachinesAndServicesNoOp(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	// calling ProcessDyingModel on a live environ should fail.
	err := st.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, "model is not dying")

	// add some machines and services
	env, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	service := s.Factory.MakeService(c, &factory.ServiceParams{Creator: env.Owner()})
	ch, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	args := state.AddServiceArgs{
		Name:  service.Name(),
		Owner: service.GetOwnerTag(),
		Charm: ch,
	}
	service, err = st.AddService(args)
	c.Assert(err, jc.ErrorIsNil)

	assertEnv := func(life state.Life, expectedMachines, expectedServices int) {
		c.Assert(env.Refresh(), jc.ErrorIsNil)
		c.Assert(env.Life(), gc.Equals, life)

		machines, err := st.AllMachines()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machines, gc.HasLen, expectedMachines)

		services, err := st.AllServices()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(services, gc.HasLen, expectedServices)
	}

	// Simulate processing a dying envrionment after an envrionment is set to
	// dying, but before the cleanup has removed machines and services.
	defer state.SetAfterHooks(c, st, func() {
		assertEnv(state.Dying, 1, 1)
		err := st.ProcessDyingModel()
		c.Assert(err, gc.ErrorMatches, `model not empty, found 1 machine\(s\)`)
		assertEnv(state.Dying, 1, 1)
	}).Check()

	c.Assert(env.Destroy(), jc.ErrorIsNil)
}

func (s *ModelSuite) TestProcessDyingControllerEnvironWithHostedEnvsNoOp(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	controllerEnv, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerEnv.DestroyIncludingHosted(), jc.ErrorIsNil)

	err = s.State.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, `one or more hosted models are not yet dead`)

	c.Assert(controllerEnv.Refresh(), jc.ErrorIsNil)
	c.Assert(controllerEnv.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestListModelUsers(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	expected := addModelUsers(c, s.State)
	obtained, err := env.Users()
	c.Assert(err, gc.IsNil)

	assertObtainedUsersMatchExpectedUsers(c, obtained, expected)
}

func (s *ModelSuite) TestMisMatchedEnvs(c *gc.C) {
	// create another model
	otherEnvState := s.Factory.MakeModel(c, nil)
	defer otherEnvState.Close()
	otherEnv, err := otherEnvState.Model()
	c.Assert(err, jc.ErrorIsNil)

	// get that model from State
	env, err := s.State.GetModel(otherEnv.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	// check that the Users method errors
	users, err := env.Users()
	c.Assert(users, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot lookup model users outside the current model")
}

func (s *ModelSuite) TestListUsersTwoModels(c *gc.C) {
	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	otherEnvState := s.Factory.MakeModel(c, nil)
	defer otherEnvState.Close()
	otherEnv, err := otherEnvState.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Add users to both models
	expectedUsers := addModelUsers(c, s.State)
	expectedUsersOtherEnv := addModelUsers(c, otherEnvState)

	// test that only the expected users are listed for each model
	obtainedUsers, err := env.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedUsers)

	obtainedUsersOtherEnv, err := otherEnv.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsersOtherEnv, expectedUsersOtherEnv)
}

func addModelUsers(c *gc.C, st *state.State) (expected []*state.ModelUser) {
	// get the model owner
	testAdmin := names.NewUserTag("test-admin")
	owner, err := st.ModelUser(testAdmin)
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(st)
	return []*state.ModelUser{
		// we expect the owner to be an existing model user
		owner,
		// add new users to the model
		f.MakeModelUser(c, nil),
		f.MakeModelUser(c, nil),
		f.MakeModelUser(c, nil),
	}
}

func assertObtainedUsersMatchExpectedUsers(c *gc.C, obtainedUsers, expectedUsers []*state.ModelUser) {
	c.Assert(len(obtainedUsers), gc.Equals, len(expectedUsers))
	for i, obtained := range obtainedUsers {
		c.Assert(obtained.ModelTag().Id(), gc.Equals, expectedUsers[i].ModelTag().Id())
		c.Assert(obtained.UserName(), gc.Equals, expectedUsers[i].UserName())
		c.Assert(obtained.DisplayName(), gc.Equals, expectedUsers[i].DisplayName())
		c.Assert(obtained.CreatedBy(), gc.Equals, expectedUsers[i].CreatedBy())
	}
}

func (s *ModelSuite) TestAllModels(c *gc.C) {
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test", Owner: names.NewUserTag("bob@remote")}).Close()
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "test", Owner: names.NewUserTag("mary@remote")}).Close()
	envs, err := s.State.AllModels()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envs, gc.HasLen, 3)
	var obtained []string
	for _, env := range envs {
		obtained = append(obtained, fmt.Sprintf("%s/%s", env.Owner().Canonical(), env.Name()))
	}
	expected := []string{
		"test-admin@local/testenv",
		"bob@remote/test",
		"mary@remote/test",
	}
	c.Assert(obtained, jc.SameContents, expected)
}

func (s *ModelSuite) TestHostedModelCount(c *gc.C) {
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)

	st1 := s.Factory.MakeModel(c, nil)
	defer st1.Close()
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 1)

	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 2)

	env1, err := st1.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env1.Destroy(), jc.ErrorIsNil)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 1)

	env2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env2.Destroy(), jc.ErrorIsNil)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)
}

func assertCleanupRuns(c *gc.C, st *state.State) {
	err := st.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
}

func assertNeedsCleanup(c *gc.C, st *state.State) {
	actual, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsTrue)
}

func assertDoesNotNeedCleanup(c *gc.C, st *state.State) {
	actual, err := st.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, jc.IsFalse)
}

// assertCleanupCount is useful because certain cleanups cause other cleanups
// to be queued; it makes more sense to just run cleanup again than to unpick
// object destruction so that we run the cleanups inline while running cleanups.
func assertCleanupCount(c *gc.C, st *state.State, count int) {
	for i := 0; i < count; i++ {
		c.Logf("checking cleanups %d", i)
		assertNeedsCleanup(c, st)
		assertCleanupRuns(c, st)
	}
	assertDoesNotNeedCleanup(c, st)
}

// The provisioner will remove dead machines once their backing instances are
// stopped. For the tests, we remove them directly.
func assertAllMachinesDeadAndRemove(c *gc.C, st *state.State) {
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		if _, isContainer := m.ParentId(); isContainer {
			continue
		}
		manual, err := m.IsManual()
		c.Assert(err, jc.ErrorIsNil)
		if manual {
			continue
		}

		c.Assert(m.Life(), gc.Equals, state.Dead)
		c.Assert(m.Remove(), jc.ErrorIsNil)
	}
}
