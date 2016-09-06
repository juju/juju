// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
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
	c.Assert(model.ControllerTag(), gc.Equals, s.State.ControllerTag())
	c.Assert(model.Name(), gc.Equals, "testenv")
	c.Assert(model.Owner(), gc.Equals, s.Owner)
	c.Assert(model.Life(), gc.Equals, state.Alive)
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeNone)
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
}

func (s *ModelSuite) TestNewModelNonExistentLocalUser(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("non-existent@local")

	_, _, err := s.State.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, `cannot create model: user "non-existent" not found`)
}

func (s *ModelSuite) TestNewModelSameUserSameNameFails(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := s.Factory.MakeUser(c, nil).UserTag()

	// Create the first model.
	_, st1, err := s.State.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
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
	_, _, err = s.State.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg2,
		Owner:       owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
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
	env2, st2, err := s.State.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg2,
		Owner:       owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st2.Close()
	c.Assert(env2, gc.NotNil)
	c.Assert(st2, gc.NotNil)
}

func (s *ModelSuite) TestNewModel(c *gc.C) {
	cfg, uuid := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	model, st, err := s.State.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	modelTag := names.NewModelTag(uuid)
	assertModelMatches := func(model *state.Model) {
		c.Assert(model.UUID(), gc.Equals, modelTag.Id())
		c.Assert(model.Tag(), gc.Equals, modelTag)
		c.Assert(model.ControllerTag(), gc.Equals, s.State.ControllerTag())
		c.Assert(model.Owner(), gc.Equals, owner)
		c.Assert(model.Name(), gc.Equals, "testing")
		c.Assert(model.Life(), gc.Equals, state.Alive)
	}
	assertModelMatches(model)

	// Since the model tag for the State connection is different,
	// asking for this model through FindEntity returns a not found error.
	model, err = s.State.GetModel(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	assertModelMatches(model)

	model, err = st.Model()
	c.Assert(err, jc.ErrorIsNil)
	assertModelMatches(model)

	_, err = s.State.FindEntity(modelTag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	entity, err := st.FindEntity(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, modelTag)

	// Ensure the model is functional by adding a machine
	_, err = st.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSuite) TestNewModelImportingMode(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	env, st, err := s.State.NewModel(state.ModelArgs{
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		MigrationMode:           state.MigrationModeImporting,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(env.MigrationMode(), gc.Equals, state.MigrationModeImporting)
}

func (s *ModelSuite) TestSetMigrationMode(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")

	env, st, err := s.State.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = env.SetMigrationMode(state.MigrationModeExporting)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.MigrationMode(), gc.Equals, state.MigrationModeExporting)
}

func (s *ModelSuite) TestControllerModel(c *gc.C) {
	model, err := s.State.ControllerModel()
	c.Assert(err, jc.ErrorIsNil)

	expectedTag := names.NewModelTag(model.UUID())
	c.Assert(model.Tag(), gc.Equals, expectedTag)
	c.Assert(model.ControllerTag(), gc.Equals, s.State.ControllerTag())
	c.Assert(model.Name(), gc.Equals, "testenv")
	c.Assert(model.Owner(), gc.Equals, s.Owner)
	c.Assert(model.Life(), gc.Equals, state.Alive)
}

func (s *ModelSuite) TestControllerModelAccessibleFromOtherModels(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	_, st, err := s.State.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       names.NewUserTag("test@remote"),
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(conf.UUID(), gc.Equals, s.modelTag.Id())
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
	c.Assert(conf.UUID(), gc.Equals, otherEnv.UUID())
}

// createTestModelConfig returns a new model config and its UUID for testing.
func (s *ModelSuite) createTestModelConfig(c *gc.C) (*config.Config, string) {
	return createTestModelConfig(c, s.modelTag.Id())
}

func createTestModelConfig(c *gc.C, controllerUUID string) (*config.Config, string) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	if controllerUUID == "" {
		controllerUUID = uuid.String()
	}
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
	c.Assert(cfg.UUID(), gc.Equals, s.State.ModelUUID())
}

func (s *ModelSuite) TestModelConfigDifferentEnvThanState(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()
	env, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := env.Config()
	c.Assert(err, jc.ErrorIsNil)
	uuid := cfg.UUID()
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

func (s *ModelSuite) TestDestroyControllerNonEmptyModelFails(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	factory.NewFactory(st2).MakeApplication(c, nil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), gc.ErrorMatches, "failed to destroy model: hosting 1 other models")
}

func (s *ModelSuite) TestDestroyControllerEmptyModel(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerModel.Destroy(), jc.ErrorIsNil)
	c.Assert(controllerModel.Refresh(), jc.ErrorIsNil)
	c.Assert(controllerModel.Life(), gc.Equals, state.Dying)

	hostedModel, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostedModel.Life(), gc.Equals, state.Dead)
}

func (s *ModelSuite) TestDestroyControllerAndHostedModels(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()
	factory.NewFactory(st2).MakeApplication(c, nil)

	controllerEnv, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerEnv.DestroyIncludingHosted(), jc.ErrorIsNil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Life(), gc.Equals, state.Dying)

	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	// Cleanups for hosted model enqueued by controller model cleanups.
	assertNeedsCleanup(c, st2)
	assertCleanupRuns(c, st2)

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

		services, err := st.AllApplications()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(services, gc.HasLen, expectedServices)
	}

	// add some machines and services
	otherEnv, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherSt.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	service := s.Factory.MakeApplication(c, nil)
	ch, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)

	args := state.AddApplicationArgs{
		Name:  service.Name(),
		Charm: ch,
	}
	service, err = otherSt.AddApplication(args)
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

func (s *ModelSuite) TestDestroyControllerEmptyModelRace(c *gc.C) {
	defer s.Factory.MakeModel(c, nil).Close()

	// Simulate an empty model being added just before the
	// remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		s.Factory.MakeModel(c, nil).Close()
	}).Check()

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)
}

func (s *ModelSuite) TestDestroyControllerRemoveEmptyAddNonEmptyModel(c *gc.C) {
	st2 := s.Factory.MakeModel(c, nil)
	defer st2.Close()

	// Simulate an empty model being removed, and a new non-empty
	// model being added, just before the remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		// Destroy the empty model, which should move it right
		// along to Dead, and then remove it.
		model, err := st2.Model()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(model.Destroy(), jc.ErrorIsNil)
		err = st2.RemoveAllModelDocs()
		c.Assert(err, jc.ErrorIsNil)

		// Add a new, non-empty model. This should still prevent
		// the controller from being destroyed.
		st3 := s.Factory.MakeModel(c, nil)
		defer st3.Close()
		factory.NewFactory(st3).MakeApplication(c, nil)
	}).Check()

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Destroy(), gc.ErrorMatches, "failed to destroy model: hosting 1 other models")
}

func (s *ModelSuite) TestDestroyControllerNonEmptyModelRace(c *gc.C) {
	// Simulate an empty model being added just before the
	// remove txn is called.
	defer state.SetBeforeHooks(c, s.State, func() {
		st := s.Factory.MakeModel(c, nil)
		defer st.Close()
		factory.NewFactory(st).MakeApplication(c, nil)
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

func (s *ModelSuite) TestDestroyModelNonEmpty(c *gc.C) {
	m, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Add a service to prevent the model from transitioning directly to Dead.
	s.Factory.MakeApplication(c, nil)

	c.Assert(m.Destroy(), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelAddServiceConcurrently(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, st, func() {
		factory.NewFactory(st).MakeApplication(c, nil)
	}).Check()

	c.Assert(m.Destroy(), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelAddMachineConcurrently(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, st, func() {
		factory.NewFactory(st).MakeMachine(c, nil)
	}).Check()

	c.Assert(m.Destroy(), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dying)
}

func (s *ModelSuite) TestDestroyModelEmpty(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(m.Destroy(), jc.ErrorIsNil)
	c.Assert(m.Refresh(), jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
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
	// Add a service to prevent the model from transitioning directly to Dead.
	// Add the service before getting the Model, otherwise we'll have to run
	// the transaction twice, and hit the hook point too early.
	svc := factory.NewFactory(st).MakeApplication(c, nil)
	env, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	// ProcessDyingModel is called by a worker after Destroy is called. To
	// avoid a race, we jump the gun here and test immediately after the
	// environement was set to dead.
	defer state.SetAfterHooks(c, st, func() {
		c.Assert(env.Refresh(), jc.ErrorIsNil)
		c.Assert(env.Life(), gc.Equals, state.Dying)

		err := svc.Destroy()
		c.Assert(err, jc.ErrorIsNil)

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
	service := s.Factory.MakeApplication(c, nil)
	ch, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	args := state.AddApplicationArgs{
		Name:  service.Name(),
		Charm: ch,
	}
	service, err = st.AddApplication(args)
	c.Assert(err, jc.ErrorIsNil)

	assertEnv := func(life state.Life, expectedMachines, expectedServices int) {
		c.Assert(env.Refresh(), jc.ErrorIsNil)
		c.Assert(env.Life(), gc.Equals, life)

		machines, err := st.AllMachines()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(machines, gc.HasLen, expectedMachines)

		services, err := st.AllApplications()
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

	c.Assert(env.Refresh(), jc.ErrorIsNil)
	c.Assert(env.Destroy(), jc.ErrorIsNil)
}

func (s *ModelSuite) TestProcessDyingControllerEnvironWithHostedEnvsNoOp(c *gc.C) {
	// Add a non-empty model to the controller.
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	factory.NewFactory(st).MakeApplication(c, nil)

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

func (s *ModelSuite) TestListUsersIgnoredDeletedUsers(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	expectedUsers := addModelUsers(c, s.State)

	obtainedUsers, err := model.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedUsers)

	lastUser := obtainedUsers[len(obtainedUsers)-1]
	err = s.State.RemoveUser(lastUser.UserTag)
	c.Assert(err, jc.ErrorIsNil)
	expectedAfterDeletion := obtainedUsers[:len(obtainedUsers)-1]

	obtainedUsers, err = model.Users()
	c.Assert(err, jc.ErrorIsNil)
	assertObtainedUsersMatchExpectedUsers(c, obtainedUsers, expectedAfterDeletion)
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

func addModelUsers(c *gc.C, st *state.State) (expected []description.UserAccess) {
	// get the model owner
	testAdmin := names.NewUserTag("test-admin")
	owner, err := st.UserAccess(testAdmin, st.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(st)
	return []description.UserAccess{
		// we expect the owner to be an existing model user
		owner,
		// add new users to the model
		f.MakeModelUser(c, nil),
		f.MakeModelUser(c, nil),
		f.MakeModelUser(c, nil),
	}
}

func assertObtainedUsersMatchExpectedUsers(c *gc.C, obtainedUsers, expectedUsers []description.UserAccess) {
	c.Assert(len(obtainedUsers), gc.Equals, len(expectedUsers))
	for i, obtained := range obtainedUsers {
		c.Assert(obtained.Object.Id(), gc.Equals, expectedUsers[i].Object.Id())
		c.Assert(obtained.UserTag, gc.Equals, expectedUsers[i].UserTag)
		c.Assert(obtained.DisplayName, gc.Equals, expectedUsers[i].DisplayName)
		c.Assert(obtained.CreatedBy, gc.Equals, expectedUsers[i].CreatedBy)
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
	c.Assert(st1.RemoveAllModelDocs(), jc.ErrorIsNil)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 1)

	env2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env2.Destroy(), jc.ErrorIsNil)
	c.Assert(st2.RemoveAllModelDocs(), jc.ErrorIsNil)
	c.Assert(state.HostedModelCount(c, s.State), gc.Equals, 0)
}

type ModelCloudValidationSuite struct {
	gitjujutesting.MgoSuite
}

var _ = gc.Suite(&ModelCloudValidationSuite{})

// TODO(axw) concurrency tests when we can modify the cloud definition,
// and update/remove credentials.

func (s *ModelCloudValidationSuite) TestNewModelCloudNameMismatch(c *gc.C) {
	st, owner := s.initializeState(c, []cloud.Region{{Name: "some-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer st.Close()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := st.NewModel(state.ModelArgs{
		CloudName: "another",
		Config:    cfg,
		Owner:     owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, "controller cloud dummy does not match model cloud another")
}

func (s *ModelCloudValidationSuite) TestNewModelUnknownCloudRegion(c *gc.C) {
	st, owner := s.initializeState(c, []cloud.Region{{Name: "some-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer st.Close()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := st.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, `region "dummy-region" not found \(expected one of \["some-region"\]\)`)
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudRegion(c *gc.C) {
	st, owner := s.initializeState(c, []cloud.Region{{Name: "dummy-region"}}, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer st.Close()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := st.NewModel(state.ModelArgs{
		CloudName: "dummy",
		Config:    cfg,
		Owner:     owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, "missing CloudRegion not valid")
}

func (s *ModelCloudValidationSuite) TestNewModelUnknownCloudCredential(c *gc.C) {
	regions := []cloud.Region{cloud.Region{Name: "dummy-region"}}
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	st, owner := s.initializeState(
		c, regions, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer st.Close()
	unknownCredentialTag := names.NewCloudCredentialTag("dummy/" + owner.Canonical() + "/unknown-credential")
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := st.NewModel(state.ModelArgs{
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		CloudCredential:         unknownCredentialTag,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, `credential "dummy/test@remote/unknown-credential" not found`)
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudCredential(c *gc.C) {
	regions := []cloud.Region{cloud.Region{Name: "dummy-region"}}
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	st, owner := s.initializeState(
		c, regions, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer st.Close()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := st.NewModel(state.ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, "missing CloudCredential not valid")
}

func (s *ModelCloudValidationSuite) TestNewModelMissingCloudCredentialSupportsEmptyAuth(c *gc.C) {
	regions := []cloud.Region{
		cloud.Region{
			Name:             "dummy-region",
			Endpoint:         "dummy-endpoint",
			IdentityEndpoint: "dummy-identity-endpoint",
			StorageEndpoint:  "dummy-storage-endpoint",
		},
	}
	st, owner := s.initializeState(c, regions, []cloud.AuthType{cloud.EmptyAuthType}, nil)
	defer st.Close()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	cfg, err := cfg.Apply(map[string]interface{}{"name": "whatever"})
	c.Assert(err, jc.ErrorIsNil)
	_, newSt, err := st.NewModel(state.ModelArgs{
		CloudName: "dummy", CloudRegion: "dummy-region", Config: cfg, Owner: owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	newSt.Close()
}

func (s *ModelCloudValidationSuite) TestNewModelOtherUserCloudCredential(c *gc.C) {
	controllerCredentialTag := names.NewCloudCredentialTag("dummy/test@remote/controller-credential")
	st, _ := s.initializeState(
		c, nil, []cloud.AuthType{cloud.UserPassAuthType}, map[names.CloudCredentialTag]cloud.Credential{
			controllerCredentialTag: cloud.NewCredential(cloud.UserPassAuthType, nil),
		},
	)
	defer st.Close()
	owner := factory.NewFactory(st).MakeUser(c, nil).UserTag()
	cfg, _ := createTestModelConfig(c, st.ModelUUID())
	_, _, err := st.NewModel(state.ModelArgs{
		CloudName:               "dummy",
		Config:                  cfg,
		Owner:                   owner,
		CloudCredential:         controllerCredentialTag,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, `credential "dummy/test@remote/controller-credential" not found`)
}

func (s *ModelCloudValidationSuite) initializeState(
	c *gc.C,
	regions []cloud.Region,
	authTypes []cloud.AuthType,
	credentials map[names.CloudCredentialTag]cloud.Credential,
) (*state.State, names.UserTag) {
	owner := names.NewUserTag("test@remote")
	cfg, _ := createTestModelConfig(c, "")
	var controllerRegion string
	var controllerCredential names.CloudCredentialTag
	if len(regions) > 0 {
		controllerRegion = regions[0].Name
	}
	if len(credentials) > 0 {
		// pick an arbitrary credential
		for controllerCredential = range credentials {
		}
	}
	controllerCfg := testing.FakeControllerConfig()
	st, err := state.Initialize(state.InitializeParams{
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Owner:                   owner,
			Config:                  cfg,
			CloudName:               "dummy",
			CloudRegion:             controllerRegion,
			CloudCredential:         controllerCredential,
			StorageProviderRegistry: storage.StaticProviderRegistry{},
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: authTypes,
			Regions:   regions,
		},
		CloudCredentials: credentials,
		MongoInfo:        statetesting.NewMongoInfo(),
		MongoDialOpts:    mongotest.DialOpts(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return st, owner
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
