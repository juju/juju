// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"context"
	"net/http"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
)

type multiplexerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&multiplexerSuite{})

func (*multiplexerSuite) TestObserverFactoryMultiplexerCallsAllFactories(c *gc.C) {
	callCount := 0
	factories := []observer.ObserverFactory{
		func() observer.Observer { callCount++; return nil },
		func() observer.Observer { callCount++; return nil },
	}

	newMultiplexObserver := observer.ObserverFactoryMultiplexer(factories...)
	c.Assert(callCount, gc.Equals, 0)

	multiplexedObserver := newMultiplexObserver()
	c.Check(multiplexedObserver, gc.NotNil)
	c.Check(callCount, gc.Equals, 2)
}

func (*multiplexerSuite) TestJoinCallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		{},
		{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	var req http.Request
	o.Join(req.Context(), &req, 1234)

	for _, f := range observers {
		f.CheckCall(c, 0, "Join", &req, uint64(1234))
	}
}

func (*multiplexerSuite) TestLeaveCallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		{},
		{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	o.Leave(context.Background())

	for _, f := range observers {
		f.CheckCall(c, 0, "Leave")
	}
}

func (*multiplexerSuite) TestRPCObserverCallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		{},
		{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1], &fakeobserver.NoRPCInstance{})
	o.RPCObserver()

	for _, f := range observers {
		f.CheckCallNames(c, "RPCObserver")
	}
}

func (*multiplexerSuite) TestLoginCallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		{},
		{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	entity := names.NewMachineTag("42")
	model := names.NewModelTag("fake-uuid")
	fromController := false
	userData := "foo"
	o.Login(context.Background(), entity, model, fromController, userData)

	for _, f := range observers {
		f.CheckCall(c, 0, "Login", entity, model, fromController, userData)
	}
}
