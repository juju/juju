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
	"github.com/juju/juju/core/status"
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
	owner := s.Factory.MakeUser(c, nil)
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "caas-cloud",
		Type:      "kubernetes",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}, owner.Name())
	c.Assert(err, jc.ErrorIsNil)
	cfg, uuid := s.createTestModelConfig(c)
	modelTag := names.NewModelTag(uuid)
	cred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	credTag := names.NewCloudCredentialTag(
		fmt.Sprintf("caas-cloud/%s/dummy-credential", owner.Name()))
	err = s.State.UpdateCloudCredential(credTag, cred)
	c.Assert(err, jc.ErrorIsNil)
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               "caas-cloud",
		Config:                  cfg,
		Owner:                   owner.UserTag(),
		CloudCredential:         credTag,
		StorageProviderRegistry: provider.CommonStorageProviders(),
	})
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	c.Assert(model.Type(), gc.Equals, state.ModelTypeCAAS)
	c.Assert(model.UUID(), gc.Equals, modelTag.Id())
	c.Assert(model.Tag(), gc.Equals, modelTag)
	c.Assert(model.ControllerTag(), gc.Equals, s.State.ControllerTag())
	c.Assert(model.Owner().Name(), gc.Equals, owner.Name())
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

	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
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

	f := factory.NewFactory(st, s.StatePool)
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
	assertCleanupCount(c, st, 5)

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)
	fs, err := sb.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fs, gc.HasLen, 0)
}

func (s *CAASModelSuite) TestCAASModelsCantHaveCloudRegion(c *gc.C) {
	cfg, _ := s.createTestModelConfig(c)
	_, _, err := s.Controller.NewModel(state.ModelArgs{
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

	f := factory.NewFactory(st2, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
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
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab"})
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(otherSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
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
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{
		Name:   "gitlab",
		Series: "kubernetes",
	})
	args := state.AddApplicationArgs{
		Name:   "gitlab",
		Series: "bionic",
		Charm:  ch,
	}
	_, err := st.AddApplication(args)
	c.Assert(err, gc.ErrorMatches, `cannot add application "gitlab": series "bionic" in a kubernetes model not valid`)
}

func (s *CAASModelSuite) TestContainers(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{
		Name:   "gitlab",
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

func (s *CAASModelSuite) TestCloudContainerStatus(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	unit := f.MakeUnit(c, &factory.UnitParams{
		Status: &status.StatusInfo{
			Status:  status.Active,
			Message: "Unit Active",
		},
	})

	// Cloud container overrides Allocating unit
	setCloudContainerStatus(c, unit, status.Allocating, "k8s allocating")
	msWorkload := unitWorkloadStatus(c, m, unit.Name())
	c.Check(msWorkload.Message, gc.Equals, "k8s allocating")
	c.Check(msWorkload.Status, gc.Equals, status.Allocating)

	// Cloud container error overrides unit status
	setCloudContainerStatus(c, unit, status.Error, "k8s charm error")
	msWorkload = unitWorkloadStatus(c, m, unit.Name())
	c.Check(msWorkload.Message, gc.Equals, "k8s charm error")
	c.Check(msWorkload.Status, gc.Equals, status.Error)

	// Unit status must be used.
	setCloudContainerStatus(c, unit, status.Running, "k8s idle")
	msWorkload = unitWorkloadStatus(c, m, unit.Name())
	c.Check(msWorkload.Message, gc.Equals, "Unit Active")
	c.Check(msWorkload.Status, gc.Equals, status.Active)

	// Cloud container overrides
	setCloudContainerStatus(c, unit, status.Blocked, "POD storage issue")
	msWorkload = unitWorkloadStatus(c, m, unit.Name())
	c.Check(msWorkload.Message, gc.Equals, "POD storage issue")
	c.Check(msWorkload.Status, gc.Equals, status.Blocked)

	// Cloud container overrides
	setCloudContainerStatus(c, unit, status.Waiting, "Building the bits")
	msWorkload = unitWorkloadStatus(c, m, unit.Name())
	c.Check(msWorkload.Message, gc.Equals, "Building the bits")
	c.Check(msWorkload.Status, gc.Equals, status.Waiting)

	// Cloud container overrides
	setCloudContainerStatus(c, unit, status.Running, "Bits have been built")
	msWorkload = unitWorkloadStatus(c, m, unit.Name())
	c.Check(msWorkload.Message, gc.Equals, "Unit Active")
	c.Check(msWorkload.Status, gc.Equals, status.Active)
}

func (s *CAASModelSuite) TestCloudContainerHistoryOverwrite(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	unit := f.MakeUnit(c, &factory.UnitParams{})

	workloadStatus := unitWorkloadStatus(c, m, unit.Name())
	c.Assert(workloadStatus.Message, gc.Equals, status.MessageWaitForContainer)
	c.Assert(workloadStatus.Status, gc.Equals, status.Waiting)
	statusHistory, err := unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusHistory, gc.HasLen, 1)
	c.Assert(statusHistory[0].Message, gc.Equals, status.MessageWaitForContainer)
	c.Assert(statusHistory[0].Status, gc.Equals, status.Waiting)

	unit.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "Unit Active",
	})
	unitStatus, err := unit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitStatus.Message, gc.Equals, "Unit Active")
	c.Assert(unitStatus.Status, gc.Equals, status.Active)

	// Now that status is stored as Active, but displayed (and in history)
	// as waiting for container, once we set cloud container status as active
	// it must show active from the unit (incl. history)
	setCloudContainerStatus(c, unit, status.Running, "Container Active")
	workloadStatus = unitWorkloadStatus(c, m, unit.Name())
	c.Assert(workloadStatus.Message, gc.Equals, "Unit Active")
	c.Assert(workloadStatus.Status, gc.Equals, status.Active)
	statusHistory, err = unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusHistory, gc.HasLen, 2)
	c.Assert(statusHistory[0].Message, gc.Equals, "Unit Active")
	c.Assert(statusHistory[0].Status, gc.Equals, status.Active)
	c.Assert(statusHistory[1].Message, gc.Equals, status.MessageWaitForContainer)
	c.Assert(statusHistory[1].Status, gc.Equals, status.Waiting)

	unit.SetStatus(status.StatusInfo{
		Status:  status.Waiting,
		Message: "This is a different message",
	})
	workloadStatus = unitWorkloadStatus(c, m, unit.Name())
	c.Assert(workloadStatus.Message, gc.Equals, "This is a different message")
	c.Assert(workloadStatus.Status, gc.Equals, status.Waiting)
	statusHistory, err = unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusHistory, gc.HasLen, 3)
	c.Assert(statusHistory[0].Message, gc.Equals, "This is a different message")
	c.Assert(statusHistory[0].Status, gc.Equals, status.Waiting)
	c.Assert(statusHistory[1].Message, gc.Equals, "Unit Active")
	c.Assert(statusHistory[1].Status, gc.Equals, status.Active)
	c.Assert(statusHistory[2].Message, gc.Equals, status.MessageWaitForContainer)
	c.Assert(statusHistory[2].Status, gc.Equals, status.Waiting)
}

func unitWorkloadStatus(c *gc.C, model *state.CAASModel, unitName string) status.StatusInfo {
	ms, err := model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)
	msWorkload, err := ms.UnitWorkload(unitName)
	c.Assert(err, jc.ErrorIsNil)
	return msWorkload
}

func setCloudContainerStatus(c *gc.C, unit *state.Unit, statusCode status.Status, message string) {
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		unit.UpdateOperation(state.UnitUpdateProperties{
			CloudContainerStatus: &status.StatusInfo{Status: statusCode, Message: message},
		})}
	app, err := unit.Application()
	c.Assert(err, jc.ErrorIsNil)
	err = app.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CAASModelSuite) TestApplicationOperatorStatusOverwriteError(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	f.MakeUnit(c, &factory.UnitParams{})
	ch := f.MakeCharm(c, &factory.CharmParams{
		Name:   "gitlab",
		Series: "kubernetes",
	})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})
	app.SetOperatorStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "operator error",
	})
	app.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "app active",
	})
	ms, err := m.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := ms.Application("gitlab", []string{"gitlab/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appStatus.Message, gc.Equals, "operator error")
	c.Check(appStatus.Status, gc.Equals, status.Error)
}

func (s *CAASModelSuite) TestApplicationOperatorStatusOverwriteRunning(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	f.MakeUnit(c, &factory.UnitParams{})
	ch := f.MakeCharm(c, &factory.CharmParams{
		Name:   "gitlab",
		Series: "kubernetes",
	})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	app.SetOperatorStatus(status.StatusInfo{
		Status:  status.Running,
		Message: "operator running",
	})
	app.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "app active",
	})
	ms, err := m.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := ms.Application("gitlab", []string{"gitlab/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appStatus.Message, gc.Equals, "app active")
	c.Check(appStatus.Status, gc.Equals, status.Active)
}
