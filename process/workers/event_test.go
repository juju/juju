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
	apiClient context.APIClient // TODO(ericsnow) Use a stub.
}

var _ = gc.Suite(&eventHandlerSuite{})

func (s *eventHandlerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.runner = workertesting.NewStubRunner(s.stub)
}

func (s *eventHandlerSuite) handler(events []process.Event, apiClient context.APIClient, runner workers.Runner) error {
	s.stub.AddCall("handler", events, apiClient, runner)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *eventHandlerSuite) TestNewEventHandlers(c *gc.C) {
	eh := workers.NewEventHandlers(s.apiClient, s.runner)
	defer eh.Close()

	// TODO(ericsnow) This test is rather weak.
	c.Check(eh, gc.NotNil)
}

func (s *eventHandlerSuite) TestRegisterHandler(c *gc.C) {
	eh := workers.NewEventHandlers(s.apiClient, s.runner)
	defer eh.Close()
	eh.RegisterHandler(s.handler)

	// TODO(ericsnow) Check something here.
}

func (s *eventHandlerSuite) TestAddEvents(c *gc.C) {
	events := []process.Event{{
		Kind: process.EventKindTracked,
		ID:   "spam/eggs",
	}}
	eh := workers.NewEventHandlers(s.apiClient, s.runner)
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

func (s *eventHandlerSuite) TestNewWorker(c *gc.C) {
	events := []process.Event{{
		Kind: process.EventKindTracked,
		ID:   "spam/eggs",
	}}
	eh := workers.NewEventHandlers(s.apiClient, s.runner)
	eh.RegisterHandler(s.handler)
	w, err := eh.NewWorker()
	c.Assert(err, jc.ErrorIsNil)

	eh.AddEvents(events...)

	w.Kill()
	err = w.Wait()
	c.Assert(err, jc.ErrorIsNil)
	eh.Close()

	var unhandled [][]process.Event
	for event := range workers.ExposeChannel(eh) {
		unhandled = append(unhandled, event)
	}
	c.Check(unhandled, gc.HasLen, 0)
	s.stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "handler",
		Args:     []interface{}{events, s.apiClient, s.runner},
	}})
}
