// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sender_test

import (
	"net/url"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/httprequest"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/metricsadder"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/metrics/sender"
	"github.com/juju/juju/worker/metrics/spool"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	factory     spool.MetricFactory
	client      metricsadder.MetricsAdderClient
	manifold    dependency.Manifold
	getResource dependency.GetResourceFunc
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
		APICallerName:   "api-caller",
		MetricSpoolName: "metric-spool",
	})
	s.getResource = dt.StubGetResource(dt.StubResources{
		"api-caller":   dt.StubResource{Output: &stubAPICaller{&testing.Stub{}}},
		"metric-spool": dt.StubResource{Output: s.factory},
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"api-caller", "metric-spool"})
}

func (s *ManifoldSuite) TestStartMissingAPICaller(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller":   dt.StubResource{Error: dependency.ErrMissing},
		"metric-spool": dt.StubResource{Output: s.factory},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, dependency.ErrMissing.Error())
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	s.setupWorkerTest(c)
}

func (s *ManifoldSuite) setupWorkerTest(c *gc.C) worker.Worker {
	worker, err := s.manifold.Start(s.getResource)
	c.Check(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		worker.Kill()
		err := worker.Wait()
		c.Check(err, jc.ErrorIsNil)
	})
	return worker
}

var _ base.APICaller = (*stubAPICaller)(nil)

type stubAPICaller struct {
	*testing.Stub
}

func (s *stubAPICaller) APICall(objType string, version int, id, request string, params, response interface{}) error {
	s.MethodCall(s, "APICall", objType, version, id, request, params, response)
	return nil
}

func (s *stubAPICaller) BestFacadeVersion(facade string) int {
	s.MethodCall(s, "BestFacadeVersion", facade)
	return 42
}

func (s *stubAPICaller) EnvironTag() (names.EnvironTag, error) {
	s.MethodCall(s, "EnvironTag")
	return names.NewEnvironTag("foobar"), nil
}

func (s *stubAPICaller) ConnectStream(string, url.Values) (base.Stream, error) {
	panic("should not be called")
}

func (s *stubAPICaller) HTTPClient() (*httprequest.Client, error) {
	panic("should not be called")
}
