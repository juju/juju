// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer_test

import (
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
)

type auditFilterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&auditFilterSuite{})

func (s *auditFilterSuite) TestFiltersUninterestingConversations(c *gc.C) {
	target := &apitesting.FakeAuditLog{}
	filter := func(r auditlog.Request) bool {
		return !strings.HasPrefix(r.Method, "List")
	}
	log := observer.NewAuditLogFilter(target, filter)

	err := log.AddConversation(auditlog.Conversation{})
	c.Assert(err, jc.ErrorIsNil)
	// Nothing written out yet.
	target.CheckCallNames(c)

	err = log.AddRequest(auditlog.Request{Method: "ListBuckets"})
	c.Assert(err, jc.ErrorIsNil)
	target.CheckCallNames(c)

	err = log.AddResponse(auditlog.ResponseErrors{})
	c.Assert(err, jc.ErrorIsNil)
	target.CheckCallNames(c)

	err = log.AddRequest(auditlog.Request{Method: "ListSpades"})
	c.Assert(err, jc.ErrorIsNil)
	target.CheckCallNames(c)

	err = log.AddRequest(auditlog.Request{Method: "BuildCastle"})
	c.Assert(err, jc.ErrorIsNil)
	// Everything gets written now.
	target.CheckCallNames(c,
		"AddConversation", "AddRequest", "AddResponse", "AddRequest",
		"AddRequest")
	calls := target.Calls()
	getMethod := func(i int) string {
		return calls[i].Args[0].(auditlog.Request).Method
	}
	requests := []string{getMethod(1), getMethod(3), getMethod(4)}
	c.Assert(requests, gc.DeepEquals, []string{"ListBuckets", "ListSpades", "BuildCastle"})

	// Subsequent messages are passed through directly even if they're
	// not inherently interesting.
	target.ResetCalls()

	err = log.AddRequest(auditlog.Request{Method: "ListTrowels"})
	c.Assert(err, jc.ErrorIsNil)
	target.CheckCallNames(c, "AddRequest")

	calls = target.Calls()
	c.Assert(getMethod(0), gc.Equals, "ListTrowels")

	err = log.AddResponse(auditlog.ResponseErrors{})
	c.Assert(err, jc.ErrorIsNil)
	target.CheckCallNames(c, "AddRequest", "AddResponse")
}
