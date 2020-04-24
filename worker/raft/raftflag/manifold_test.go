// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftflag_test

import (
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/raft/raftflag"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold
	context  dependency.Context
	raft     *raft.Raft
	worker   *mockWorker
	stub     testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.raft = &raft.Raft{}
	s.worker = &mockWorker{}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = raftflag.Manifold(raftflag.ManifoldConfig{
		RaftName:  "raft",
		NewWorker: s.newWorker,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"raft": s.raft,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config raftflag.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

var expectedInputs = []string{"raft"}

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
	s.startWorkerClean(c)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], jc.DeepEquals, raftflag.Config{
		Raft: s.raft,
	})
}

func (s *ManifoldSuite) TestErrRefresh(c *gc.C) {
	w := s.startWorkerClean(c)

	s.worker.SetErrors(raftflag.ErrRefresh)
	err := w.Wait()
	c.Assert(err, gc.Equals, dependency.ErrBounce)
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	s.startWorkerClean(c)

	var flag engine.Flag
	err := s.manifold.Output(s.worker, &flag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(flag, gc.Equals, s.worker)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

type mockWorker struct {
	testing.Stub
	worker.Worker
	engine.Flag
}

func (w *mockWorker) Wait() error {
	return w.NextErr()
}
