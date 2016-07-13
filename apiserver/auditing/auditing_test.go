// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// auditing provides the API server facade for managing audit
// functionality.
package auditing_test

import (
	"fmt"
	"net/http"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/auditing"
	"github.com/juju/juju/apiserver/auditing/fakeconnection"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/audit"
)

type auditingSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&auditingSuite{})

func (*auditingSuite) TestAuditStreamHandler_AuthFailureReturnsErr(c *gc.C) {
	done := make(chan struct{})
	defer close(done)

	testLog := loggo.TestWriter{}
	if err := loggo.RegisterWriter("test writer", &testLog, loggo.TRACE); err != nil {
		c.Fatalf("cannot register test logger: %v", err)
	}

	handler, err := auditing.NewConnHandler(auditing.ConnHandlerContext{
		ServerDone:       done,
		Logger:           loggo.GetLogger("juju.apiserver"),
		AuthAgent:        func(*http.Request) error { return fmt.Errorf("mock auth error") },
		OpenAuditEntries: func(<-chan struct{}) <-chan auditing.AuditEntryRecord { return nil },
	})
	if err != nil {
		c.Fatalf("cannot instantiate connection handler: %v", err)
	}
	conn := fakeconnection.Instance{}

	handler(&conn)

	c.Assert(testLog.Log(), gc.HasLen, 1)
	c.Check(testLog.Log()[0].Message, gc.Equals, "mock auth error")
	conn.CheckCall(c, 1, "Send", params.ErrorResult{Error: &params.Error{Message: "mock auth error"}})
}

func (*auditingSuite) TestAuditStreamHandler_SendsAuditEntries(c *gc.C) {
	done := make(chan struct{})
	defer close(done)

	openAuditEntries := func(done <-chan struct{}) <-chan auditing.AuditEntryRecord {
		records := make(chan auditing.AuditEntryRecord)
		go func() {
			defer close(records)
			records <- auditing.AuditEntryRecord{
				Value: audit.AuditEntry{},
			}
		}()
		return records
	}

	handler, err := auditing.NewConnHandler(auditing.ConnHandlerContext{
		ServerDone:       done,
		Logger:           loggo.GetLogger("juju.apiserver"),
		AuthAgent:        func(*http.Request) error { return nil },
		OpenAuditEntries: openAuditEntries,
	})
	if err != nil {
		c.Fatalf("cannot instantiate connection handler: %v", err)
	}
	conn := fakeconnection.Instance{}

	handler(&conn)

	// Client expects empty error to be sent first
	conn.CheckCall(c, 1, "Send", params.ErrorResult{})

	conn.CheckCall(c, 2, "Send", params.AuditEntryParams{
		JujuServerVersion: version.Number{Major: 0, Minor: 0, Tag: "", Patch: 0, Build: 0},
		ModelUUID:         "",
		Timestamp:         "0001-01-01T00:00:00Z",
		RemoteAddress:     "",
		OriginType:        "",
		OriginName:        "",
		Operation:         "",
		Data:              map[string]interface{}(nil),
	})
}
