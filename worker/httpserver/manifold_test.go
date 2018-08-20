// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/httpserver"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold             dependency.Manifold
	context              dependency.Context
	agent                *mockAgent
	clock                *testing.Clock
	state                stubStateTracker
	prometheusRegisterer stubPrometheusRegisterer
	certWatcher          stubCertWatcher
	authenticator        mockLocalMacaroonAuthenticator
	tlsConfig            *tls.Config

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.clock = testclock.NewClock(time.Time{})
	s.state = stubStateTracker{}
	s.prometheusRegisterer = stubPrometheusRegisterer{}
	s.certWatcher = stubCertWatcher{}
	s.tlsConfig = &tls.Config{}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = httpserver.Manifold(httpserver.ManifoldConfig{
		AgentName:             "agent",
		CertWatcherName:       "cert-watcher",
		ClockName:             "clock",
		StateName:             "state",
		PrometheusRegisterer:  &s.prometheusRegisterer,
		NewStateAuthenticator: s.newStateAuthenticator,
		NewTLSConfig:          s.newTLSConfig,
		NewWorker:             s.newWorker,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":        s.agent,
		"cert-watcher": s.certWatcher.get,
		"clock":        s.clock,
		"state":        &s.state,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newStateAuthenticator(
	statePool *state.StatePool,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
) (httpcontext.LocalMacaroonAuthenticator, error) {
	s.stub.MethodCall(s, "NewStateAuthenticator", statePool, mux, clock, abort)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &s.authenticator, nil
}

func (s *ManifoldSuite) newTLSConfig(st *state.State, getCertificate func() *tls.Certificate) (*tls.Config, error) {
	s.stub.MethodCall(s, "NewTLSConfig", st)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.tlsConfig, nil
}

func (s *ManifoldSuite) newWorker(config httpserver.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{}), nil
}

var expectedInputs = []string{
	"agent", "cert-watcher", "clock", "state",
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewTLSConfig", "NewStateAuthenticator", "NewWorker")
	newWorkerArgs := s.stub.Calls()[2].Args
	c.Assert(newWorkerArgs, gc.HasLen, 1)
	c.Assert(newWorkerArgs[0], gc.FitsTypeOf, httpserver.Config{})
	config := newWorkerArgs[0].(httpserver.Config)

	c.Assert(config, jc.DeepEquals, httpserver.Config{
		AgentConfig:          &s.agent.conf,
		PrometheusRegisterer: &s.prometheusRegisterer,
		TLSConfig:            s.tlsConfig,
		Mux:                  apiserverhttp.NewMux(),
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.state.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.state.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) TestMuxOutput(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var mux *apiserverhttp.Mux
	err := s.manifold.Output(w, &mux)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mux, gc.NotNil)
}

func (s *ManifoldSuite) TestAuthenticatorOutput(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var auth1 httpcontext.Authenticator
	var auth2 httpcontext.LocalMacaroonAuthenticator
	for _, out := range []interface{}{&auth1, &auth2} {
		err := s.manifold.Output(w, out)
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Assert(auth1, gc.NotNil)
	c.Assert(auth1, gc.Equals, auth2)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}
