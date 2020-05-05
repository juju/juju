// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sender_test

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/metricsadder"
	"github.com/juju/juju/worker/metrics/sender"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	factory   spool.MetricFactory
	client    metricsadder.MetricsAdderClient
	apiCaller *stubAPICaller
	manifold  dependency.Manifold
	resources dt.StubResources
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	spoolDir := c.MkDir()
	s.IsolationSuite.SetUpTest(c)
	s.factory = &stubMetricFactory{
		&testing.Stub{},
		spoolDir,
	}

	testAPIClient := func(apiCaller base.APICaller) metricsadder.MetricsAdderClient {
		return newTestAPIMetricSender()
	}
	s.PatchValue(&sender.NewMetricAdderClient, testAPIClient)

	s.manifold = sender.Manifold(sender.ManifoldConfig{
		AgentName:       "agent",
		APICallerName:   "api-caller",
		MetricSpoolName: "metric-spool",
	})

	dataDir := c.MkDir()
	// create unit agent base dir so that hooks can run.
	err := os.MkdirAll(filepath.Join(dataDir, "agents", "unit-u-0"), 0777)
	c.Assert(err, jc.ErrorIsNil)

	s.apiCaller = &stubAPICaller{&testing.Stub{}}
	s.resources = dt.StubResources{
		"agent":        dt.NewStubResource(&dummyAgent{dataDir: dataDir}),
		"api-caller":   dt.NewStubResource(s.apiCaller),
		"metric-spool": dt.NewStubResource(s.factory),
	}
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent", "api-caller", "metric-spool"})
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller":   dependency.ErrMissing,
		"metric-spool": s.factory,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, dependency.ErrMissing.Error())
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":        dependency.ErrMissing,
		"api-caller":   &stubAPICaller{&testing.Stub{}},
		"metric-spool": s.factory,
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, dependency.ErrMissing.Error())
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	s.setupWorkerTest(c)
}

func (s *ManifoldSuite) setupWorkerTest(c *gc.C) worker.Worker {
	worker, err := s.manifold.Start(s.resources.Context())
	c.Check(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		worker.Kill()
		err := worker.Wait()
		c.Check(err, jc.ErrorIsNil)
	})
	return worker
}

type mockListener struct {
	testing.Stub
	spool.ConnectionHandler
}

// Stop implements the stopper interface.
func (l *mockListener) Stop() error {
	l.AddCall("Stop")
	return nil
}

func (s *ManifoldSuite) TestWorkerErrorStopsSender(c *gc.C) {
	listener := &mockListener{}
	s.PatchValue(sender.NewListener, sender.NewListenerFunc(listener))

	s.apiCaller.SetErrors(errors.New("blah"))

	worker, err := s.manifold.Start(s.resources.Context())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	err = worker.Wait()
	c.Assert(err, gc.ErrorMatches, ".*blah")
	listener.CheckCallNames(c, "Stop")
}

var _ base.APICaller = (*stubAPICaller)(nil)

type stubAPICaller struct {
	*testing.Stub
}

func (s *stubAPICaller) APICall(objType string, version int, id, request string, params, response interface{}) error {
	s.MethodCall(s, "APICall", objType, version, id, request, params, response)
	return s.NextErr()
}

func (s *stubAPICaller) BestFacadeVersion(facade string) int {
	s.MethodCall(s, "BestFacadeVersion", facade)
	return 42
}

func (s *stubAPICaller) ModelTag() (names.ModelTag, bool) {
	s.MethodCall(s, "ModelTag")
	return names.NewModelTag("foobar"), true
}

func (s *stubAPICaller) ConnectStream(string, url.Values) (base.Stream, error) {
	panic("should not be called")
}

func (s *stubAPICaller) ConnectControllerStream(string, url.Values, http.Header) (base.Stream, error) {
	panic("should not be called")
}

func (s *stubAPICaller) HTTPClient() (*httprequest.Client, error) {
	panic("should not be called")
}

func (s *stubAPICaller) Context() context.Context {
	return context.Background()
}

func (s *stubAPICaller) BakeryClient() base.MacaroonDischarger {
	panic("should not be called")
}

type dummyAgent struct {
	agent.Agent
	dataDir string
}

func (a dummyAgent) CurrentConfig() agent.Config {
	return &dummyAgentConfig{dataDir: a.dataDir}
}

type dummyAgentConfig struct {
	agent.Config
	dataDir string
}

// Tag implements agent.AgentConfig.
func (ac dummyAgentConfig) Tag() names.Tag {
	return names.NewUnitTag("u/0")
}

// DataDir implements agent.AgentConfig.
func (ac dummyAgentConfig) DataDir() string {
	return ac.dataDir
}
