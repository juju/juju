// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher_test

import (
	"sync"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiservercertwatcher"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	manifold dependency.Manifold
	getter   dependency.Getter
	agent    *mockAgent
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{
		conf: mockConfig{
			caCert: coretesting.OtherCACert,
			info: &controller.ControllerAgentInfo{
				CAPrivateKey: coretesting.OtherCAKey,
				Cert:         coretesting.ServerCert,
				PrivateKey:   coretesting.ServerKey,
			},
		},
	}
	s.getter = dt.StubGetter(map[string]interface{}{
		"agent": s.agent,
	})
	s.manifold = apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
		AgentName: "agent",
	})
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, []string{"agent"})
}

func (s *ManifoldSuite) TestNoAgent(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	_, err := s.manifold.Start(c.Context(), getter)
	c.Assert(err, tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNoStateServingInfo(c *tc.C) {
	s.agent.conf.info = nil
	_, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorMatches, "setting up initial ca authority: no state serving info in agent config")
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestOutput(c *tc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var authority pki.Authority
	err := s.manifold.Output(w, &authority)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) startWorkerClean(c *tc.C) worker.Worker {
	w, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockConfig struct {
	agent.Config

	mu     sync.Mutex
	info   *controller.ControllerAgentInfo
	caCert string
}

func (mc *mockConfig) CACert() string {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.caCert
}

func (mc *mockConfig) StateServingInfo() (controller.ControllerAgentInfo, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.info != nil {
		return *mc.info, true
	}
	return controller.ControllerAgentInfo{}, false
}
