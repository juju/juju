// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"time"

	"github.com/juju/description/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type Suite struct {
	statetesting.StateSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer

	facadeContext facadetest.Context
	callContext   context.ProviderCallContext
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	// Set up InitialConfig with a dummy provider configuration. This
	// is required to allow model import test to work.
	s.InitialConfig = jujutesting.CustomModelConfig(c, dummy.SampleConfig())

	// The call up to StateSuite's SetUpTest uses s.InitialConfig so
	// it has to happen here.
	s.StateSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}
	s.callContext = context.NewEmptyCloudCallContext()
	s.facadeContext = facadetest.Context{
		State_:     s.State,
		StatePool_: s.StatePool,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	}
}

func (s *Suite) TestFacadeRegistered(c *gc.C) {
	aFactory, err := apiserver.AllFacades().GetFactory("MigrationTarget", 1)
	c.Assert(err, jc.ErrorIsNil)

	api, err := aFactory(&facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, gc.FitsTypeOf, new(migrationtarget.API))
}

func (s *Suite) TestNotUser(c *gc.C) {
	s.authorizer.Tag = names.NewMachineTag("0")
	_, err := s.newAPI(nil, nil)
	c.Assert(errors.Cause(err), gc.Equals, apiservererrors.ErrPerm)
}

func (s *Suite) TestNotControllerAdmin(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("jrandomuser")
	_, err := s.newAPI(nil, nil)
	c.Assert(errors.Cause(err), gc.Equals, apiservererrors.ErrPerm)
}

func (s *Suite) importModel(c *gc.C, api *migrationtarget.API) names.ModelTag {
	uuid, bytes := s.makeExportedModel(c)
	err := api.Import(params.SerializedModel{Bytes: bytes})
	c.Assert(err, jc.ErrorIsNil)
	return names.NewModelTag(uuid)
}

func (s *Suite) TestPrechecks(c *gc.C) {
	api := s.mustNewAPI(c)
	args := params.MigrationModelInfo{
		UUID:                   "uuid",
		Name:                   "some-model",
		OwnerTag:               names.NewUserTag("someone").String(),
		AgentVersion:           s.controllerVersion(c),
		ControllerAgentVersion: s.controllerVersion(c),
	}
	err := api.Prechecks(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *Suite) TestCACert(c *gc.C) {
	api := s.mustNewAPI(c)
	r, err := api.CACert()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(r.Result), gc.Equals, jujutesting.CACert)
}

func (s *Suite) TestPrechecksFail(c *gc.C) {
	controllerVersion := s.controllerVersion(c)

	// Set the model version ahead of the controller.
	modelVersion := controllerVersion
	modelVersion.Minor++

	api := s.mustNewAPI(c)
	args := params.MigrationModelInfo{
		AgentVersion: modelVersion,
	}
	err := api.Prechecks(args)
	c.Assert(err, gc.NotNil)
}

func (s *Suite) TestImport(c *gc.C) {
	api := s.mustNewAPI(c)
	tag := s.importModel(c, api)
	// Check the model was imported.
	model, ph, err := s.StatePool.GetModel(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()
	c.Assert(model.Name(), gc.Equals, "some-model")
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeImporting)
}

func (s *Suite) TestImportLeadership(c *gc.C) {
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	for i := 0; i < 3; i++ {
		s.Factory.MakeUnit(c, &factory.UnitParams{Application: application})
	}
	target := s.State.LeaseNotifyTarget(loggo.GetLogger("migrationtarget_test"))
	target.Claimed(
		lease.Key{"application-leadership", s.State.ModelUUID(), "wordpress"},
		"wordpress/2",
	)

	var claimer fakeClaimer
	s.facadeContext.LeadershipClaimer_ = &claimer
	api := s.mustNewAPI(c)
	s.importModel(c, api)

	c.Assert(claimer.stub.Calls(), gc.HasLen, 1)
	claimer.stub.CheckCall(c, 0, "ClaimLeadership", "wordpress", "wordpress/2", time.Minute)
}

func (s *Suite) TestAbort(c *gc.C) {
	api := s.mustNewAPI(c)
	tag := s.importModel(c, api)

	err := api.Abort(params.ModelArgs{ModelTag: tag.String()})
	c.Assert(err, jc.ErrorIsNil)

	// The model should no longer exist.
	exists, err := s.State.ModelExists(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, jc.IsFalse)
}

func (s *Suite) TestAbortNotATag(c *gc.C) {
	api := s.mustNewAPI(c)
	err := api.Abort(params.ModelArgs{ModelTag: "not-a-tag"})
	c.Assert(err, gc.ErrorMatches, `"not-a-tag" is not a valid tag`)
}

func (s *Suite) TestAbortMissingModel(c *gc.C) {
	api := s.mustNewAPI(c)
	newUUID := utils.MustNewUUID().String()
	err := api.Abort(params.ModelArgs{ModelTag: names.NewModelTag(newUUID).String()})
	c.Assert(err, gc.ErrorMatches, `model "`+newUUID+`" not found`)
}

func (s *Suite) TestAbortNotImportingModel(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c)
	err = api.Abort(params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, gc.ErrorMatches, `migration mode for the model is not importing`)
}

func (s *Suite) TestActivate(c *gc.C) {
	api := s.mustNewAPI(c)
	tag := s.importModel(c, api)

	err := api.Activate(params.ModelArgs{ModelTag: tag.String()})
	c.Assert(err, jc.ErrorIsNil)

	model, ph, err := s.StatePool.GetModel(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	defer ph.Release()
	c.Assert(model.MigrationMode(), gc.Equals, state.MigrationModeNone)
}

func (s *Suite) TestActivateNotATag(c *gc.C) {
	api := s.mustNewAPI(c)
	err := api.Activate(params.ModelArgs{ModelTag: "not-a-tag"})
	c.Assert(err, gc.ErrorMatches, `"not-a-tag" is not a valid tag`)
}

func (s *Suite) TestActivateMissingModel(c *gc.C) {
	api := s.mustNewAPI(c)
	newUUID := utils.MustNewUUID().String()
	err := api.Activate(params.ModelArgs{ModelTag: names.NewModelTag(newUUID).String()})
	c.Assert(err, gc.ErrorMatches, `model "`+newUUID+`" not found`)
}

func (s *Suite) TestActivateNotImportingModel(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c)
	err = api.Activate(params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, gc.ErrorMatches, `migration mode for the model is not importing`)
}

func (s *Suite) TestLatestLogTime(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	t := time.Date(2016, 11, 30, 18, 14, 0, 100, time.UTC)
	tracker := state.NewLastSentLogTracker(st, model.UUID(), "migration-logtransfer")
	defer tracker.Close()
	err = tracker.Set(0, t.UnixNano())
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c)
	latest, err := api.LatestLogTime(params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latest, gc.Equals, t)
}

func (s *Suite) TestLatestLogTimeNeverSet(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	api := s.mustNewAPI(c)
	latest, err := api.LatestLogTime(params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latest, gc.Equals, time.Time{})
}

func (s *Suite) TestAdoptIAASResources(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	env := mockEnv{Stub: &testing.Stub{}}
	api, err := s.newAPI(func(model stateenvirons.Model) (environs.Environ, error) {
		c.Assert(model.ModelTag().Id(), gc.Equals, st.ModelUUID())
		return &env, nil
	}, func(model stateenvirons.Model) (caas.Broker, error) {
		return nil, errors.New("should not be called")
	})
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = api.AdoptResources(params.AdoptResourcesArgs{
		ModelTag:                m.ModelTag().String(),
		SourceControllerVersion: version.MustParse("3.2.1"),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(env.Stub.Calls(), gc.HasLen, 1)
	aCall := env.Stub.Calls()[0]
	c.Assert(aCall.FuncName, gc.Equals, "AdoptResources")
	c.Assert(aCall.Args[1], gc.Equals, st.ControllerUUID())
	c.Assert(aCall.Args[2], gc.Equals, version.MustParse("3.2.1"))
}

func (s *Suite) TestAdoptCAASResources(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()

	broker := mockBroker{Stub: &testing.Stub{}}
	api, err := s.newAPI(func(model stateenvirons.Model) (environs.Environ, error) {
		return nil, errors.New("should not be called")
	}, func(model stateenvirons.Model) (caas.Broker, error) {
		c.Assert(model.ModelTag().Id(), gc.Equals, st.ModelUUID())
		return &broker, nil
	})
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = api.AdoptResources(params.AdoptResourcesArgs{
		ModelTag:                m.ModelTag().String(),
		SourceControllerVersion: version.MustParse("3.2.1"),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(broker.Stub.Calls(), gc.HasLen, 1)
	aCall := broker.Stub.Calls()[0]
	c.Assert(aCall.FuncName, gc.Equals, "AdoptResources")
	c.Assert(aCall.Args[1], gc.Equals, st.ControllerUUID())
	c.Assert(aCall.Args[2], gc.Equals, version.MustParse("3.2.1"))
}

func (s *Suite) TestCheckMachinesSuccess(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	fact := factory.NewFactory(st, s.StatePool)
	fact.MakeMachine(c, &factory.MachineParams{
		InstanceId: "eriatarka",
	})
	m := fact.MakeMachine(c, &factory.MachineParams{
		InstanceId: "volta",
	})
	c.Assert(m.Id(), gc.Equals, "1")

	mockEnv := mockEnv{
		Stub: &testing.Stub{},
		instances: []*mockInstance{
			{id: "volta"},
			{id: "eriatarka"},
		},
	}
	api := s.mustNewAPIWithModel(c, &mockEnv, &mockBroker{})
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.CheckMachines(
		params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *Suite) TestCheckMachinesHandlesContainers(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	fact := factory.NewFactory(st, s.StatePool)
	m := fact.MakeMachine(c, &factory.MachineParams{
		InstanceId: "birds",
	})
	fact.MakeMachineNested(c, m.Id(), nil)

	mockEnv := mockEnv{
		Stub:      &testing.Stub{},
		instances: []*mockInstance{{id: "birds"}},
	}
	api := s.mustNewAPIWithModel(c, &mockEnv, &mockBroker{})
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.CheckMachines(
		params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *Suite) TestCheckMachinesHandlesManual(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	fact := factory.NewFactory(st, s.StatePool)
	fact.MakeMachine(c, &factory.MachineParams{
		InstanceId: "birds",
	})
	fact.MakeMachine(c, &factory.MachineParams{
		Nonce: "manual:flibbertigibbert",
	})

	mockEnv := mockEnv{
		Stub:      &testing.Stub{},
		instances: []*mockInstance{{id: "birds"}},
	}
	api := s.mustNewAPIWithModel(c, &mockEnv, &mockBroker{})

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	results, err := api.CheckMachines(
		params.ModelArgs{ModelTag: model.ModelTag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *Suite) newAPI(environFunc stateenvirons.NewEnvironFunc, brokerFunc stateenvirons.NewCAASBrokerFunc) (*migrationtarget.API, error) {
	api, err := migrationtarget.NewAPI(&s.facadeContext, environFunc, brokerFunc)
	return api, err
}

func (s *Suite) mustNewAPI(c *gc.C) *migrationtarget.API {
	api, err := s.newAPI(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) mustNewAPIWithModel(c *gc.C, env environs.Environ, broker caas.Broker) *migrationtarget.API {
	api, err := s.newAPI(func(stateenvirons.Model) (environs.Environ, error) {
		return env, nil
	}, func(stateenvirons.Model) (caas.Broker, error) {
		return broker, nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) makeExportedModel(c *gc.C) (string, []byte) {
	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	newUUID := utils.MustNewUUID().String()
	model.UpdateConfig(map[string]interface{}{
		"name": "some-model",
		"uuid": newUUID,
	})

	bytes, err := description.Serialize(model)
	c.Assert(err, jc.ErrorIsNil)
	return newUUID, bytes
}

func (s *Suite) controllerVersion(c *gc.C) version.Number {
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	vers, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	return vers
}

type mockEnv struct {
	environs.Environ
	*testing.Stub

	instances []*mockInstance
}

func (e *mockEnv) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, sourceVersion version.Number) error {
	e.MethodCall(e, "AdoptResources", ctx, controllerUUID, sourceVersion)
	return e.NextErr()
}

func (e *mockEnv) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	e.MethodCall(e, "AllInstances", ctx)
	results := make([]instances.Instance, len(e.instances))
	for i, anInstance := range e.instances {
		results[i] = anInstance
	}
	return results, e.NextErr()
}

type mockBroker struct {
	caas.Broker
	*testing.Stub
}

func (e *mockBroker) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, sourceVersion version.Number) error {
	e.MethodCall(e, "AdoptResources", ctx, controllerUUID, sourceVersion)
	return e.NextErr()
}

type mockInstance struct {
	instances.Instance
	id string
}

func (i *mockInstance) Id() instance.Id {
	return instance.Id(i.id)
}

type fakeClaimer struct {
	leadership.Claimer
	stub testing.Stub
}

func (c *fakeClaimer) ClaimLeadership(application, unit string, duration time.Duration) error {
	c.stub.AddCall("ClaimLeadership", application, unit, duration)
	return c.stub.NextErr()
}
