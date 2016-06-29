// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"net/http"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/rpc"
)

type observerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&observerSuite{})

func (*observerSuite) TestObserverFactoryMultiplexer_CallsAllFactories(c *gc.C) {
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

func (*observerSuite) TestJoin_CallsAllObservers(c *gc.C) {
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

func (*observerSuite) TestLeave_CallsAllObservers(c *gc.C) {
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

func (*observerSuite) TestServerReply_CallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		&fakeobserver.Instance{},
		&fakeobserver.Instance{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	var (
		req  rpc.Request
		hdr  rpc.Header
		body string
	)
	o.ServerReply(req, &hdr, body)

	for _, f := range observers {
		f.CheckCall(c, 0, "ServerReply", req, &hdr, body)
	}
}

func (*observerSuite) TestServerRequest_CallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.Instance{
		&fakeobserver.Instance{},
		&fakeobserver.Instance{},
	}

	o := observer.NewMultiplexer(observers[0], observers[1])
	var (
		hdr  rpc.Header
		body string
	)
	o.ServerRequest(&hdr, body)

	for _, f := range observers {
		f.CheckCall(c, 0, "ServerRequest", &hdr, body)
	}
}

func (*observerSuite) TestLogin_CallsAllObservers(c *gc.C) {
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
