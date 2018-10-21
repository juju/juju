// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs_test

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
	"github.com/juju/juju/worker/httpserverargs"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	config        httpserverargs.ManifoldConfig
	manifold      dependency.Manifold
	context       dependency.Context
	clock         *testclock.Clock
	state         stubStateTracker
	authenticator mockLocalMacaroonAuthenticator
	tlsConfig     *tls.Config

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Time{})
	s.state = stubStateTracker{}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.config = httpserverargs.ManifoldConfig{
		ClockName:             "clock",
		StateName:             "state",
		NewStateAuthenticator: s.newStateAuthenticator,
	}
	s.manifold = httpserverargs.Manifold(s.config)
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"clock": s.clock,
		"state": &s.state,
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

var expectedInputs = []string{"state", "clock"}

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

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.state.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.state.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) TestStoppingWorkerClosesAuthenticator(c *gc.C) {
	w := s.startWorkerClean(c)
	s.stub.CheckCallNames(c, "NewStateAuthenticator")
	authArgs := s.stub.Calls()[0].Args
	c.Assert(authArgs, gc.HasLen, 4)
	abort := authArgs[3].(<-chan struct{})

	// abort should still be open at this point.
	select {
	case <-abort:
		c.Fatalf("abort closed while worker still running")
	default:
	}

	workertest.CleanKill(c, w)
	select {
	case <-abort:
	default:
		c.Fatalf("authenticator abort channel not closed")
	}
}

func (s *ManifoldSuite) TestValidate(c *gc.C) {
	type test struct {
		f      func(*httpserverargs.ManifoldConfig)
		expect string
	}
	tests := []test{{
		func(cfg *httpserverargs.ManifoldConfig) { cfg.StateName = "" },
		"empty StateName not valid",
	}, {
		func(cfg *httpserverargs.ManifoldConfig) { cfg.ClockName = "" },
		"empty ClockName not valid",
	}, {
		func(cfg *httpserverargs.ManifoldConfig) { cfg.NewStateAuthenticator = nil },
		"nil NewStateAuthenticator not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		config := s.config
		test.f(&config)
		manifold := httpserverargs.Manifold(config)
		w, err := manifold.Start(s.context)
		workertest.CheckNilOrKill(c, w)
		c.Check(err, gc.ErrorMatches, test.expect)
	}
}

type mockLocalMacaroonAuthenticator struct {
	httpcontext.LocalMacaroonAuthenticator
}

type stubStateTracker struct {
	testing.Stub
	pool state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return &s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}
