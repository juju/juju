// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiconfigwatcher_test

import (
	"sync"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/apiconfigwatcher"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold           dependency.Manifold
	context            dependency.Context
	agent              *mockAgent
	agentConfigChanged *voyeur.Value
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = new(mockAgent)
	s.context = dt.StubContext(nil, map[string]interface{}{
		"agent": s.agent,
	})
	s.agentConfigChanged = voyeur.NewValue(0)
	s.manifold = apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
		AgentName:          "agent",
		AgentConfigChanged: s.agentConfigChanged,
		Logger:             loggo.GetLogger("test"),
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{"agent"})
}

func (s *ManifoldSuite) TestNilAgentConfigChanged(c *gc.C) {
	manifold := apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
		AgentName: "agent",
	})
	_, err := manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "nil AgentConfigChanged .+")
}

func (s *ManifoldSuite) TestNoAgent(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	_, err := s.manifold.Start(context)
	c.Assert(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestBounceOnChange(c *gc.C) {
	s.agent.conf.setAddresses("1.1.1.1:1")
	w := s.startWorkerClean(c)

	// Change API addresses - worker should bounce.
	s.agent.conf.setAddresses("2.2.2.2:2")
	s.agentConfigChanged.Set(0)
	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.Equals, dependency.ErrBounce)

	// Restart the worker - worker should stay up.
	w = s.startWorkerClean(c)

	// Change API addresses again - worker should bounce again.
	s.agent.conf.setAddresses("2.2.2.2:2", "3.3.3.3:3")
	s.agentConfigChanged.Set(0)
	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.Equals, dependency.ErrBounce)
}

func (s *ManifoldSuite) TestConfigChangeWithNoAddrChange(c *gc.C) {
	s.agent.conf.setAddresses("1.1.1.1:1")
	w := s.startWorkerClean(c)

	// Signal config change without changing API addresses - worker
	// should continue running.
	s.agentConfigChanged.Set(0)
	workertest.CheckAlive(c, w)
}

func (s *ManifoldSuite) TestConfigChangeWithAddrReordering(c *gc.C) {
	s.agent.conf.setAddresses("1.1.1.1:1", "2.2.2.2:2")
	w := s.startWorkerClean(c)

	// Change API address ordering - worker should stay up.
	s.agent.conf.setAddresses("2.2.2.2:2", "1.1.1.1:1")
	s.agentConfigChanged.Set(0)
	workertest.CheckAlive(c, w)
}

func (s *ManifoldSuite) TestClosedVoyeur(c *gc.C) {
	w := s.startWorkerClean(c)
	s.agentConfigChanged.Close()
	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "config changed value closed")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
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

	mu    sync.Mutex
	addrs []string
}

func (mc *mockConfig) setAddresses(addrs ...string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.addrs = append([]string(nil), addrs...)
}

func (mc *mockConfig) APIAddresses() ([]string, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.addrs, nil
}
