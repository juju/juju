// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type CAASFixture struct {
	ConnSuite
}

func (s *CAASFixture) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.PatchValue(&k8sprovider.NewK8sClients, k8stesting.NoopFakeK8sClients)
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
	cfg, uuid := s.createTestModelConfig(c)
	modelTag := names.NewModelTag(uuid)
	credTag := names.NewCloudCredentialTag(
		fmt.Sprintf("caas-cloud/%s/dummy-credential", owner.Name()))
	model, st, err := s.Controller.NewModel(state.NoopConfigSchemaSource, state.ModelArgs{
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
	model, st := s.newCAASModel(c)
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)
	c.Assert(st.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIs, errors.NotFound)
}

func (s *CAASModelSuite) TestDestroyModel(c *gc.C) {
	model, st := s.newCAASModel(c)

	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)

	assertCleanupCount(c, st, 3)

	// App removal requires cluster resources to be cleared.
	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = app.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	assertCleanupCount(c, st, 2)

	err = app.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	err = unit.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	assertDoesNotNeedCleanup(c, st)
}

func (s *CAASModelSuite) TestDestroyModelDestroyStorage(c *gc.C) {
	model, st := s.newCAASModel(c)
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(
		model,
		&testing.MockCloudService{CloudInfo: &cloud.Cloud{Name: "caascloud", Type: "kubernetes"}},
		&testing.MockCredentialService{Credential: ptr(cloud.NewCredential(cloud.UserPassAuthType, nil))},
	)
	c.Assert(err, jc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	s.policy = testing.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return registry, nil
		},
	}

	sb, err := state.NewStorageBackend(st)
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(st, s.StatePool)
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: state.AddTestingCharmForSeries(c, st, "focal", "storage-filesystem"),
		Storage: map[string]state.StorageConstraints{
			"data": {Count: 1, Size: 1024},
		},
	})
	unit := f.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})

	si, err := sb.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si, gc.HasLen, 1)
	fs, err := sb.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fs, gc.HasLen, 1)

	destroyStorage := true
	err = model.Destroy(state.DestroyModelParams{DestroyStorage: &destroyStorage})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)

	assertNeedsCleanup(c, st)
	assertCleanupCount(c, st, 4)

	c.Assert(app.Refresh(), jc.ErrorIsNil)
	c.Assert(app.Life(), gc.Equals, state.Dying)
	c.Assert(unit.Refresh(), jc.ErrorIsNil)
	c.Assert(unit.Life(), gc.Equals, state.Dying)

	// The uniter would call this when it sees it is dying.
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	// The deployer or the caasapplicationprovisioner would call this once the unit is Dead.
	err = unit.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	assertNeedsCleanup(c, st)
	assertCleanupCount(c, st, 2)

	// The caasapplicationprovisioner would call this when the app is gone from the cloud.
	err = app.ClearResources()
	c.Assert(err, jc.ErrorIsNil)

	assertNeedsCleanup(c, st)
	assertCleanupCount(c, st, 2)

	si, err = sb.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(si, gc.HasLen, 0)
	fs, err = sb.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fs, gc.HasLen, 0)

	vols, err := sb.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, gc.HasLen, 1)
	c.Assert(vols[0].Life(), gc.Equals, state.Dying)
	// A storage provisioner would call this.
	err = sb.RemoveVolumeAttachment(unit.UnitTag(), vols[0].VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.RemoveVolume(vols[0].VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	// Undertaker would call this.
	err = st.ProcessDyingModel()
	c.Assert(err, jc.ErrorIsNil)
	err = st.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CAASModelSuite) TestDestroyControllerAndHostedCAASModels(c *gc.C) {
	st2 := s.Factory.MakeCAASModel(c, nil)
	defer st2.Close()

	f := factory.NewFactory(st2, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

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

	// App removal requires cluster resources to be cleared.
	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = app.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	assertCleanupCount(c, st2, 2)

	c.Assert(st2.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(st2.RemoveDyingModel(), jc.ErrorIsNil)

	c.Assert(model2.Refresh(), jc.ErrorIs, errors.NotFound)

	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(s.State.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIs, errors.NotFound)
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
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"})
	args := state.AddApplicationArgs{
		Name: application.Name(),
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm: ch,
	}
	application2, err := otherSt.AddApplication(defaultInstancePrechecker, args, state.NewObjectStore(c, otherSt.ModelUUID()))
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
	c.Assert(err, jc.ErrorIs, stateerrors.HasHostedModelsError)
	c.Assert(err, gc.ErrorMatches, `hosting 1 other model`)

	assertCleanupCount(c, otherSt, 2)

	// App removal requires cluster resources to be cleared.
	err = application2.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = application2.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	assertCleanupCount(c, otherSt, 2)

	assertModel(otherModel, otherSt, state.Dying, 0)
	c.Assert(otherSt.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(otherSt.RemoveDyingModel(), jc.ErrorIsNil)

	c.Assert(otherModel.Refresh(), jc.ErrorIs, errors.NotFound)

	c.Assert(s.State.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(s.State.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(controllerModel.Refresh(), jc.ErrorIs, errors.NotFound)
}

func (s *CAASModelSuite) TestContainers(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{
		Name:   "gitlab-k8s",
		Series: "focal",
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

func (s *CAASModelSuite) TestUnitStatus(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	app := f.MakeApplication(c, nil)
	unit := f.MakeUnit(c, &factory.UnitParams{
		Application: app,
		Status: &status.StatusInfo{
			Status:  status.Waiting,
			Message: status.MessageInitializingAgent,
		},
	})

	msWorkload := unitWorkloadStatus(c, m, unit.Name())
	c.Check(msWorkload.Message, gc.Equals, "agent initialising")
	c.Check(msWorkload.Status, gc.Equals, status.Waiting)

	err := unit.SetStatus(status.StatusInfo{Status: status.Active, Message: "running"}, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	msWorkload = unitWorkloadStatus(c, m, unit.Name())
	c.Check(msWorkload.Message, gc.Equals, "running")
	c.Check(msWorkload.Status, gc.Equals, status.Active)
}

func (s *CAASModelSuite) TestCloudContainerStatus(c *gc.C) {
	m, st := s.newCAASModel(c)
	f := factory.NewFactory(st, s.StatePool)
	app := f.MakeApplication(c, nil)
	unit := f.MakeUnit(c, &factory.UnitParams{
		Application: app,
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
