// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold    dependency.Manifold
	agent       *mockAgent
	gate        *mockGate
	conn        *mockConn
	getResource dependency.GetResourceFunc
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.manifold = apicaller.Manifold(apicaller.ManifoldConfig{
		AgentName:       "agent-name",
		APIInfoGateName: "api-info-gate-name",
	})

	s.agent = &mockAgent{
		stub: &s.Stub,
		env:  coretesting.EnvironmentTag,
	}
	s.gate = &mockGate{
		stub: &s.Stub,
	}
	s.getResource = dt.StubGetResource(dt.StubResources{
		"agent-name":         dt.StubResource{Output: s.agent},
		"api-info-gate-name": dt.StubResource{Output: s.gate},
	})

	// Watch out for this: it uses its own Stub because Close calls are made from
	// the worker's loop goroutine. You should make sure to stop the worker before
	// checking the mock conn's calls (unless you know the connection will outlive
	// the test -- see setupMutatorTest).
	s.conn = &mockConn{
		stub:   &testing.Stub{},
		broken: make(chan struct{}),
	}
	s.PatchValue(apicaller.OpenConnection, func(a agent.Agent) (api.Connection, error) {
		s.AddCall("openConnection", a)
		if err := s.NextErr(); err != nil {
			return nil, err
		}
		return s.conn, nil
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name", "api-info-gate-name"})
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":         dt.StubResource{Error: dependency.ErrMissing},
		"api-info-gate-name": dt.StubResource{Output: s.gate},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	s.CheckCalls(c, nil)
}

func (s *ManifoldSuite) TestStartMissingGate(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":         dt.StubResource{Output: s.agent},
		"api-info-gate-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	s.CheckCalls(c, nil)
}

func (s *ManifoldSuite) TestStartCannotOpenAPI(c *gc.C) {
	s.SetErrors(errors.New("no api for you"))

	worker, err := s.manifold.Start(s.getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot open api: no api for you")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "openConnection",
		Args:     []interface{}{s.agent},
	}})
}

func (s *ManifoldSuite) TestStartSuccessWithEnvironnmentIdSet(c *gc.C) {
	worker, err := s.manifold.Start(s.getResource)
	c.Check(err, jc.ErrorIsNil)
	defer assertStop(c, worker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "openConnection",
		Args:     []interface{}{s.agent},
	}, {
		FuncName: "Unlock",
	}})
}

func (s *ManifoldSuite) setupMutatorTest(c *gc.C) agent.ConfigMutator {
	s.agent.env = names.EnvironTag{}
	s.conn.stub = &s.Stub // will be unsafe if worker stopped before test finished
	s.SetErrors(
		nil, //                                               openConnection,
		errors.New("nonfatal: always logged and ignored"), // ChangeConfig
	)

	worker, err := s.manifold.Start(s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { assertStop(c, worker) })

	s.CheckCallNames(c, "openConnection", "ChangeConfig", "Unlock")
	changeArgs := s.Calls()[1].Args
	c.Assert(changeArgs, gc.HasLen, 1)
	s.ResetCalls()
	return changeArgs[0].(agent.ConfigMutator)
}

func (s *ManifoldSuite) TestStartSuccessWithEnvironnmentIdNotSet(c *gc.C) {
	mutator := s.setupMutatorTest(c)
	mockSetter := &mockSetter{stub: &s.Stub}

	err := mutator(mockSetter)
	c.Check(err, jc.ErrorIsNil)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "EnvironTag",
	}, {
		FuncName: "Migrate",
		Args: []interface{}{agent.MigrateParams{
			Environment: coretesting.EnvironmentTag,
		}},
	}})
}

func (s *ManifoldSuite) TestStartSuccessWithEnvironnmentIdNotSetBadAPIState(c *gc.C) {
	mutator := s.setupMutatorTest(c)
	s.SetErrors(errors.New("no tag for you"))

	err := mutator(nil)
	c.Check(err, gc.ErrorMatches, "no environment uuid set on api: no tag for you")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "EnvironTag",
	}})
}

func (s *ManifoldSuite) TestStartSuccessWithEnvironnmentIdNotSetMigrateFailure(c *gc.C) {
	mutator := s.setupMutatorTest(c)
	mockSetter := &mockSetter{stub: &s.Stub}
	s.SetErrors(nil, errors.New("migrate failure"))

	err := mutator(mockSetter)
	c.Check(err, gc.ErrorMatches, "migrate failure")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "EnvironTag",
	}, {
		FuncName: "Migrate",
		Args: []interface{}{agent.MigrateParams{
			Environment: coretesting.EnvironmentTag,
		}},
	}})
}

func (s *ManifoldSuite) setupWorkerTest(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.getResource)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { w.Kill() })
	return w
}

func (s *ManifoldSuite) TestKillWorkerClosesConnection(c *gc.C) {
	worker := s.setupWorkerTest(c)
	assertStop(c, worker)
	s.conn.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Close",
	}})
}

func (s *ManifoldSuite) TestKillWorkerReportsCloseErr(c *gc.C) {
	s.conn.stub.SetErrors(errors.New("bad plumbing"))
	worker := s.setupWorkerTest(c)

	assertStopError(c, worker, "bad plumbing")
	s.conn.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Close",
	}})
}

func (s *ManifoldSuite) TestBrokenConnectionKillsWorkerWithCloseErr(c *gc.C) {
	s.conn.stub.SetErrors(errors.New("bad plumbing"))
	worker := s.setupWorkerTest(c)

	close(s.conn.broken)
	err := worker.Wait()
	c.Check(err, gc.ErrorMatches, "bad plumbing")
	s.conn.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Close",
	}})
}

func (s *ManifoldSuite) TestBrokenConnectionKillsWorkerWithFallbackErr(c *gc.C) {
	worker := s.setupWorkerTest(c)

	close(s.conn.broken)
	err := worker.Wait()
	c.Check(err, gc.ErrorMatches, "api connection broken unexpectedly")
	s.conn.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Close",
	}})
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	worker := s.setupWorkerTest(c)

	var apicaller base.APICaller
	err := s.manifold.Output(worker, &apicaller)
	c.Check(err, jc.ErrorIsNil)
	c.Check(apicaller, gc.Equals, s.conn)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var apicaller base.APICaller
	err := s.manifold.Output(dummyWorker{}, &apicaller)
	c.Check(apicaller, gc.IsNil)
	c.Check(err.Error(), gc.Equals, "expected *apicaller.apiConnWorker->*base.APICaller; got apicaller_test.dummyWorker->*base.APICaller")
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	worker := s.setupWorkerTest(c)

	var apicaller interface{}
	err := s.manifold.Output(worker, &apicaller)
	c.Check(apicaller, gc.IsNil)
	c.Check(err.Error(), gc.Equals, "expected *apicaller.apiConnWorker->*base.APICaller; got *apicaller.apiConnWorker->*interface {}")
}
