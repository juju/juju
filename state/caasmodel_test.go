// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing/factory"
)

type CAASFixture struct {
	ConnSuite
}

func (s *CAASFixture) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
}

// createTestModelConfig returns a new model config and its UUID for testing.
func (s *CAASFixture) createTestModelConfig(c *gc.C) (*config.Config, string) {
	return createTestModelConfig(c, s.modelTag.Id())
}

func (s *CAASFixture) newCAASModel(c *gc.C) (*state.CAASModel, *state.State) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")
	model, st, err := s.State.NewModel(state.ModelArgs{
		Type:      state.ModelTypeCAAS,
		CloudName: "dummy",
		Config:    cfg,
		Owner:     owner,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	caasModel, err := model.CAASModel()
	c.Assert(err, jc.ErrorIsNil)
	return caasModel, st
}

type CAASModelSuite struct {
	CAASFixture
}

var _ = gc.Suite(&CAASModelSuite{})

func (s *CAASModelSuite) TestNewModel(c *gc.C) {
	cfg, uuid := s.createTestModelConfig(c)
	modelTag := names.NewModelTag(uuid)
	owner := names.NewUserTag("test@remote")
	model, st, err := s.State.NewModel(state.ModelArgs{
		Type:      state.ModelTypeCAAS,
		CloudName: "dummy",
		Config:    cfg,
		Owner:     owner,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(model.Type(), gc.Equals, state.ModelTypeCAAS)
	c.Assert(model.UUID(), gc.Equals, modelTag.Id())
	c.Assert(model.Tag(), gc.Equals, modelTag)
	c.Assert(model.ControllerTag(), gc.Equals, s.State.ControllerTag())
	c.Assert(model.Owner(), gc.Equals, owner)
	c.Assert(model.Name(), gc.Equals, "testing")
	c.Assert(model.Life(), gc.Equals, state.Alive)
	c.Assert(model.CloudRegion(), gc.Equals, "")
}

func (s *CAASModelSuite) TestModelDestroy(c *gc.C) {
	model, _ := s.newCAASModel(c)
	err := model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	// TODO(caas) - this will be dying when we add cleanup steps.
	c.Assert(model.Life(), gc.Equals, state.Dead)
}

func (s *CAASModelSuite) TestCAASModelsCantHaveCloudRegion(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	_, _, err := s.State.NewModel(state.ModelArgs{
		Type:        state.ModelTypeCAAS,
		CloudName:   "dummy",
		CloudRegion: "fork",
		Config:      cfg,
		Owner:       names.NewUserTag("test@remote"),
	})
	c.Assert(err, gc.ErrorMatches, "CAAS model with CloudRegion not supported")
}

func (s *CAASModelSuite) TestNewModelCAASNeedsFeature(c *gc.C) {
	s.SetFeatureFlags( /* no feature flags */ )
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")
	_, _, err := s.State.NewModel(state.ModelArgs{
		Type:      state.ModelTypeCAAS,
		CloudName: "dummy",
		Config:    cfg,
		Owner:     owner,
	})
	c.Assert(err, gc.ErrorMatches, "model type not supported")
}

func (s *CAASModelSuite) TestNewModelCAASWithStorageRegistry(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	owner := names.NewUserTag("test@remote")
	_, _, err := s.State.NewModel(state.ModelArgs{
		Type:      state.ModelTypeCAAS,
		CloudName: "dummy",
		Config:    cfg,
		Owner:     owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, gc.ErrorMatches, "CAAS model with StorageProviderRegistry not valid")
}

func (s *CAASModelSuite) TestDestroyControllerAndHostedCAASModels(c *gc.C) {
	st2 := s.Factory.MakeModel(c, &factory.ModelParams{
		Type: state.ModelTypeCAAS, CloudRegion: "<none>", StorageProviderRegistry: factory.NilStorageProviderRegistry{}})
	defer st2.Close()
	factory.NewFactory(st2).MakeApplication(c, nil)

	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := true
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		DestroyStorage:      &destroyStorage,
	}), jc.ErrorIsNil)

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)

	assertNeedsCleanup(c, s.State)
	assertCleanupRuns(c, s.State)

	// Cleanups for hosted model enqueued by controller model cleanups.
	assertNeedsCleanup(c, st2)
	assertCleanupRuns(c, st2)

	model2, err := st2.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model2.Life(), gc.Equals, state.Dying)

	c.Assert(st2.ProcessDyingModel(), jc.ErrorIsNil)

	c.Assert(model2.Refresh(), jc.ErrorIsNil)
	c.Assert(model2.Life(), gc.Equals, state.Dead)
	err = st2.RemoveAllModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model2.Life(), gc.Equals, state.Dead)
}

func (s *CAASModelSuite) TestDestroyControllerAndHostedCAASModelsWithResources(c *gc.C) {
	otherSt := s.Factory.MakeModel(c, &factory.ModelParams{
		Type: state.ModelTypeCAAS, CloudRegion: "<none>", StorageProviderRegistry: factory.NilStorageProviderRegistry{}})
	defer otherSt.Close()

	assertModel := func(model *state.Model, st *state.State, life state.Life, expectedApps int) {
		c.Assert(model.Refresh(), jc.ErrorIsNil)
		c.Assert(model.Life(), gc.Equals, life)

		apps, err := st.AllApplications()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(apps, gc.HasLen, expectedApps)
	}

	// add some applications
	otherModel, err := otherSt.Model()
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

	controllerModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	destroyStorage := true
	c.Assert(controllerModel.Destroy(state.DestroyModelParams{
		DestroyHostedModels: true,
		DestroyStorage:      &destroyStorage,
	}), jc.ErrorIsNil)

	assertCleanupCount(c, s.State, 2)
	assertAllMachinesDeadAndRemove(c, s.State)
	assertModel(controllerModel, s.State, state.Dying, 0)

	err = s.State.ProcessDyingModel()
	c.Assert(err, jc.Satisfies, state.IsHasHostedModelsError)
	c.Assert(err, gc.ErrorMatches, `hosting 1 other model`)

	assertCleanupCount(c, otherSt, 2)
	assertModel(otherModel, otherSt, state.Dying, 0)
	c.Assert(otherSt.ProcessDyingModel(), jc.ErrorIsNil)

	c.Assert(otherModel.Refresh(), jc.ErrorIsNil)
	c.Assert(otherModel.Life(), gc.Equals, state.Dead)

	// Until the model is removed, we can't mark the controller model Dead.
	err = s.State.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, `hosting 1 other model`)

	err = otherSt.RemoveAllModelDocs()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(controllerModel.Refresh(), jc.ErrorIsNil)
	c.Assert(controllerModel.Life(), gc.Equals, state.Dead)
}
