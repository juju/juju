// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apicaller"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub

	manifold       dependency.Manifold
	manifoldConfig apicaller.ManifoldConfig
	agent          *mockAgent
	conn           *mockConn
	getter         dependency.Getter
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testhelpers.Stub{}
	s.manifoldConfig = apicaller.ManifoldConfig{
		AgentName:            "agent-name",
		APIConfigWatcherName: "api-config-watcher-name",
		APIOpen: func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
			panic("just a fake")
		},
		NewConnection: func(_ context.Context, a agent.Agent, apiOpen api.OpenFunc, logger logger.Logger) (api.Connection, error) {
			c.Check(apiOpen, tc.NotNil) // uncomparable
			c.Check(logger, tc.NotNil)  // uncomparable
			s.AddCall("NewConnection", a)
			if err := s.NextErr(); err != nil {
				return nil, err
			}
			return s.conn, nil
		},
		Filter: func(err error) error {
			panic(err)
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
	s.manifold = apicaller.Manifold(s.manifoldConfig)
	checkFilter := func() {
		s.manifold.Filter(errors.New("arrgh"))
	}
	c.Check(checkFilter, tc.PanicMatches, "arrgh")

	s.agent = &mockAgent{
		stub:   &s.Stub,
		model:  coretesting.ModelTag,
		entity: names.NewMachineTag("42"),
	}
	s.getter = dt.StubGetter(map[string]interface{}{
		"agent-name": s.agent,
	})

	// Watch out for this: it uses its own Stub because Close calls
	// are made from the worker's loop goroutine. You should make
	// sure to stop the worker before checking the mock conn's calls.
	s.conn = &mockConn{
		stub:   &testhelpers.Stub{},
		broken: make(chan struct{}),
	}
}

func (s *ManifoldSuite) TestInputsOptionalConfigPropertiesUnset(c *tc.C) {
	s.manifoldConfig.APIConfigWatcherName = ""
	c.Check(apicaller.Manifold(s.manifoldConfig).Inputs, tc.DeepEquals, []string{
		"agent-name",
	})
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold.Inputs, tc.DeepEquals, []string{
		"agent-name",
		"api-config-watcher-name",
	})
}

func (s *ManifoldSuite) TestStartMissingAgent(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
	s.CheckCalls(c, nil)
}

func (s *ManifoldSuite) TestStartCannotOpenAPI(c *tc.C) {
	s.SetErrors(errors.New("no api for you"))

	worker, err := s.manifold.Start(c.Context(), s.getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, `\[deadbe\] "machine-42" cannot open api: no api for you`)
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "NewConnection",
		Args:     []interface{}{s.agent},
	}})
}

func (s *ManifoldSuite) TestStartSuccess(c *tc.C) {
	worker, err := s.manifold.Start(c.Context(), s.getter)
	c.Check(err, tc.ErrorIsNil)
	defer assertStop(c, worker)
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "NewConnection",
		Args:     []interface{}{s.agent},
	}})
}

func (s *ManifoldSuite) setupWorkerTest(c *tc.C) worker.Worker {
	w, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) { w.Kill() })
	return w
}

func (s *ManifoldSuite) TestKillWorkerClosesConnection(c *tc.C) {
	worker := s.setupWorkerTest(c)
	assertStop(c, worker)
	s.conn.stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Close",
	}})
}

func (s *ManifoldSuite) TestKillWorkerReportsCloseErr(c *tc.C) {
	s.conn.stub.SetErrors(errors.New("bad plumbing"))
	worker := s.setupWorkerTest(c)

	assertStopError(c, worker, "bad plumbing")
	s.conn.stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Close",
	}})
}

func (s *ManifoldSuite) TestBrokenConnectionKillsWorkerWithCloseErr(c *tc.C) {
	s.conn.stub.SetErrors(errors.New("bad plumbing"))
	worker := s.setupWorkerTest(c)

	close(s.conn.broken)
	err := worker.Wait()
	c.Check(err, tc.ErrorMatches, "bad plumbing")
	s.conn.stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Close",
	}})
}

func (s *ManifoldSuite) TestBrokenConnectionKillsWorkerWithFallbackErr(c *tc.C) {
	worker := s.setupWorkerTest(c)

	close(s.conn.broken)
	err := worker.Wait()
	c.Check(err, tc.ErrorMatches, "api connection broken unexpectedly")
	s.conn.stub.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "Close",
	}})
}

func (s *ManifoldSuite) TestOutputSuccess(c *tc.C) {
	worker := s.setupWorkerTest(c)

	var apicaller base.APICaller
	err := s.manifold.Output(worker, &apicaller)
	c.Check(err, tc.ErrorIsNil)
	c.Check(apicaller, tc.Equals, s.conn)

	var conn api.Connection
	err = s.manifold.Output(worker, &conn)
	c.Check(err, tc.ErrorIsNil)
	c.Check(conn, tc.Equals, s.conn)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *tc.C) {
	var apicaller base.APICaller
	err := s.manifold.Output(dummyWorker{}, &apicaller)
	c.Check(apicaller, tc.IsNil)
	c.Check(err.Error(), tc.Equals, "in should be a *apicaller.apiConnWorker; got apicaller_test.dummyWorker")
}

func (s *ManifoldSuite) TestOutputBadTarget(c *tc.C) {
	worker := s.setupWorkerTest(c)

	var apicaller interface{}
	err := s.manifold.Output(worker, &apicaller)
	c.Check(apicaller, tc.IsNil)
	c.Check(err.Error(), tc.Equals, "out should be *base.APICaller or *api.Connection; got *interface {}")
}
