// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher_test

import (
	"crypto/tls"
	"sync"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/voyeur"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/apiservercertwatcher"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/workertest"
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

	s.agent = &mockAgent{
		conf: mockConfig{
			info: &params.StateServingInfo{
				Cert:       coretesting.ServerCert,
				PrivateKey: coretesting.ServerKey,
			},
		},
	}
	s.context = dt.StubContext(nil, map[string]interface{}{
		"agent": s.agent,
	})
	s.agentConfigChanged = voyeur.NewValue(0)
	s.manifold = apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
		AgentName:          "agent",
		AgentConfigChanged: s.agentConfigChanged,
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{"agent"})
}

func (s *ManifoldSuite) TestNilAgentConfigChanged(c *gc.C) {
	manifold := apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
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

func (s *ManifoldSuite) TestNoStateServingInfo(c *gc.C) {
	s.agent.conf.info = nil
	_, err := s.manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "parsing initial certificate: no state serving info in agent config")
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var getCert func() *tls.Certificate
	err := s.manifold.Output(w, &getCert)
	c.Assert(err, jc.ErrorIsNil)

	cert := getCert()
	c.Assert(cert, gc.NotNil)
	c.Assert(cert.Leaf, gc.NotNil)

	cert_ := getCert()
	c.Assert(cert, gc.Equals, cert_)
}

func (s *ManifoldSuite) TestCertUpdated(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var getCert func() *tls.Certificate
	err := s.manifold.Output(w, &getCert)
	c.Assert(err, jc.ErrorIsNil)

	cert := getCert()
	c.Assert(cert, gc.NotNil)
	c.Assert(cert.Leaf, gc.NotNil)

	// Update the certificate.
	s.agent.conf.setCert(coretesting.CACert, coretesting.CAKey)
	s.agentConfigChanged.Set(0)

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		cert_ := getCert()
		if cert_ == cert {
			continue
		}
		return
	}
	c.Fatal("timed out waiting for the certificate to change")
}

func (s *ManifoldSuite) TestCertUnchanged(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var getCert func() *tls.Certificate
	err := s.manifold.Output(w, &getCert)
	c.Assert(err, jc.ErrorIsNil)

	cert := getCert()
	c.Assert(cert, gc.NotNil)
	c.Assert(cert.Leaf, gc.NotNil)

	// Trigger the watcher, but without changing
	// the cert. The result should be exactly the
	// same.
	s.agentConfigChanged.Set(0)
	time.Sleep(coretesting.ShortWait)

	cert_ := getCert()
	c.Assert(cert, gc.Equals, cert_)
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
	info  *params.StateServingInfo
	addrs []string
}

func (mc *mockConfig) setCert(cert, key string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.info == nil {
		mc.info = &params.StateServingInfo{}
	}
	mc.info.Cert = cert
	mc.info.PrivateKey = key
}

func (mc *mockConfig) StateServingInfo() (params.StateServingInfo, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if mc.info != nil {
		return *mc.info, true
	}
	return params.StateServingInfo{}, false
}
