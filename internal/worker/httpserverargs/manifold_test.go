// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	accessservice "github.com/juju/juju/domain/access/service"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	macaroonservice "github.com/juju/juju/domain/macaroon/service"
	modelservice "github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/httpserverargs"
	"github.com/juju/juju/state"
)

type ManifoldSuite struct {
	config         httpserverargs.ManifoldConfig
	manifold       dependency.Manifold
	getter         dependency.Getter
	clock          *testclock.Clock
	stateTracker   stubStateTracker
	authenticator  mockLocalMacaroonAuthenticator
	domainServices stubDomainServices

	stub testhelpers.Stub
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.clock = testclock.NewClock(time.Time{})
	s.stateTracker = stubStateTracker{}
	s.domainServices = stubDomainServices{}
	s.stub.ResetCalls()

	s.getter = s.newGetter(nil)
	s.config = httpserverargs.ManifoldConfig{
		ClockName:             "clock",
		StateName:             "state",
		DomainServicesName:    "domain-services",
		NewStateAuthenticator: s.newStateAuthenticator,
	}
	s.manifold = httpserverargs.Manifold(s.config)
}

func (s *ManifoldSuite) newGetter(overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"clock":           s.clock,
		"state":           &s.stateTracker,
		"domain-services": &s.domainServices,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

func (s *ManifoldSuite) newStateAuthenticator(
	ctx context.Context,
	statePool *state.StatePool,
	controllerConfig httpserverargs.ControllerConfigService,
	agentPasswordServiceGetter httpserverargs.AgentPasswordServiceGetter,
	accessService httpserverargs.AccessService,
	modelService httpserverargs.ModelService,
	macaroonService httpserverargs.MacaroonService,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
) (macaroon.LocalMacaroonAuthenticator, error) {
	s.stub.MethodCall(s, "NewStateAuthenticator", ctx, "")
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return &s.authenticator, nil
}

var expectedInputs = []string{"state", "clock", "domain-services"}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *tc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]any{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(c.Context(), getter)
		c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestMuxOutput(c *tc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var mux *apiserverhttp.Mux
	err := s.manifold.Output(w, &mux)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(mux, tc.NotNil)
}

func (s *ManifoldSuite) TestAuthenticatorOutput(c *tc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var auth1 authentication.RequestAuthenticator
	var auth2 macaroon.LocalMacaroonAuthenticator
	for _, out := range []any{&auth1, &auth2} {
		err := s.manifold.Output(w, out)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Assert(auth1, tc.NotNil)
	c.Assert(auth1, tc.Equals, auth2)
}

func (s *ManifoldSuite) startWorkerClean(c *tc.C) worker.Worker {
	w, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *tc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.stateTracker.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) TestStoppingWorkerClosesAuthenticator(c *tc.C) {
	w := s.startWorkerClean(c)
	s.stub.CheckCallNames(c, "NewStateAuthenticator")
	authArgs := s.stub.Calls()[0].Args
	c.Assert(authArgs, tc.HasLen, 2)
	ctx := authArgs[0].(context.Context)

	// abort should still be open at this point.
	select {
	case <-ctx.Done():
		c.Fatalf("abort closed while worker still running")
	default:
	}

	workertest.CleanKill(c, w)
	select {
	case <-ctx.Done():
	default:
		c.Fatalf("authenticator abort channel not closed")
	}
}

func (s *ManifoldSuite) TestValidate(c *tc.C) {
	type test struct {
		f      func(*httpserverargs.ManifoldConfig)
		expect string
	}
	tests := []test{{
		f:      func(cfg *httpserverargs.ManifoldConfig) { cfg.StateName = "" },
		expect: "empty StateName not valid",
	}, {
		f:      func(cfg *httpserverargs.ManifoldConfig) { cfg.ClockName = "" },
		expect: "empty ClockName not valid",
	}, {
		f:      func(cfg *httpserverargs.ManifoldConfig) { cfg.DomainServicesName = "" },
		expect: "empty DomainServicesName not valid",
	}, {
		f:      func(cfg *httpserverargs.ManifoldConfig) { cfg.NewStateAuthenticator = nil },
		expect: "nil NewStateAuthenticator not valid",
	}}

	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)

		config := s.config
		test.f(&config)

		manifold := httpserverargs.Manifold(config)
		w, err := manifold.Start(c.Context(), s.getter)
		workertest.CheckNilOrKill(c, w)
		c.Check(err, tc.ErrorMatches, test.expect)
	}
}

type mockLocalMacaroonAuthenticator struct {
	macaroon.LocalMacaroonAuthenticator
}

type stubStateTracker struct {
	testhelpers.Stub
	pool  *state.StatePool
	state *state.State
}

func (s *stubStateTracker) Use() (*state.StatePool, *state.State, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.state, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

func (s *stubStateTracker) Report() map[string]any {
	s.MethodCall(s, "Report")
	return nil
}

type stubDomainServices struct {
	testhelpers.Stub
	services.ControllerDomainServices
	services.DomainServicesGetter
}

func (s *stubDomainServices) ControllerConfig() *controllerconfigservice.WatchableService {
	s.MethodCall(s, "ControllerConfig")
	return nil
}

func (s *stubDomainServices) Access() *accessservice.Service {
	s.MethodCall(s, "Access")
	return nil
}

func (s *stubDomainServices) Macaroon() *macaroonservice.Service {
	s.MethodCall(s, "Macaroon")
	return nil
}

func (s *stubDomainServices) Model() *modelservice.WatchableService {
	s.MethodCall(s, "Model")
	return nil
}
