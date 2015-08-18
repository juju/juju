// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/process/workers"
	"github.com/juju/juju/testing"
	workertesting "github.com/juju/juju/worker/testing"
)

type eventHandlerSuite struct {
	testing.BaseSuite

	stub      *gitjujutesting.Stub
	runner    *workertesting.StubRunner
	apiClient *stubAPIClient
}

var _ = gc.Suite(&eventHandlerSuite{})

func (s *eventHandlerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.runner = workertesting.NewStubRunner(s.stub)
	s.apiClient = &stubAPIClient{stub: s.stub}
}

func (s *eventHandlerSuite) handler(events []process.Event, apiClient context.APIClient, runner workers.Runner) error {
	s.stub.AddCall("handler", events, apiClient, runner)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *eventHandlerSuite) TestNewEventHandlers(c *gc.C) {
	eh := workers.NewEventHandlers()
	defer eh.Close()

	// TODO(ericsnow) This test is rather weak.
	c.Check(eh, gc.NotNil)
}

func (s *eventHandlerSuite) TestRegisterHandler(c *gc.C) {
	eh := workers.NewEventHandlers()
	defer eh.Close()
	eh.RegisterHandler(s.handler)

	// TODO(ericsnow) Check something here.
}

func (s *eventHandlerSuite) TestAddEvents(c *gc.C) {
	events := []process.Event{{
		Kind: process.EventKindTracked,
		ID:   "spam/eggs",
	}}
	eh := workers.NewEventHandlers()
	eh.Reset(s.apiClient, s.runner)
	go func() {
		eh.AddEvents(events...)
		eh.Close()
	}()

	var got [][]process.Event
	for event := range workers.ExposeChannel(eh) {
		got = append(got, event)
	}
	c.Check(got, jc.DeepEquals, [][]process.Event{events})
}

func (s *eventHandlerSuite) TestStartEngine(c *gc.C) {
	events := []process.Event{{
		Kind: process.EventKindTracked,
		ID:   "spam/eggs",
	}}

	eh := workers.NewEventHandlers()
	eh.Reset(s.apiClient, s.runner)
	eh.RegisterHandler(s.handler)
	engine, err := eh.StartEngine()
	c.Assert(err, jc.ErrorIsNil)

	eh.AddEvents(events...)

	engine.Kill()
	err = engine.Wait()
	c.Assert(err, jc.ErrorIsNil)
	eh.Close()

	var unhandled [][]process.Event
	for event := range workers.ExposeChannel(eh) {
		unhandled = append(unhandled, event)
	}
	c.Check(unhandled, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "ListProcesses", "handler", "handler")
	c.Check(s.stub.Calls()[1].Args[0], gc.HasLen, 0)
	c.Check(s.stub.Calls()[2].Args[0], gc.DeepEquals, events)
	c.Check(s.stub.Calls()[2].Args[1], gc.DeepEquals, s.apiClient)
	runner, running := workers.ExposeRunner(s.stub.Calls()[2].Args[2].(workers.Runner))
	c.Check(runner, jc.DeepEquals, s.runner)
	c.Check(running.Values(), gc.HasLen, 0)
}

type stubAPIClient struct {
	context.APIClient
	stub *gitjujutesting.Stub
}

func (c *stubAPIClient) ListProcesses(ids ...string) ([]process.Info, error) {
	c.stub.AddCall("ListProcesses", ids)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return nil, nil
}
