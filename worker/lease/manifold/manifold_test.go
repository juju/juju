// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manifold_test

import (
	"io"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	coredatabase "github.com/juju/juju/core/database"
	corelease "github.com/juju/juju/core/lease"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/lease"
	leasemanager "github.com/juju/juju/worker/lease/manifold"
)

type manifoldSuite struct {
	statetesting.StateSuite

	context  dependency.Context
	manifold dependency.Manifold

	agent      *mockAgent
	clock      *testclock.Clock
	dbAccessor stubDBGetter

	logger  loggo.Logger
	metrics prometheus.Registerer

	worker    worker.Worker
	trackedDB coredatabase.TrackedDB
	store     *lease.Store

	stub testing.Stub
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.StateSuite.SetUpTest(c)

	s.stub.ResetCalls()

	s.trackedDB = NewMockTrackedDB(ctrl)

	s.agent = &mockAgent{conf: mockAgentConfig{
		uuid:    "controller-uuid",
		apiInfo: &api.Info{},
	}}
	s.clock = testclock.NewClock(time.Now())
	s.dbAccessor = stubDBGetter{s.trackedDB}

	s.logger = loggo.GetLogger("lease.manifold_test")
	registerer := struct{ prometheus.Registerer }{}
	s.metrics = &registerer

	s.worker = &mockWorker{}
	s.store = &lease.Store{}

	s.context = s.newContext(nil)
	s.manifold = leasemanager.Manifold(leasemanager.ManifoldConfig{
		AgentName:            "agent",
		ClockName:            "clock",
		DBAccessorName:       "db-accessor",
		Logger:               &s.logger,
		PrometheusRegisterer: s.metrics,
		NewWorker:            s.newWorker,
		NewStore:             s.newStore,
	})
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":       s.agent,
		"clock":       s.clock,
		"db-accessor": s.dbAccessor,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) newWorker(config lease.ManagerConfig) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *manifoldSuite) newStore(config lease.StoreConfig) *lease.Store {
	s.stub.MethodCall(s, "NewStore", config)
	return s.store
}

var expectedInputs = []string{
	"agent", "clock", "db-accessor",
}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		ctx := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(ctx)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	_, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "NewStore", "NewWorker")

	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, lease.StoreConfig{})

	storeConfig := args[0].(lease.StoreConfig)

	c.Assert(storeConfig, gc.DeepEquals, lease.StoreConfig{
		TrackedDB: s.trackedDB,
		Logger:    &s.logger,
	})

	args = s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, lease.ManagerConfig{})
	config := args[0].(lease.ManagerConfig)

	secretary, err := config.Secretary(corelease.SingularControllerNamespace)
	c.Assert(err, jc.ErrorIsNil)
	// Check that this secretary knows the controller uuid.
	err = secretary.CheckLease(corelease.Key{Lease: "controller-uuid"})
	c.Assert(err, jc.ErrorIsNil)
	config.Secretary = nil

	c.Assert(config, jc.DeepEquals, lease.ManagerConfig{
		Store:                s.store,
		Clock:                s.clock,
		Logger:               &s.logger,
		MaxSleep:             time.Minute,
		EntityUUID:           "controller-uuid",
		PrometheusRegisterer: s.metrics,
	})
}

func (s *manifoldSuite) TestOutput(c *gc.C) {
	s.worker = &lease.Manager{}
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)

	var manager corelease.Manager
	err = s.manifold.Output(w, &manager)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(manager, gc.Equals, s.worker)

	var other io.Writer
	err = s.manifold.Output(w, &other)
	c.Assert(err, gc.ErrorMatches, `expected output of type \*core/lease.Manager, got \*io.Writer`)
}

type mockAgent struct {
	agent.Agent
	conf mockAgentConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockAgentConfig struct {
	agent.Config
	uuid    string
	apiInfo *api.Info
}

func (c *mockAgentConfig) Controller() names.ControllerTag {
	return names.NewControllerTag(c.uuid)
}

func (c *mockAgentConfig) APIInfo() (*api.Info, bool) {
	return c.apiInfo, true
}

type mockWorker struct{}

func (*mockWorker) Kill() {}
func (*mockWorker) Wait() error {
	return nil
}

type stubDBGetter struct {
	trackedDB coredatabase.TrackedDB
}

func (s stubDBGetter) GetDB(name string) (coredatabase.TrackedDB, error) {
	if name != "controller" {
		return nil, errors.Errorf(`expected a request for "controller" DB; got %q`, name)
	}
	return s.trackedDB, nil
}
