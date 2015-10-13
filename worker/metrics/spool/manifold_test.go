// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spool_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/metrics/spool"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	factory     *stubFactory
	manifold    dependency.Manifold
	getResource dependency.GetResourceFunc
	spoolDir    string
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.factory = &stubFactory{}
	s.PatchValue(spool.NewFactory, s.factory.newFactory)
	s.manifold = spool.Manifold(spool.ManifoldConfig{
		AgentName: "agent-name",
	})
	s.spoolDir = c.MkDir()
	s.getResource = dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: &dummyAgent{spoolDir: s.spoolDir}},
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name"})
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Error: dependency.ErrMissing},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	s.setupWorkerTest(c)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	worker := s.setupWorkerTest(c)
	var factory spool.MetricFactory
	err := s.manifold.Output(worker, &factory)
	c.Check(err, jc.ErrorIsNil)
	s.factory.CheckCall(c, 0, "newFactory", s.spoolDir)
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

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	worker := s.setupWorkerTest(c)
	var spoolDirPlaceholder interface{}
	err := s.manifold.Output(worker, &spoolDirPlaceholder)
	c.Check(err.Error(), gc.Equals, "expected *spool.spoolWorker->*spool.MetricFactory; got *spool.spoolWorker->*interface {}")
	c.Check(spoolDirPlaceholder, gc.IsNil)
}

func (s *ManifoldSuite) TestCannotCreateSpoolDir(c *gc.C) {
	c.Assert(ioutil.WriteFile(filepath.Join(s.spoolDir, "x"), nil, 0666), jc.ErrorIsNil)
	spoolDir := filepath.Join(s.spoolDir, "x", "y")
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: &dummyAgent{spoolDir: spoolDir}},
	})
	w, err := s.manifold.Start(getResource)
	c.Check(err, gc.ErrorMatches, ".*error checking spool directory.*")

	var factory spool.MetricFactory
	err = s.manifold.Output(w, &factory)
	c.Check(err.Error(), gc.Equals, "expected *spool.spoolWorker->*spool.MetricFactory; got <nil>->*spool.MetricFactory")
}

type dummyAgent struct {
	agent.Agent
	spoolDir string
}

func (a dummyAgent) CurrentConfig() agent.Config {
	return &dummyAgentConfig{spoolDir: a.spoolDir}
}

type dummyAgentConfig struct {
	agent.Config
	spoolDir string
}

func (ac dummyAgentConfig) MetricsSpoolDir() string {
	return ac.spoolDir
}

type dummyFactory struct {
	spool.MetricFactory
}

type stubFactory struct {
	testing.Stub
}

func (s *stubFactory) newFactory(spoolDir string) spool.MetricFactory {
	s.AddCall("newFactory", spoolDir)
	return &dummyFactory{}
}
