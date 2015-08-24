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

type eventsSuite struct {
	testing.BaseSuite

	stub      *gitjujutesting.Stub
	apiClient *stubAPIClient
}

var _ = gc.Suite(&eventsSuite{})

func (s *eventsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.apiClient = &stubAPIClient{stub: s.stub}
}

func (s *eventsSuite) TestAddEventsOkay(c *gc.C) {
	events := []workload.Event{{
		Kind: workload.EventKindTracked,
		ID:   "spam/eggs",
	}}
	e := workers.NewEvents()
	go func() {
		e.AddEvents(events...)
		e.Close()
	}()

	checkUnhandledEvents(c, e, events)
}

func (s *eventsSuite) TestAddEventsEmpty(c *gc.C) {
	e := workers.NewEvents()
	go func() {
		e.AddEvents()
		e.Close()
	}()

	checkUnhandledEvents(c, e)
}

func checkUnhandledEvents(c *gc.C, e *workers.Events, expected ...[]workload.Event) {
	eventsChan := workers.ExposeEvents(e)

	var unhandled [][]workload.Event
	for events := range eventsChan {
		unhandled = append(unhandled, events)
	}
	c.Check(unhandled, jc.DeepEquals, expected)
}

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

func (s *eventHandlerSuite) checkRegistered(c *gc.C, eh *workers.EventHandlers, expected ...string) {
	data := workers.ExposeEventHandlers(eh)
	for _, handler := range data.Handlers {
		err := handler(nil, s.apiClient, nil)
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Check(s.stub.Calls(), gc.HasLen, len(expected))
	s.stub.CheckCallNames(c, expected...)
}

func (s *eventHandlerSuite) TestNewEventHandlers(c *gc.C) {
	eh := workers.NewEventHandlers()
	c.Assert(eh, gc.NotNil)
	eh.Close()

	s.checkRegistered(c, eh)
	data := workers.ExposeEventHandlers(eh)
	checkUnhandledEvents(c, data.Events)
	c.Check(data.APIClient, gc.IsNil)
	c.Check(data.Runner, gc.IsNil)
}

func (s *eventHandlerSuite) TestReset(c *gc.C) {
	eh := workers.NewEventHandlers()
	c.Assert(eh, gc.NotNil)
	err := eh.Reset(s.apiClient)
	c.Assert(err, jc.ErrorIsNil)
	eh.Close()

	s.checkRegistered(c, eh)
	data := workers.ExposeEventHandlers(eh)
	checkUnhandledEvents(c, data.Events)
	c.Check(data.APIClient, gc.Equals, s.apiClient)
	c.Check(data.Runner, gc.IsNil)
	s.stub.CheckCalls(c, nil)
}

func (s *eventHandlerSuite) TestCloseFresh(c *gc.C) {
	eh := workers.NewEventHandlers()
	c.Assert(eh, gc.NotNil)

	err := eh.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.checkRegistered(c, eh)
	data := workers.ExposeEventHandlers(eh)
	checkUnhandledEvents(c, data.Events)
	c.Check(data.APIClient, gc.IsNil)
	c.Check(data.Runner, gc.IsNil)
	s.stub.CheckCalls(c, nil)
}

func (s *eventHandlerSuite) TestCloseIdempotent(c *gc.C) {
	eh := workers.NewEventHandlers()
	c.Assert(eh, gc.NotNil)

	err := eh.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = eh.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.checkRegistered(c, eh)
	data := workers.ExposeEventHandlers(eh)
	checkUnhandledEvents(c, data.Events)
	c.Check(data.APIClient, gc.IsNil)
	c.Check(data.Runner, gc.IsNil)
}

func (s *eventHandlerSuite) TestRegisterHandler(c *gc.C) {
	eh := workers.NewEventHandlers()
	defer eh.Close()
	eh.RegisterHandler(s.handler)

	s.checkRegistered(c, eh, "handler")
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
	data := workers.ExposeEventHandlers(eh)
	runner := data.Runner

	eh.AddEvents(events...)

	engine.Kill()
	err = engine.Wait()
	c.Assert(err, jc.ErrorIsNil)
	eh.Close()

	checkUnhandledEvents(c, data.Events)
	s.stub.CheckCallNames(c, "List", "handler")
	c.Check(s.stub.Calls()[1].Args[0], gc.DeepEquals, events)
	c.Check(s.stub.Calls()[1].Args[1], gc.DeepEquals, s.apiClient)
	c.Check(runner, gc.NotNil)
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
