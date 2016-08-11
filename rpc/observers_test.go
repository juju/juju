// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/rpc"
)

type multiplexerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&multiplexerSuite{})

func (*multiplexerSuite) TestServerReply_CallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.RPCInstance{
		(&fakeobserver.Instance{}).RPCObserver().(*fakeobserver.RPCInstance),
		(&fakeobserver.Instance{}).RPCObserver().(*fakeobserver.RPCInstance),
	}

	o := rpc.NewObserverMultiplexer(observers[0], observers[1])
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

func (*multiplexerSuite) TestServerRequest_CallsAllObservers(c *gc.C) {
	observers := []*fakeobserver.RPCInstance{
		(&fakeobserver.Instance{}).RPCObserver().(*fakeobserver.RPCInstance),
		(&fakeobserver.Instance{}).RPCObserver().(*fakeobserver.RPCInstance),
	}

	o := rpc.NewObserverMultiplexer(observers[0], observers[1])
	var (
		hdr  rpc.Header
		body string
	)
	o.ServerRequest(&hdr, body)

	for _, f := range observers {
		f.CheckCall(c, 0, "ServerRequest", &hdr, body)
	}
}
