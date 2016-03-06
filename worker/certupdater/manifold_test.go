// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	apiserverworker "github.com/juju/juju/worker/apiserver"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	workerstate "github.com/juju/juju/worker/state"
	"github.com/juju/juju/worker/util"
)

type ManifoldSuite struct {
	jujutesting.JujuConnSuite
	// testing.IsolationSuite.
	newCalled bool
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.newCalled = false
	s.PatchValue(&certupdater.NewCertificateUpdater,
		func(addressWatcher certupdater.AddressWatcher, getter certupdater.StateServingInfoGetter,
			configGetter certupdater.ModelConfigGetter, hostPortsGetter certupdater.APIHostPortsGetter, setter certupdater.StateServingInfoSetter,
		) worker.Worker {
			s.newCalled = true
			return new(mockWorker)
		},
	)
}

func (s *ManifoldSuite) TestMachineShouldWrite(c *gc.C) {
	config := s.fakeManifoldConfig()
	_, err := s.runManifold(
		certupdater.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		nil)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

func (s *ManifoldSuite) TestMachineShouldntWrite(c *gc.C) {
	config := s.fakeManifoldConfig()
	_, err := s.runManifold(
		certupdater.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

func (s *ManifoldSuite) TestNonAgent(c *gc.C) {
	config := s.fakeManifoldConfig()
	_, err := s.runManifold(
		certupdater.Manifold(config),
		&fakeAgent{tag: names.NewUserTag("foo")},
		nil)
	c.Assert(err, gc.ErrorMatches, "agent's tag is not a machine tag")
	c.Assert(s.newCalled, jc.IsTrue)
}

type fakeAgent struct {
	agent.Agent
	tag names.Tag
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: a.tag}
}

type fakeConfig struct {
	agent.Config
	tag names.Tag
}

func (c *fakeConfig) Tag() names.Tag {
	return c.tag
}

func (s *ManifoldSuite) fakeManifoldConfig() certupdater.ManifoldConfig {
	return certupdater.ManifoldConfig{
		PostUpgradeManifoldConfig: util.PostUpgradeManifoldConfig{
			AgentName: "agent-name",
		},
		StateName:     "state",
		APIServerName: "apiserver",
		ChangeConfig: func(mutate agent.ConfigMutator) error {
			s.newCalled = true
			return nil
		},
	}
}

func (s *ManifoldSuite) runManifold(
	manifold dependency.Manifold, agent agent.Agent, apiCaller base.APICaller,
) (worker.Worker, error) {
	if agent == nil {
		agent = new(dummyAgent)
	}

	// stTracker := workerstate.StateTracker{}
	getResource := dt.StubGetResource(dt.StubResources{
		"upgradewaiter-name": dt.StubResource{Output: true},
		"agent-name":         dt.StubResource{Output: agent},
		"state":              dt.StubResource{Output: s.newMockTracker()},
		"apiserver":          dt.StubResource{Output: new(mockCertChanger)},
	})
	return manifold.Start(getResource)
}

type dummyAgent struct {
	agent.Agent
}

type mockCertChanger struct {
	apiserverworker.CertChanger
}

func (m *mockCertChanger) CertChangedChan() chan params.StateServingInfo {
	return make(chan params.StateServingInfo)
}

type mockTracker struct {
	workerstate.StateTracker
	Use func() (*state.State, error)
}

func (s *ManifoldSuite) newMockTracker() *mockTracker {
	return &mockTracker{
		Use: func() (*state.State, error) {
			return s.State, nil
		},
	}
}

func (m *mockTracker) Done() error {
	return nil
}

type mockWorker struct {
	tomb tomb.Tomb
}

func (w *mockWorker) Kill() {
	w.tomb.Kill(nil)
	w.tomb.Done()
}

func (w *mockWorker) Wait() error {
	return w.tomb.Wait()
}
