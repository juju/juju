// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/internal/observability/probe"
	"github.com/juju/juju/internal/worker/caasprober"
)

type ControllerSuite struct{}

var _ = tc.Suite(&ControllerSuite{})

type dummyMux struct {
	AddHandlerFunc    func(string, string, http.Handler) error
	RemoveHandlerFunc func(string, string)
}

func (d *dummyMux) AddHandler(i, j string, h http.Handler) error {
	if d.AddHandlerFunc == nil {
		return nil
	}
	return d.AddHandlerFunc(i, j, h)
}

func (d *dummyMux) RemoveHandler(i, j string) {
	if d.RemoveHandlerFunc != nil {
		d.RemoveHandlerFunc(i, j)
	}
}

func (s *ControllerSuite) TestControllerMuxRegistration(c *tc.C) {
	var (
		livenessRegistered    = false
		livenessDeRegistered  = false
		readinessRegistered   = false
		readinessDeRegistered = false
		startupRegistered     = false
		startupDeRegistered   = false
		waitGroup             = sync.WaitGroup{}
	)

	waitGroup.Add(3)
	mux := dummyMux{
		AddHandlerFunc: func(m, p string, _ http.Handler) error {
			c.Check(m, tc.Equals, http.MethodGet)
			switch p {
			case k8sconstants.AgentHTTPPathLiveness:
				c.Check(livenessRegistered, jc.IsFalse)
				livenessRegistered = true
				waitGroup.Done()
			case k8sconstants.AgentHTTPPathReadiness:
				c.Check(readinessRegistered, jc.IsFalse)
				readinessRegistered = true
				waitGroup.Done()
			case k8sconstants.AgentHTTPPathStartup:
				c.Check(startupRegistered, jc.IsFalse)
				startupRegistered = true
				waitGroup.Done()
			default:
				c.Errorf("unknown path registered in controller: %s", p)
			}
			return nil
		},
		RemoveHandlerFunc: func(m, p string) {
			c.Check(m, tc.Equals, http.MethodGet)
			switch p {
			case k8sconstants.AgentHTTPPathLiveness:
				c.Check(livenessDeRegistered, jc.IsFalse)
				livenessDeRegistered = true
				waitGroup.Done()
			case k8sconstants.AgentHTTPPathReadiness:
				c.Check(readinessDeRegistered, jc.IsFalse)
				readinessDeRegistered = true
				waitGroup.Done()
			case k8sconstants.AgentHTTPPathStartup:
				c.Check(startupDeRegistered, jc.IsFalse)
				startupDeRegistered = true
				waitGroup.Done()
			default:
				c.Errorf("unknown path registered in controller: %s", p)
			}
		},
	}

	probes := caasprober.NewCAASProbes()
	livenessAgg, _ := probes.ProbeAggregate(probe.ProbeLiveness)
	livenessAgg.AddProber("test", probe.NotImplemented)
	readinessAgg, _ := probes.ProbeAggregate(probe.ProbeReadiness)
	readinessAgg.AddProber("test", probe.NotImplemented)
	startupAgg, _ := probes.ProbeAggregate(probe.ProbeStartup)
	startupAgg.AddProber("test", probe.NotImplemented)

	controller, err := caasprober.NewController(probes, &mux)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	waitGroup.Add(3)
	controller.Kill()

	waitGroup.Wait()
	err = controller.Wait()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(livenessRegistered, jc.IsTrue)
	c.Assert(livenessDeRegistered, jc.IsTrue)
	c.Assert(readinessRegistered, jc.IsTrue)
	c.Assert(readinessDeRegistered, jc.IsTrue)
	c.Assert(startupRegistered, jc.IsTrue)
	c.Assert(startupDeRegistered, jc.IsTrue)
}

func (s *ControllerSuite) TestControllerNotImplemented(c *tc.C) {
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	mux := dummyMux{
		AddHandlerFunc: func(m, p string, h http.Handler) error {
			req := httptest.NewRequest(m, p, nil)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req)
			c.Check(recorder.Result().StatusCode, tc.Equals, http.StatusNotImplemented)
			waitGroup.Done()
			return nil
		},
		RemoveHandlerFunc: func(m, p string) {},
	}

	probes := caasprober.NewCAASProbes()
	livenessAgg, _ := probes.ProbeAggregate(probe.ProbeLiveness)
	livenessAgg.AddProber("test", probe.NotImplemented)
	readinessAgg, _ := probes.ProbeAggregate(probe.ProbeReadiness)
	readinessAgg.AddProber("test", probe.NotImplemented)
	startupAgg, _ := probes.ProbeAggregate(probe.ProbeStartup)
	startupAgg.AddProber("test", probe.NotImplemented)

	controller, err := caasprober.NewController(probes, &mux)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	controller.Kill()
	err = controller.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerProbeError(c *tc.C) {
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	mux := dummyMux{
		AddHandlerFunc: func(m, p string, h http.Handler) error {
			req := httptest.NewRequest(m, p, nil)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req)
			c.Check(recorder.Result().StatusCode, tc.Equals, http.StatusInternalServerError)
			waitGroup.Done()
			return nil
		},
		RemoveHandlerFunc: func(m, p string) {},
	}

	probeErr := probe.ProberFn(func() (bool, error) {
		return false, errors.New("test error")
	})

	probes := caasprober.NewCAASProbes()
	livenessAgg, _ := probes.ProbeAggregate(probe.ProbeLiveness)
	livenessAgg.AddProber("test", probeErr)
	readinessAgg, _ := probes.ProbeAggregate(probe.ProbeReadiness)
	readinessAgg.AddProber("test", probeErr)
	startupAgg, _ := probes.ProbeAggregate(probe.ProbeStartup)
	startupAgg.AddProber("test", probeErr)
	controller, err := caasprober.NewController(probes, &mux)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	controller.Kill()
	err = controller.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerProbeFail(c *tc.C) {
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	mux := dummyMux{
		AddHandlerFunc: func(m, p string, h http.Handler) error {
			req := httptest.NewRequest(m, p, nil)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req)
			c.Check(recorder.Result().StatusCode, tc.Equals, http.StatusTeapot)
			waitGroup.Done()
			return nil
		},
		RemoveHandlerFunc: func(m, p string) {},
	}

	probeFail := probe.ProberFn(func() (bool, error) {
		return false, nil
	})

	probes := caasprober.NewCAASProbes()
	livenessAgg, _ := probes.ProbeAggregate(probe.ProbeLiveness)
	livenessAgg.AddProber("test", probeFail)
	readinessAgg, _ := probes.ProbeAggregate(probe.ProbeReadiness)
	readinessAgg.AddProber("test", probeFail)
	startupAgg, _ := probes.ProbeAggregate(probe.ProbeStartup)
	startupAgg.AddProber("test", probeFail)
	controller, err := caasprober.NewController(probes, &mux)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	controller.Kill()
	err = controller.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerProbePass(c *tc.C) {
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	mux := dummyMux{
		AddHandlerFunc: func(m, p string, h http.Handler) error {
			req := httptest.NewRequest(m, p, nil)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req)
			c.Check(recorder.Result().StatusCode, tc.Equals, http.StatusOK)
			waitGroup.Done()
			return nil
		},
		RemoveHandlerFunc: func(m, p string) {},
	}

	probes := caasprober.NewCAASProbes()
	livenessAgg, _ := probes.ProbeAggregate(probe.ProbeLiveness)
	livenessAgg.AddProber("test", probe.Success)
	readinessAgg, _ := probes.ProbeAggregate(probe.ProbeReadiness)
	readinessAgg.AddProber("test", probe.Success)
	startupAgg, _ := probes.ProbeAggregate(probe.ProbeStartup)
	startupAgg.AddProber("test", probe.Success)

	controller, err := caasprober.NewController(probes, &mux)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	controller.Kill()
	err = controller.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerProbePassDetailed(c *tc.C) {
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	mux := dummyMux{
		AddHandlerFunc: func(m, p string, h http.Handler) error {
			req := httptest.NewRequest(m, p+"?detailed=true", nil)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req)
			c.Check(recorder.Result().StatusCode, tc.Equals, http.StatusOK)
			c.Check(recorder.Body.String(), tc.Matches, `(?m)OK: probe (liveness|readiness|startup)\+ test`)
			waitGroup.Done()
			return nil
		},
		RemoveHandlerFunc: func(m, p string) {},
	}

	probes := caasprober.NewCAASProbes()
	livenessAgg, _ := probes.ProbeAggregate(probe.ProbeLiveness)
	livenessAgg.AddProber("test", probe.Success)
	readinessAgg, _ := probes.ProbeAggregate(probe.ProbeReadiness)
	readinessAgg.AddProber("test", probe.Success)
	startupAgg, _ := probes.ProbeAggregate(probe.ProbeStartup)
	startupAgg.AddProber("test", probe.Success)

	controller, err := caasprober.NewController(probes, &mux)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	controller.Kill()
	err = controller.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerSuite) TestControllerProbeFailDetailed(c *tc.C) {
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	mux := dummyMux{
		AddHandlerFunc: func(m, p string, h http.Handler) error {
			req := httptest.NewRequest(m, p+"?detailed=true", nil)
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, req)
			c.Check(recorder.Result().StatusCode, tc.Equals, http.StatusInternalServerError)
			c.Check(recorder.Body.String(), tc.Matches, `(?m)Internal Server Error: probe (liveness|readiness|startup)`)
			waitGroup.Done()
			return nil
		},
		RemoveHandlerFunc: func(m, p string) {},
	}

	probeFail := probe.ProberFn(func() (bool, error) {
		return false, errors.New("test error")
	})

	probes := caasprober.NewCAASProbes()
	livenessAgg, _ := probes.ProbeAggregate(probe.ProbeLiveness)
	livenessAgg.AddProber("test", probeFail)
	readinessAgg, _ := probes.ProbeAggregate(probe.ProbeReadiness)
	readinessAgg.AddProber("test", probeFail)
	startupAgg, _ := probes.ProbeAggregate(probe.ProbeStartup)
	startupAgg.AddProber("test", probeFail)
	controller, err := caasprober.NewController(probes, &mux)
	c.Assert(err, jc.ErrorIsNil)

	waitGroup.Wait()
	controller.Kill()
	err = controller.Wait()
	c.Assert(err, jc.ErrorIsNil)
}
