// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"net/http"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
)

type multiplexerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&multiplexerSuite{})

func (*multiplexerSuite) TestObserverFactoryMultiplexer_CallsAllFactories(c *gc.C) {
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

func (*multiplexerSuite) TestJoin_CallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		&fakeobserver.Instance{},
		&fakeobserver.Instance{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	var req http.Request
	o.Join(&req)

	for _, f := range observers {
		f.CheckCall(c, 0, "Join", &req)
	}
}

func (*multiplexerSuite) TestLeave_CallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		&fakeobserver.Instance{},
		&fakeobserver.Instance{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	o.Leave()

	for _, f := range observers {
		f.CheckCall(c, 0, "Leave")
	}
}

func (*multiplexerSuite) TestRPCObserver_CallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		&fakeobserver.Instance{},
		&fakeobserver.Instance{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	o.RPCObserver()

	for _, f := range observers {
		f.CheckCall(c, 0, "RPCObserver")
	}
}

func (*multiplexerSuite) TestLogin_CallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		&fakeobserver.Instance{},
		&fakeobserver.Instance{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	tag := "foo"
	o.Login(tag)

	for _, f := range observers {
		f.CheckCall(c, 0, "Login", tag)
	}
}
