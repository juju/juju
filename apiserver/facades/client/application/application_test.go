// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"fmt"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	unitassignerapi "github.com/juju/juju/api/agent/unitassigner"
	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/client/application"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testcharms"
)

type applicationSuite struct {
	jujutesting.ApiServerSuite
	commontesting.BlockHelper

	applicationAPI *application.APIBase
	application    *state.Application
	authorizer     *apiservertesting.FakeAuthorizer
	lastKnownRev   map[string]int

	store              objectstore.ObjectStore
	networkService     *application.MockNetworkService
	modelConfigService *application.MockModelConfigService
	modelAgentService  *application.MockModelAgentService
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelConfigService = application.NewMockModelConfigService(ctrl)
	s.modelAgentService = application.NewMockModelAgentService(ctrl)

	s.networkService = application.NewMockNetworkService(ctrl)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{
		{ID: "0", Name: "alpha"},
	}, nil).AnyTimes()

	return ctrl
}

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.OpenControllerModelAPI(c))
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.application = f.MakeApplication(c, nil)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: jujutesting.AdminUser,
	}
	s.lastKnownRev = make(map[string]int)

	s.store = jujutesting.NewObjectStore(c, s.ControllerModelUUID())
}

func (s *applicationSuite) makeAPI(c *gc.C) {
	st := s.ControllerModel(c).State()
	storageAccess, err := application.GetStorageState(st)
	c.Assert(err, jc.ErrorIsNil)
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	blockChecker := common.NewBlockChecker(st)

	serviceFactory := s.DefaultModelServiceFactory(c)

	envFunc := stateenvirons.GetNewEnvironFunc(environs.New)
	env, err := envFunc(s.ControllerModel(c), serviceFactory.Cloud(), serviceFactory.Credential())
	c.Assert(err, jc.ErrorIsNil)

	s.InstancePrechecker = func(c *gc.C, st *state.State) environs.InstancePrechecker {
		return env
	}

	modelInfo := model.ReadOnlyModel{
		UUID: model.UUID(testing.ModelTag.Id()),
		Type: model.IAAS,
	}
	registry := stateenvirons.NewStorageProviderRegistry(env)
	serviceFactoryGetter := s.ServiceFactoryGetter(c)
	storageService := serviceFactoryGetter.FactoryForModel(model.UUID(st.ModelUUID())).Storage(registry)
	api, err := application.NewAPIBase(
		application.GetState(st, env),
		nil,
		s.networkService,
		storageAccess,
		s.authorizer,
		nil,
		blockChecker,
		application.GetModel(m),
		modelInfo,
		s.modelConfigService,
		s.modelAgentService,
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
		serviceFactory.Machine(),
		serviceFactory.Application(registry),
		nil, // leadership not used in these tests.
		application.CharmToStateCharm,
		application.DeployApplication,
		storageService,
		registry,
		common.NewResources(),
		nil, // CAAS Broker not used in this suite.
		jujutesting.NewObjectStore(c, st.ModelUUID()),
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.applicationAPI = api
}

func (s *applicationSuite) setupApplicationDeploy(c *gc.C, args string) (string, charm.Charm, constraints.Value) {
	curl, ch := s.addCharmToState(c, "ch:jammy/dummy-42", "dummy")
	cons := constraints.MustParse(args)
	return curl, ch, cons
}

func (s *applicationSuite) assertApplicationDeployPrincipal(c *gc.C, curl string, ch charm.Charm, mem4g constraints.Value) {
	results, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application",
			NumUnits:        3,
			Constraints:     mem4g,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	apiservertesting.AssertPrincipalApplicationDeployed(c, s.ControllerModel(c).State(), "application", curl, false, ch, mem4g)
}

func (s *applicationSuite) assertApplicationDeployPrincipalBlocked(c *gc.C, msg string, curl string, mem4g constraints.Value) {
	_, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application",
			NumUnits:        3,
			Constraints:     mem4g,
		}}})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroyApplicationDeployPrincipal(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	curl, bundle, cons := s.setupApplicationDeploy(c, "arch=amd64 mem=4G")
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipal(c, curl, bundle, cons)
}

func (s *applicationSuite) TestBlockRemoveApplicationDeployPrincipal(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	curl, bundle, cons := s.setupApplicationDeploy(c, "arch=amd64 mem=4G")
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipal(c, curl, bundle, cons)
}

func (s *applicationSuite) TestBlockChangesApplicationDeployPrincipal(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	curl, _, cons := s.setupApplicationDeploy(c, "mem=4G")
	s.BlockAllChanges(c, "TestBlockChangesApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipalBlocked(c, "TestBlockChangesApplicationDeployPrincipal", curl, cons)
}

func (s *applicationSuite) TestApplicationDeploySubordinate(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	curl, ch := s.addCharmToState(c, "ch:utopic/logging-47", "logging")
	results, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	app, err := s.ControllerModel(c).State().Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, ch.Config())

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
}

func (s *applicationSuite) combinedSettings(ch *state.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

func (s *applicationSuite) TestApplicationDeployConfig(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	curl, _ := s.addCharmToState(c, "ch:jammy/dummy-0", "dummy")
	results, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  username: fred",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	app, err := s.ControllerModel(c).State().Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := app.CharmConfig(model.GenerationMaster)
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{"username": "fred"}))
}

func (s *applicationSuite) TestApplicationDeployConfigError(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	curl, _ := s.addCharmToState(c, "ch:jammy/dummy-0", "dummy")
	results, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  skill-level: fred",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `.*option "skill-level" expected int, got "fred"`)
	_, err = s.ControllerModel(c).State().Application("application-name")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *applicationSuite) TestApplicationDeployToMachine(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	curl, ch := s.addCharmToState(c, "ch:jammy/dummy-0", "dummy")

	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(s.InstancePrechecker(c, st), state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	arch := arch.DefaultArchitecture
	hwChar := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	instId := instance.Id("i-host-machine")
	err = machine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  username: fred",
			Placement:       []*instance.Placement{instance.MustParsePlacement("0")},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	app, err := st.Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, ch.Config())

	errs, err := unitassignerapi.New(s.OpenControllerModelAPI(c)).AssignUnits(context.Background(), []names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToMachineWithLXDProfile(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	curl, ch := s.addCharmToState(c, "ch:jammy/lxd-profile-0", "lxd-profile")

	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(s.InstancePrechecker(c, st), state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	arch := arch.DefaultArchitecture
	hwChar := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	instId := instance.Id("i-host-machine")
	err = machine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("0")},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	application, err := st.Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	expected, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(expected.URL(), gc.DeepEquals, curl)
	c.Assert(expected.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(expected.Config(), gc.DeepEquals, ch.Config())

	expectedProfile := ch.(charm.LXDProfiler).LXDProfile()
	c.Assert(expected.LXDProfile(), gc.DeepEquals, &charm.LXDProfile{
		Description: expectedProfile.Description,
		Config:      expectedProfile.Config,
		Devices:     expectedProfile.Devices,
	})

	errs, err := unitassignerapi.New(s.OpenControllerModelAPI(c)).AssignUnits(context.Background(), []names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := application.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToMachineWithInvalidLXDProfileAndForceStillSucceeds(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	curl, ch := s.addCharmToState(c, "ch:jammy/lxd-profile-fail-0", "lxd-profile-fail")

	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(s.InstancePrechecker(c, st), state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	arch := arch.DefaultArchitecture
	hwChar := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	instId := instance.Id("i-host-machine")
	err = machine.SetProvisioned(instId, "", "fake-nonce", hwChar)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl,
			CharmOrigin:     createCharmOriginFromURL(curl),
			ApplicationName: "application-name",
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("0")},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	app, err := st.Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	expected, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(expected.URL(), gc.DeepEquals, curl)
	c.Assert(expected.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(expected.Config(), gc.DeepEquals, ch.Config())

	expectedProfile := ch.(charm.LXDProfiler).LXDProfile()
	c.Assert(expected.LXDProfile(), gc.DeepEquals, &charm.LXDProfile{
		Description: expectedProfile.Description,
		Config:      expectedProfile.Config,
		Devices:     expectedProfile.Devices,
	})

	errs, err := unitassignerapi.New(s.OpenControllerModelAPI(c)).AssignUnits(context.Background(), []names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToCharmNotFound(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	results, err := s.applicationAPI.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        "ch:jammy/application-name-1",
			CharmOrigin:     &params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"}},
			ApplicationName: "application-name",
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("42")},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `cannot deploy "application-name": charm "ch:jammy/application-name-1" not found`)

	_, err = s.ControllerModel(c).State().Application("application-name")
	c.Assert(err, gc.ErrorMatches, `application "application-name" not found`)
}

func (s *applicationSuite) TestApplicationUpdateDoesNotSetMinUnitsWithLXDProfile(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	series := "quantal"
	repo := testcharms.RepoForSeries(series)
	ch := repo.CharmDir("lxd-profile-fail")
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL(fmt.Sprintf("local:%s/%s", series, ident))
	_, err := jujutesting.PutCharm(s.ControllerModel(c).State(), s.ObjectStore(c, s.ControllerModelUUID()), curl, ch)
	c.Assert(err, gc.ErrorMatches, `invalid lxd-profile.yaml: contains device type "unix-disk"`)
}

var clientAddApplicationUnitsTests = []struct {
	about       string
	application string // if not set, defaults to 'dummy'
	numUnits    int
	expected    []string
	to          string
	err         string
}{
	{
		about:    "returns unit names",
		numUnits: 3,
		expected: []string{"dummy/0", "dummy/1", "dummy/2"},
	},
	{
		about: "fails trying to add zero units",
		err:   "must add at least one unit",
	},
	{
		// Note: chained-state, we add 1 unit here, but the 3 units
		// from the first condition still exist
		about:    "force the unit onto bootstrap machine",
		numUnits: 1,
		expected: []string{"dummy/3"},
		to:       "0",
	},
	{
		about:       "unknown application name",
		application: "unknown-application",
		numUnits:    1,
		err:         `application "unknown-application" not found`,
	},
}

func (s *applicationSuite) TestClientAddApplicationUnits(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})
	for i, t := range clientAddApplicationUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		applicationName := t.application
		if applicationName == "" {
			applicationName = "dummy"
		}
		args := params.AddApplicationUnits{
			ApplicationName: applicationName,
			NumUnits:        t.numUnits,
		}
		if t.to != "" {
			args.Placement = []*instance.Placement{instance.MustParsePlacement(t.to)}
		}
		result, err := s.applicationAPI.AddUnits(context.Background(), args)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Units, gc.DeepEquals, t.expected)
	}
	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.ControllerModel(c).State().Unit("dummy/3")
	c.Assert(err, jc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

func (s *applicationSuite) TestAddApplicationUnitsToNewContainer(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})
	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(s.InstancePrechecker(c, st), state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.applicationAPI.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        1,
		Placement:       []*instance.Placement{instance.MustParsePlacement("lxd:" + machine.Id())},
	})
	c.Assert(err, jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id()+"/lxd/0")
}

var addApplicationUnitTests = []struct {
	about       string
	application string // if not set, defaults to 'dummy'
	expected    []string
	machineIds  []string
	placement   []*instance.Placement
	err         string
}{
	/*{
		about:      "valid placement directives",
		expected:   []string{"dummy/0"},
		placement:  []*instance.Placement{{Scope: "deadbeef-0bad-400d-8000-4b1d0d06f00d", Directive: "valid"}},
		machineIds: []string{"1"},
	}, {
		about:      "direct machine assignment placement directive",
		expected:   []string{"dummy/1", "dummy/2"},
		placement:  []*instance.Placement{{Scope: "#", Directive: "1"}, {Scope: "lxd", Directive: "1"}},
		machineIds: []string{"1", "1/lxd/0"},
	},*/{
		about:     "invalid placement directive",
		err:       ".* invalid placement is invalid",
		expected:  []string{"dummy/3"},
		placement: []*instance.Placement{{Scope: "deadbeef-0bad-400d-8000-4b1d0d06f00d", Directive: "invalid"}},
	},
}

func (s *applicationSuite) TestAddApplicationUnits(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})

	// Add a machine for the units to be placed on.
	st := s.ControllerModel(c).State()
	_, err := st.AddMachine(s.InstancePrechecker(c, st), state.UbuntuBase("22.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	for i, t := range addApplicationUnitTests {
		c.Logf("test %d. %s", i, t.about)
		applicationName := t.application
		if applicationName == "" {
			applicationName = "dummy"
		}
		result, err := s.applicationAPI.AddUnits(context.Background(), params.AddApplicationUnits{
			ApplicationName: applicationName,
			NumUnits:        len(t.expected),
			Placement:       t.placement,
		})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Units, gc.DeepEquals, t.expected)
		for i, unitName := range result.Units {
			u, err := s.ControllerModel(c).State().Unit(unitName)
			c.Assert(err, jc.ErrorIsNil)
			assignedMachine, err := u.AssignedMachineId()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(assignedMachine, gc.Equals, t.machineIds[i])
		}
	}
}

func (s *applicationSuite) assertAddApplicationUnits(c *gc.C) {
	result, err := s.applicationAPI.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Units, gc.DeepEquals, []string{"dummy/0", "dummy/1", "dummy/2"})

	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.ControllerModel(c).State().Unit("dummy/0")
	c.Assert(err, jc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

func (s *applicationSuite) TestApplicationCharmRelations(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})

	st := s.ControllerModel(c).State()
	eps, err := st.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.applicationAPI.CharmRelations(context.Background(), params.ApplicationCharmRelations{ApplicationName: "blah"})
	c.Assert(err, gc.ErrorMatches, `application "blah" not found`)

	result, err := s.applicationAPI.CharmRelations(context.Background(), params.ApplicationCharmRelations{ApplicationName: "wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.CharmRelations, gc.DeepEquals, []string{
		"cache", "db", "juju-info", "logging-dir", "monitoring-port", "url",
	})
}

func (s *applicationSuite) assertAddApplicationUnitsBlocked(c *gc.C, msg string) {
	_, err := s.applicationAPI.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
	})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroyAddApplicationUnits(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})
	s.BlockDestroyModel(c, "TestBlockDestroyAddApplicationUnits")
	s.assertAddApplicationUnits(c)
}

func (s *applicationSuite) TestBlockRemoveAddApplicationUnits(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})
	s.BlockRemoveObject(c, "TestBlockRemoveAddApplicationUnits")
	s.assertAddApplicationUnits(c)
}

func (s *applicationSuite) TestBlockChangeAddApplicationUnits(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})
	s.BlockAllChanges(c, "TestBlockChangeAddApplicationUnits")
	s.assertAddApplicationUnitsBlocked(c, "TestBlockChangeAddApplicationUnits")
}

func (s *applicationSuite) TestAddUnitToMachineNotFound(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})
	_, err := s.applicationAPI.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
		Placement:       []*instance.Placement{instance.MustParsePlacement("42")},
	})
	c.Assert(err, gc.ErrorMatches, `acquiring machine to host unit "dummy/0": machine 42 not found`)
}

func (s *applicationSuite) TestApplicationExpose(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	charm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
	applicationNames := []string{"dummy-application", "exposed-application"}
	apps := make([]*state.Application, len(applicationNames))
	var err error
	for i, name := range applicationNames {
		apps[i] = f.MakeApplication(c, &factory.ApplicationParams{
			Name:  name,
			Charm: charm,
		})
		c.Assert(apps[i].IsExposed(), jc.IsFalse)
	}
	err = apps[1].MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apps[1].IsExposed(), jc.IsTrue)

	s.assertApplicationExpose(c)
}

func (s *applicationSuite) TestApplicationExposeEndpoints(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	c.Assert(app.IsExposed(), jc.IsFalse)

	err := s.applicationAPI.Expose(context.Background(), params.ApplicationExpose{
		ApplicationName: app.Name(),
		ExposedEndpoints: map[string]params.ExposedEndpoint{
			// Exposing an endpoint with no expose options implies
			// expose to 0.0.0.0/0 and ::/0.
			"monitoring-port": {},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.ControllerModel(c).State().Application(app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.IsExposed(), gc.Equals, true)
	c.Assert(got.ExposedEndpoints(), gc.DeepEquals, map[string]state.ExposedEndpoint{
		"monitoring-port": {
			ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR},
		},
	})
}

func (s *applicationSuite) TestApplicationExposeEndpointsWithPre29Client(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	c.Assert(app.IsExposed(), jc.IsFalse)

	err := s.applicationAPI.Expose(context.Background(), params.ApplicationExpose{
		ApplicationName: app.Name(),
		// If no endpoint-specific expose params are provided, the call
		// will emulate the behavior of a pre 2.9 controller where all
		// ports are exposed to 0.0.0.0/0 and ::/0.
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := s.ControllerModel(c).State().Application(app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.IsExposed(), gc.Equals, true)
	c.Assert(got.ExposedEndpoints(), gc.DeepEquals, map[string]state.ExposedEndpoint{
		"": {
			ExposeToCIDRs: []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR},
		},
	})
}

func (s *applicationSuite) setupApplicationExpose(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	charm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
	applicationNames := []string{"dummy-application", "exposed-application"}
	apps := make([]*state.Application, len(applicationNames))
	var err error
	for i, name := range applicationNames {
		apps[i] = f.MakeApplication(c, &factory.ApplicationParams{
			Name:  name,
			Charm: charm,
		})
		c.Assert(apps[i].IsExposed(), jc.IsFalse)
	}
	err = apps[1].MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apps[1].IsExposed(), jc.IsTrue)
}

var applicationExposeTests = []struct {
	about                 string
	application           string
	exposedEndpointParams map[string]params.ExposedEndpoint
	//
	expExposed          bool
	expExposedEndpoints map[string]state.ExposedEndpoint
	expErr              string
}{
	{
		about:       "unknown application name",
		application: "unknown-application",
		expErr:      `application "unknown-application" not found`,
	},
	{
		about:       "expose all endpoints of an application ",
		application: "dummy-application",
		expExposed:  true,
		expExposedEndpoints: map[string]state.ExposedEndpoint{
			"": {
				ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
			},
		},
	},
	{
		about:       "expose an already exposed application",
		application: "exposed-application",
		expExposed:  true,
		expExposedEndpoints: map[string]state.ExposedEndpoint{
			"": {
				ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
			},
		},
	},
	{
		about:       "unknown endpoint name in expose parameters",
		application: "dummy-application",
		exposedEndpointParams: map[string]params.ExposedEndpoint{
			"bogus": {},
		},
		expErr: `endpoint "bogus" not found`,
	},
	{
		about:       "unknown space name in expose parameters",
		application: "dummy-application",
		exposedEndpointParams: map[string]params.ExposedEndpoint{
			"": {
				ExposeToSpaces: []string{"invaders"},
			},
		},
		expErr: `space "invaders" not found`,
	},
	{
		about:       "expose an application and provide expose parameters",
		application: "exposed-application",
		exposedEndpointParams: map[string]params.ExposedEndpoint{
			"": {
				ExposeToSpaces: []string{network.AlphaSpaceName},
				ExposeToCIDRs:  []string{"13.37.0.0/16"},
			},
		},
		expExposed: true,
		expExposedEndpoints: map[string]state.ExposedEndpoint{
			"": {
				ExposeToSpaceIDs: []string{network.AlphaSpaceId},
				ExposeToCIDRs:    []string{"13.37.0.0/16"},
			},
		},
	},
}

func (s *applicationSuite) assertApplicationExpose(c *gc.C) {
	for i, t := range applicationExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.applicationAPI.Expose(context.Background(), params.ApplicationExpose{
			ApplicationName:  t.application,
			ExposedEndpoints: t.exposedEndpointParams,
		})
		if t.expErr != "" {
			c.Assert(err, gc.ErrorMatches, t.expErr)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			app, err := s.ControllerModel(c).State().Application(t.application)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(app.IsExposed(), gc.Equals, t.expExposed)
			c.Assert(app.ExposedEndpoints(), gc.DeepEquals, t.expExposedEndpoints)
		}
	}
}

func (s *applicationSuite) assertApplicationExposeBlocked(c *gc.C, msg string) {
	for i, t := range applicationExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.applicationAPI.Expose(context.Background(), params.ApplicationExpose{
			ApplicationName:  t.application,
			ExposedEndpoints: t.exposedEndpointParams,
		})
		s.AssertBlocked(c, err, msg)
	}
}

func (s *applicationSuite) TestBlockDestroyApplicationExpose(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupApplicationExpose(c)
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationExpose")
	s.assertApplicationExpose(c)
}

func (s *applicationSuite) TestBlockRemoveApplicationExpose(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupApplicationExpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationExpose")
	s.assertApplicationExpose(c)
}

func (s *applicationSuite) TestBlockChangesApplicationExpose(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupApplicationExpose(c)
	s.BlockAllChanges(c, "TestBlockChangesApplicationExpose")
	s.assertApplicationExposeBlocked(c, "TestBlockChangesApplicationExpose")
}

var applicationUnexposeTests = []struct {
	about               string
	application         string
	err                 string
	initial             map[string]state.ExposedEndpoint
	unexposeEndpoints   []string
	expExposed          bool
	expExposedEndpoints map[string]state.ExposedEndpoint
}{
	{
		about:       "unknown application name",
		application: "unknown-application",
		err:         `application "unknown-application" not found`,
	},
	{
		about:       "unexpose a application without specifying any endpoints",
		application: "dummy-application",
		initial: map[string]state.ExposedEndpoint{
			"": {},
		},
		expExposed: false,
	},
	{
		about:       "unexpose specific application endpoint",
		application: "dummy-application",
		initial: map[string]state.ExposedEndpoint{
			"server":       {},
			"server-admin": {},
		},
		unexposeEndpoints: []string{"server"},
		// The server-admin (and hence the app) should remain exposed
		expExposed: true,
		expExposedEndpoints: map[string]state.ExposedEndpoint{
			"server-admin": {ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"}},
		},
	},
	{
		about:       "unexpose all currently exposed application endpoints",
		application: "dummy-application",
		initial: map[string]state.ExposedEndpoint{
			"server":       {},
			"server-admin": {},
		},
		unexposeEndpoints: []string{"server", "server-admin"},
		// Application should now be unexposed as all its endpoints have
		// been unexposed.
		expExposed: false,
	},
	{
		about:       "unexpose an already unexposed application",
		application: "dummy-application",
		initial:     nil,
		expExposed:  false,
	},
}

func (s *applicationSuite) TestApplicationUnexpose(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	charm := f.MakeCharm(c, nil)
	for i, t := range applicationUnexposeTests {
		c.Logf("test %d. %s", i, t.about)
		app := f.MakeApplication(c, &factory.ApplicationParams{
			Name:  "dummy-application",
			Charm: charm,
		})
		if len(t.initial) != 0 {
			err := app.MergeExposeSettings(t.initial)
			c.Assert(err, jc.ErrorIsNil)
		}
		c.Assert(app.IsExposed(), gc.Equals, len(t.initial) != 0)
		err := s.applicationAPI.Unexpose(context.Background(), params.ApplicationUnexpose{
			ApplicationName:  t.application,
			ExposedEndpoints: t.unexposeEndpoints,
		})
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			app.Refresh()
			c.Assert(app.IsExposed(), gc.Equals, t.expExposed)
			c.Assert(app.ExposedEndpoints(), gc.DeepEquals, t.expExposedEndpoints)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		err = app.Destroy(s.store)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *applicationSuite) setupApplicationUnexpose(c *gc.C) *state.Application {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy-application",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})
	err := app.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.IsExposed(), gc.Equals, true)
	return app
}

func (s *applicationSuite) assertApplicationUnexpose(c *gc.C, app *state.Application) {
	err := s.applicationAPI.Unexpose(context.Background(), params.ApplicationUnexpose{ApplicationName: "dummy-application"})
	c.Assert(err, jc.ErrorIsNil)
	app.Refresh()
	c.Assert(app.IsExposed(), gc.Equals, false)
	err = app.Destroy(s.store)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) assertApplicationUnexposeBlocked(c *gc.C, app *state.Application, msg string) {
	err := s.applicationAPI.Unexpose(context.Background(), params.ApplicationUnexpose{ApplicationName: "dummy-application"})
	s.AssertBlocked(c, err, msg)
	err = app.Destroy(s.store)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestBlockDestroyApplicationUnexpose(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	app := s.setupApplicationUnexpose(c)
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationUnexpose")
	s.assertApplicationUnexpose(c, app)
}

func (s *applicationSuite) TestBlockRemoveApplicationUnexpose(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	app := s.setupApplicationUnexpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationUnexpose")
	s.assertApplicationUnexpose(c, app)
}

func (s *applicationSuite) TestBlockChangesApplicationUnexpose(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	app := s.setupApplicationUnexpose(c)
	s.BlockAllChanges(c, "TestBlockChangesApplicationUnexpose")
	s.assertApplicationUnexposeBlocked(c, app, "TestBlockChangesApplicationUnexpose")
}

func (s *applicationSuite) TestClientSetApplicationConstraints(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})

	// Update constraints for the application.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.applicationAPI.SetConstraints(context.Background(), params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := app.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *applicationSuite) setupSetApplicationConstraints(c *gc.C) (*state.Application, constraints.Value) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "dummy",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})
	// Update constraints for the application.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	return app, cons
}

func (s *applicationSuite) assertSetApplicationConstraints(c *gc.C, application *state.Application, cons constraints.Value) {
	err := s.applicationAPI.SetConstraints(context.Background(), params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure the constraints have been correctly updated.
	obtained, err := application.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *applicationSuite) assertSetApplicationConstraintsBlocked(c *gc.C, msg string, application *state.Application, cons constraints.Value) {
	err := s.applicationAPI.SetConstraints(context.Background(), params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroySetApplicationConstraints(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockDestroyModel(c, "TestBlockDestroySetApplicationConstraints")
	s.assertSetApplicationConstraints(c, app, cons)
}

func (s *applicationSuite) TestBlockRemoveSetApplicationConstraints(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockRemoveObject(c, "TestBlockRemoveSetApplicationConstraints")
	s.assertSetApplicationConstraints(c, app, cons)
}

func (s *applicationSuite) TestBlockChangesSetApplicationConstraints(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockAllChanges(c, "TestBlockChangesSetApplicationConstraints")
	s.assertSetApplicationConstraintsBlocked(c, "TestBlockChangesSetApplicationConstraints", app, cons)
}

func (s *applicationSuite) TestClientGetApplicationConstraints(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	fooConstraints := constraints.MustParse("arch=amd64", "mem=4G")
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:        "foo",
		Constraints: fooConstraints,
	})
	barConstraints := constraints.MustParse("arch=amd64", "mem=128G", "cores=64")
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:        "bar",
		Constraints: barConstraints,
	})

	results, err := s.applicationAPI.GetConstraints(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "wat"}, {Tag: "machine-0"}, {Tag: "user-foo"},
			{Tag: "application-foo"}, {Tag: "application-bar"}, {Tag: "application-wat"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ApplicationGetConstraintsResults{
		Results: []params.ApplicationConstraint{
			{
				Error: &params.Error{Message: `"wat" is not a valid tag`},
			}, {
				Error: &params.Error{Message: `unexpected tag type, expected application, got machine`},
			}, {
				Error: &params.Error{Message: `unexpected tag type, expected application, got user`},
			}, {
				Constraints: fooConstraints,
			}, {
				Constraints: barConstraints,
			}, {
				Error: &params.Error{Message: `application "wat" not found`, Code: "not found"},
			},
		}})
}

func (s *applicationSuite) checkEndpoints(c *gc.C, mysqlAppName string, endpoints map[string]params.CharmRelation) {
	c.Assert(endpoints["wordpress"], gc.DeepEquals, params.CharmRelation{
		Name:      "db",
		Role:      "requirer",
		Interface: "mysql",
		Optional:  false,
		Limit:     1,
		Scope:     "global",
	})
	ep := params.CharmRelation{
		Name:      "server",
		Role:      "provider",
		Interface: "mysql",
		Scope:     "global",
	}
	// Remote applications don't use scope.
	if mysqlAppName == "hosted-mysql" {
		ep.Scope = ""
	}
	c.Assert(endpoints[mysqlAppName], gc.DeepEquals, ep)
}

func (s *applicationSuite) setupRelationScenario(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	eps, err := s.ControllerModel(c).State().InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.ControllerModel(c).State().AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) assertAddRelation(c *gc.C, endpoints, viaCIDRs []string) {
	s.setupRelationScenario(c)

	res, err := s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints, ViaCIDRs: viaCIDRs})
	c.Assert(err, jc.ErrorIsNil)
	// Show that the relation was added.
	st := s.ControllerModel(c).State()
	wpApp, err := st.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rels, err := wpApp.Relations()
	c.Assert(err, jc.ErrorIsNil)
	// There are 2 relations - the logging-wordpress one set up in the
	// scenario and the one created in this test.
	c.Assert(len(rels), gc.Equals, 2)

	// We may be related to a local application or a remote offer
	// or an application in another model.
	var mySqlApplication state.ApplicationEntity
	mySqlApplication, err = st.RemoteApplication("hosted-mysql")
	if errors.Is(err, errors.NotFound) {
		mySqlApplication, err = st.RemoteApplication("othermysql")
		if errors.Is(err, errors.NotFound) {
			mySqlApplication, err = st.Application("mysql")
			c.Assert(err, jc.ErrorIsNil)
			s.checkEndpoints(c, "mysql", res.Endpoints)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			s.checkEndpoints(c, "othermysql", res.Endpoints)
		}
	} else {
		c.Assert(err, jc.ErrorIsNil)
		s.checkEndpoints(c, "hosted-mysql", res.Endpoints)
	}
	c.Assert(err, jc.ErrorIsNil)
	rels, err = mySqlApplication.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(rels), gc.Equals, 1)
}

func (s *applicationSuite) TestSuccessfullyAddRelation(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	endpoints := []string{"wordpress", "mysql"}
	s.assertAddRelation(c, endpoints, nil)
}

func (s *applicationSuite) TestBlockDestroyAddRelation(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.BlockDestroyModel(c, "TestBlockDestroyAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"}, nil)
}

func (s *applicationSuite) TestBlockRemoveAddRelation(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.BlockRemoveObject(c, "TestBlockRemoveAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"}, nil)
}

func (s *applicationSuite) TestBlockChangesAddRelation(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	s.BlockAllChanges(c, "TestBlockChangesAddRelation")
	_, err := s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: []string{"wordpress", "mysql"}})
	s.AssertBlocked(c, err, "TestBlockChangesAddRelation")
}

func (s *applicationSuite) TestSuccessfullyAddRelationSwapped(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	// Show that the order of the applications listed in the AddRelation call
	// does not matter.  This is a repeat of the previous test with the application
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertAddRelation(c, endpoints, nil)
}

func (s *applicationSuite) TestCallWithOnlyOneEndpoint(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	endpoints := []string{"wordpress"}
	_, err := s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestCallWithOneEndpointTooMany(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	endpoints := []string{"wordpress", "mysql", "logging"}
	_, err := s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "cannot relate 3 endpoints")
}

func (s *applicationSuite) TestAddAlreadyAddedRelation(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	// Add a relation between wordpress and mysql.
	st := s.ControllerModel(c).State()
	endpoints := []string{"wordpress", "mysql"}
	eps, err := st.InferEndpoints(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	// And try to add it again.
	_, err = s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation wordpress:db mysql:server`)
}

func (s *applicationSuite) setupRemoteApplication(c *gc.C) {
	results, err := s.applicationAPI.Consume(context.Background(), params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{
			{ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag:         testing.ModelTag.String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "A pretty popular database",
				Endpoints: []params.RemoteEndpoint{
					{Name: "server", Interface: "mysql", Role: "provider"},
				},
			}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *applicationSuite) TestAddRemoteRelation(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupRemoteApplication(c)
	// There's already a wordpress in the scenario this assertion sets up.
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql"}, nil)
}

func (s *applicationSuite) TestAddRemoteRelationWithRelName(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupRemoteApplication(c)
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql:server"}, nil)
}

func (s *applicationSuite) TestAddRemoteRelationVia(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupRemoteApplication(c)
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql:server"}, []string{"192.168.0.0/16"})

	rel, err := s.ControllerModel(c).State().KeyRelation("wordpress:db hosted-mysql:server")
	c.Assert(err, jc.ErrorIsNil)
	w := rel.WatchRelationEgressNetworks()
	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	wc.AssertChange("192.168.0.0/16")
	wc.AssertNoChange()
}

func (s *applicationSuite) TestAddRemoteRelationOnlyOneEndpoint(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupRemoteApplication(c)
	endpoints := []string{"hosted-mysql"}
	_, err := s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestAlreadyAddedRemoteRelation(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupRemoteApplication(c)
	endpoints := []string{"wordpress", "hosted-mysql"}
	s.assertAddRelation(c, endpoints, nil)

	// And try to add it again.
	_, err := s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`cannot add relation "wordpress:db hosted-mysql:server": relation wordpress:db hosted-mysql:server`))
}

func (s *applicationSuite) TestRemoteRelationInvalidEndpoint(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	s.setupRemoteApplication(c)
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})

	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `saas application "hosted-mysql" has no "nope" relation`)
}

func (s *applicationSuite) TestRemoteRelationNoMatchingEndpoint(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	results, err := s.applicationAPI.Consume(context.Background(), params.ConsumeApplicationArgsV5{
		Args: []params.ConsumeApplicationArgV5{
			{ApplicationOfferDetailsV5: params.ApplicationOfferDetailsV5{
				SourceModelTag: testing.ModelTag.String(),
				OfferName:      "hosted-db2",
				OfferUUID:      "hosted-db2-uuid",
				Endpoints: []params.RemoteEndpoint{
					{Name: "database", Interface: "db2", Role: "provider"},
				},
			}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	endpoints := []string{"wordpress", "hosted-db2"}
	_, err = s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestRemoteRelationApplicationNotFound(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	endpoints := []string{"wordpress", "unknown"}
	_, err := s.applicationAPI.AddRelation(context.Background(), params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `application "unknown" not found`)
}

// addCharmToState emulates the side-effects of an AddCharm call so that the
// deploy tests in the suite can still work even though the AddCharmX calls
// have been updated to return NotSupported errors for Juju 3.
func (s *applicationSuite) addCharmToState(c *gc.C, charmURL string, name string) (string, charm.Charm) {
	curl := charm.MustParseURL(charmURL)

	if curl.Revision < 0 {
		base := curl.String()

		if rev, found := s.lastKnownRev[base]; found {
			curl.Revision = rev + 1
		} else {
			curl.Revision = 0
		}

		s.lastKnownRev[base] = curl.Revision
	}

	st := s.ControllerModel(c).State()
	_, err := st.PrepareCharmUpload(charmURL)
	c.Assert(err, jc.ErrorIsNil)

	ch, err := charm.ReadCharmArchive(
		testcharms.RepoWithSeries("quantal").CharmArchivePath(c.MkDir(), name))
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.UpdateUploadedCharm(state.CharmInfo{
		Charm:       ch,
		ID:          charmURL,
		StoragePath: fmt.Sprintf("charms/%s", name),
		SHA256:      "1234",
		Version:     ch.Version(),
	})
	c.Assert(err, jc.ErrorIsNil)

	return charmURL, ch
}

func (s *applicationSuite) TestValidateSecretConfig(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.makeAPI(c)

	chCfg := &charm.Config{
		Options: map[string]charm.Option{
			"foo": {Type: "secret"},
		},
	}
	cfg := charm.Settings{
		"foo": "bar",
	}
	err := application.ValidateSecretConfig(chCfg, cfg)
	c.Assert(err, gc.ErrorMatches, `invalid secret URI for option "foo": secret URI "bar" not valid`)

	cfg = charm.Settings{"foo": "secret:cj4v5vm78ohs79o84r4g"}
	err = application.ValidateSecretConfig(chCfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
}
