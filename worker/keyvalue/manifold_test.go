// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyvalue_test

import (
	"path"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/keyvalue"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	manifold    dependency.Manifold
	getResource dependency.GetResourceFunc
	path        string
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.manifold = keyvalue.Manifold(keyvalue.ManifoldConfig{
		AgentName: "agent-name",
	})
	s.path = c.MkDir()
	s.getResource = dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: &dummyAgent{dataDir: s.path}},
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
	var getter keyvalue.Getter
	var setter keyvalue.Setter
	err := s.manifold.Output(worker, &setter)
	c.Check(err, jc.ErrorIsNil)
	err = s.manifold.Output(worker, &getter)
	c.Check(err, jc.ErrorIsNil)

	err = setter.Set("charm-url", "cs:wordpress-42")
	c.Assert(err, jc.ErrorIsNil)

	var yamlState map[string]interface{}
	err = utils.ReadYaml(path.Join(s.path, "uniter_keyvalue.yaml"), &yamlState)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(yamlState["charm-url"], gc.Equals, "cs:wordpress-42")

	charmURL, err := getter.Get("charm-url")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmURL, gc.Equals, "cs:wordpress-42")

	err = setter.Set("charm-url", "cs:wordpress-43")
	c.Assert(err, jc.ErrorIsNil)

	charmURL, err = getter.Get("charm-url")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmURL, gc.Equals, "cs:wordpress-43")
}

func (s *ManifoldSuite) TestOutputIfStateNotSet(c *gc.C) {
	worker := s.setupWorkerTest(c)
	var getter keyvalue.Getter
	err := s.manifold.Output(worker, &getter)
	c.Check(err, jc.ErrorIsNil)

	charmURL, err := getter.Get("charm-url")
	c.Assert(err, gc.NotNil)
	c.Assert(keyvalue.IsNotSetError(err), gc.Equals, true)
	c.Assert(charmURL, gc.Equals, nil)
}

func (s *ManifoldSuite) setupWorkerTest(c *gc.C) worker.Worker {
	worker, err := s.manifold.Start(s.getResource)
	c.Check(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		worker.Kill()
		err := worker.Wait()
		c.Assert(err, jc.ErrorIsNil)
	})
	return worker
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	worker := s.setupWorkerTest(c)
	c.Assert(worker, gc.NotNil)
	var keyvaluePlaceholder interface{}
	err := s.manifold.Output(worker, &keyvaluePlaceholder)
	c.Assert(err.Error(), gc.Equals, "expected *keyvalue.keyValueWorker-> *keyvalue.Setter or *keyvalue.Getter, got *keyvalue.keyValueWorker->*interface {}")
	c.Assert(keyvaluePlaceholder, gc.IsNil)
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

func (ac dummyAgentConfig) DataDir() string {
	return ac.dataDir
}
