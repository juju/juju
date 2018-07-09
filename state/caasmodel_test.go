// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
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
	st := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	caasModel, err := model.CAASModel()
	c.Assert(err, jc.ErrorIsNil)
	return caasModel, st
}

type CAASModelSuite struct {
	CAASFixture
}

var _ = gc.Suite(&CAASModelSuite{})

func (s *CAASModelSuite) TestNewModel(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "caas-cloud",
		Type:      "kubernetes",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)
	cfg, uuid := s.createTestModelConfig(c)
	modelTag := names.NewModelTag(uuid)
	owner := names.NewUserTag("test@remote")
	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	credTag := names.NewCloudCredentialTag(
		fmt.Sprintf("caas-cloud/%s/dummy-credential", owner.Id()))
	err = s.State.UpdateCloudCredential(credTag, cred)
	c.Assert(err, jc.ErrorIsNil)
	model, st, err := s.State.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "caas-cloud",
		Config:                  cfg,
		Owner:                   owner,
		CloudCredential:         credTag,
		StorageProviderRegistry: provider.CommonStorageProviders(),
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

func (s *CAASModelSuite) TestDestroyEmptyModel(c *gc.C) {
	model, _ := s.newCAASModel(c)
	err := model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dead)
}

func (s *CAASModelSuite) TestDestroyModel(c *gc.C) {
	model, st := s.newCAASModel(c)

	f := factory.NewFactory(st)
	ch := f.MakeCharm(c, &factory.CharmParams{Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)

	assertCleanupCount(c, st, 3)
	err = app.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = unit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	assertDoesNotNeedCleanup(c, st)
}

func (s *CAASModelSuite) TestDestroyModelDestroyStorage(c *gc.C) {
	model, st := s.newCAASModel(c)
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(st)
	c.Assert(err, jc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	f := factory.NewFactory(st)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: state.AddTestingCharmForSeries(c, st, "kubernetes", "storage-filesystem"),
			Storage: map[string]state.StorageConstraints{
				"data": {Count: 1, Size: 1024},
			},
		}),
	})

	destroyStorage := true
	err = model.Destroy(state.DestroyModelParams{DestroyStorage: &destroyStorage})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)

	assertNeedsCleanup(c, st)
	assertCleanupCount(c, st, 4)

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)
	fs, err := sb.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fs, gc.HasLen, 0)
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

func (s *CAASModelSuite) TestDestroyControllerAndHostedCAASModels(c *gc.C) {
	st2 := s.Factory.MakeCAASModel(c, nil)
	defer st2.Close()

	f := factory.NewFactory(st2)
	ch := f.MakeCharm(c, &factory.CharmParams{Series: "kubernetes"})
	f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

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
	otherSt := s.Factory.MakeCAASModel(c, nil)
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
	application := s.Factory.MakeApplication(c, nil)
	ch, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)

	args := state.AddApplicationArgs{
		Name:   application.Name(),
		Series: "kubernetes",
		Charm:  ch,
	}
	application, err = otherSt.AddApplication(args)
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

func (s *CAASModelSuite) TestDeployIAASApplication(c *gc.C) {
	_, st := s.newCAASModel(c)
	f := factory.NewFactory(st)
	ch := f.MakeCharm(c, &factory.CharmParams{
		Series: "kubernetes",
	})
	args := state.AddApplicationArgs{
		Name:   "foo",
		Series: "bionic",
		Charm:  ch,
	}
	_, err := st.AddApplication(args)
	c.Assert(err, gc.ErrorMatches, `cannot add application "foo": series "bionic" in a kubernetes model not valid`)
}

func (s *CAASModelSuite) TestContainers(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st)
	ch := f.MakeCharm(c, &factory.CharmParams{
		Series: "kubernetes",
	})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	_, err := app.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id1")})
	c.Assert(err, jc.ErrorIsNil)
	_, err = app.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id2")})
	c.Assert(err, jc.ErrorIsNil)

	containers, err := m.Containers("provider-id1", "provider-id2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containers, gc.HasLen, 2)
	var unitNames []string
	for _, c := range containers {
		unitNames = append(unitNames, c.Unit())
	}
	c.Assert(unitNames, jc.SameContents, []string{app.Name() + "/0", app.Name() + "/1"})
}
