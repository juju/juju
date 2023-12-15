// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/syslogger"
)

type ManifoldSuite struct {
	manifold dependency.Manifold
	context  dependency.Context
	worker   *mockWorker
	stub     testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.stub.ResetCalls()

	s.worker = &mockWorker{}
	s.context = s.newContext(nil)
	s.manifold = syslogger.Manifold(syslogger.ManifoldConfig{
		NewWorker: s.newWorker,
		NewLogger: s.newLogger,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config syslogger.WorkerConfig) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *ManifoldSuite) newLogger(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
	s.stub.MethodCall(s, "NewLogger", priority, tag)
	return nil, nil
}

var expectedInputs = []string{}

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
	c.Assert(args[0], gc.FitsTypeOf, syslogger.WorkerConfig{})
	config := args[0].(syslogger.WorkerConfig)

	c.Assert(config.NewLogger, gc.NotNil)
	config.NewLogger = nil

	c.Assert(config, jc.DeepEquals, syslogger.WorkerConfig{})
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	w := s.startWorkerClean(c)

	var logger syslogger.SysLogger
	err := s.manifold.Output(w, &logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, s.worker)
	return w
}

type mockWorker struct {
	worker.Worker
	testing.Stub
}

func (r *mockWorker) Log(logs []corelogger.LogRecord) error {
	r.MethodCall(r, "Log", logs)
	return r.NextErr()
}

func (r *mockWorker) Kill() {
	r.MethodCall(r, "Kill")
}

func (r *mockWorker) Wait() error {
	r.MethodCall(r, "Wait")
	return r.NextErr()
}
