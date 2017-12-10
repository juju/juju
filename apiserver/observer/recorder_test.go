// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/rpc"
)

type recorderSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&recorderSuite{})

func (s *recorderSuite) TestServerRequest(c *gc.C) {
	fake := &fakeobserver.Instance{}
	log := &apitesting.FakeAuditLog{}
	auditRecorder, err := auditlog.NewRecorder(log, auditlog.ConversationArgs{
		ConnectionID: 4567,
	})
	c.Assert(err, jc.ErrorIsNil)
	factory := observer.NewRecorderFactory(fake, auditRecorder)
	recorder := factory()
	hdr := &rpc.Header{
		RequestId: 123,
		Request:   rpc.Request{"Type", 5, "", "Action"},
	}
	err = recorder.HandleRequest(hdr, "the args")
	c.Assert(err, jc.ErrorIsNil)

	fake.CheckCallNames(c, "RPCObserver")
	fakeOb := fake.Calls()[0].Args[0].(*fakeobserver.RPCInstance)
	fakeOb.CheckCallNames(c, "ServerRequest")
	fakeOb.CheckCall(c, 0, "ServerRequest", hdr, "the args")

	log.CheckCallNames(c, "AddConversation", "AddRequest")

	request := log.Calls()[1].Args[0].(auditlog.Request)
	c.Assert(request.ConversationID, gc.HasLen, 16)
	request.ConversationID = "abcdef0123456789"
	c.Assert(request, gc.Equals, auditlog.Request{
		ConversationID: "abcdef0123456789",
		ConnectionID:   "11D7",
		RequestID:      123,
		Facade:         "Type",
		Method:         "Action",
		Version:        5,
		Args:           `"the args"`,
	})
}

func (s *recorderSuite) TestServerReply(c *gc.C) {
	fake := &fakeobserver.Instance{}
	log := &apitesting.FakeAuditLog{}
	auditRecorder, err := auditlog.NewRecorder(log, auditlog.ConversationArgs{
		ConnectionID: 4567,
	})
	c.Assert(err, jc.ErrorIsNil)
	factory := observer.NewRecorderFactory(fake, auditRecorder)
	recorder := factory()

	req := rpc.Request{"Type", 5, "", "Action"}
	hdr := &rpc.Header{RequestId: 123}
	err = recorder.HandleReply(req, hdr, "the response")
	c.Assert(err, jc.ErrorIsNil)

	fake.CheckCallNames(c, "RPCObserver")
	fakeOb := fake.Calls()[0].Args[0].(*fakeobserver.RPCInstance)
	fakeOb.CheckCallNames(c, "ServerReply")
	fakeOb.CheckCall(c, 0, "ServerReply", req, hdr, "the response")

	log.CheckCallNames(c, "AddConversation", "AddResponse")

	respErrors := log.Calls()[1].Args[0].(auditlog.ResponseErrors)
	c.Assert(respErrors.ConversationID, gc.HasLen, 16)
	respErrors.ConversationID = "abcdef0123456789"
	c.Assert(respErrors, gc.DeepEquals, auditlog.ResponseErrors{
		ConversationID: "abcdef0123456789",
		ConnectionID:   "11D7",
		RequestID:      123,
		Errors:         nil,
	})
}

func (s *recorderSuite) TestNoAuditRequest(c *gc.C) {
	fake := &fakeobserver.Instance{}
	factory := observer.NewRecorderFactory(fake, nil)
	recorder := factory()
	hdr := &rpc.Header{
		RequestId: 123,
		Request:   rpc.Request{"Type", 0, "", "Action"},
	}
	err := recorder.HandleRequest(hdr, "the body")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *recorderSuite) TestNoAuditReply(c *gc.C) {
	fake := &fakeobserver.Instance{}
	factory := observer.NewRecorderFactory(fake, nil)
	recorder := factory()
	req := rpc.Request{"Type", 0, "", "Action"}
	hdr := &rpc.Header{RequestId: 123}
	err := recorder.HandleReply(req, hdr, "the body")
	c.Assert(err, jc.ErrorIsNil)
}
