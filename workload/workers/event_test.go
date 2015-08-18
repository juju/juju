// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
	"github.com/juju/juju/workload/workers"
)

type eventHandlerSuite struct {
	testing.BaseSuite

	stub      *gitjujutesting.Stub
	apiClient *stubAPIClient
}

var _ = gc.Suite(&eventHandlerSuite{})

func (s *eventHandlerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.apiClient = &stubAPIClient{stub: s.stub}
}

func (s *eventHandlerSuite) handler(events []workload.Event, apiClient context.APIClient, runner workers.Runner) error {
	s.stub.AddCall("handler", events, apiClient, runner)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *eventHandlerSuite) checkUnhandled(c *gc.C, eh *workers.EventHandlers, expected ...[]workload.Event) {
	eventsChan, _, _, _ := workers.ExposeEventHandlers(eh)

	var unhandled [][]workload.Event
	for events := range eventsChan {
		unhandled = append(unhandled, events)
	}
	c.Check(unhandled, jc.DeepEquals, expected)
}

func (s *eventHandlerSuite) checkRegistered(c *gc.C, eh *workers.EventHandlers, expected ...string) {
	_, handlers, _, _ := workers.ExposeEventHandlers(eh)
	for _, handler := range handlers {
		err := handler(nil, s.apiClient, nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Check(s.stub.Calls(), gc.HasLen, len(expected))
	s.stub.CheckCallNames(c, expected...)
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

	s.checkRegistered(c, eh, "handler")
}

func (s *eventHandlerSuite) TestAddEventsOkay(c *gc.C) {
	events := []workload.Event{{
		Kind: workload.EventKindTracked,
		ID:   "spam/eggs",
	}}
	eh := workers.NewEventHandlers()
	eh.Reset(s.apiClient)
	go func() {
		eh.AddEvents(events...)
		eh.Close()
	}()

	s.checkUnhandled(c, eh, events)
}

func (s *eventHandlerSuite) TestAddEventsEmpty(c *gc.C) {
	eh := workers.NewEventHandlers()
	eh.Reset(s.apiClient)
	go func() {
		eh.AddEvents()
		eh.Close()
	}()

	s.checkUnhandled(c, eh)
}

func (s *eventHandlerSuite) TestStartEngine(c *gc.C) {
	events := []workload.Event{{
		Kind: workload.EventKindTracked,
		ID:   "spam/eggs",
	}}

	eh := workers.NewEventHandlers()
	eh.Reset(s.apiClient)
	eh.RegisterHandler(s.handler)
	engine, err := eh.StartEngine()
	c.Assert(err, jc.ErrorIsNil)

	eh.AddEvents(events...)

	engine.Kill()
	err = engine.Wait()
	c.Assert(err, jc.ErrorIsNil)
	eh.Close()

	s.checkUnhandled(c, eh)
	s.stub.CheckCallNames(c, "List", "handler")
	c.Check(s.stub.Calls()[1].Args[0], gc.DeepEquals, events)
	c.Check(s.stub.Calls()[1].Args[1], gc.DeepEquals, s.apiClient)
	// TODO(ericsnow) Check the runner.
}

type stubAPIClient struct {
	context.APIClient
	stub *gitjujutesting.Stub
}

func (c *stubAPIClient) List(ids ...string) ([]workload.Info, error) {
	c.stub.AddCall("List", ids)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return nil, nil
}
