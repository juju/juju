// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// auditing provides the API server facade for managing audit
// functionality.
package auditing_test

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/internal/auditing"
	"github.com/juju/juju/apiserver/internal/auditing/fakeconnection"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/audit"
	"github.com/juju/utils"
)

type auditingSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&auditingSuite{})

func (*auditingSuite) TestNewConnHandler_AuthFailureReturnsErr(c *gc.C) {
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

func (*auditingSuite) TestNewConnHandler_HandlesTeardown(c *gc.C) {
	done := make(chan struct{})
	var wg sync.WaitGroup

	openAuditEntries := func(done <-chan struct{}) <-chan auditing.AuditEntryRecord {
		records := make(chan auditing.AuditEntryRecord)
		wg.Add(1)
		go func() {
			defer close(records)
			defer wg.Done()
			// Wait until we're told to exit.
			<-done
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
		c.Fatalf("cannot set up connection handler: %v", err)
	}

	fakeConn := fakeconnection.Instance{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler(&fakeConn)
	}()

	close(done)

	// Wait until both the openAuditEntries & handler are done.
	wg.Wait()

	fakeConn.CheckCall(c, 2, "Close")
}

func (*auditingSuite) TestNewConnHandler_SendsAuditEntries(c *gc.C) {
	done := make(chan struct{})
	defer close(done)

	timestamp, err := time.Parse("2006-01-02", "2016-06-14")
	if err != nil {
		c.Fatalf("could not parse time: %v", err)
	}
	entryToSend := audit.AuditEntry{
		JujuServerVersion: version.MustParse("1.0.0"),
		ModelUUID:         utils.MustNewUUID().String(),
		Timestamp:         timestamp,
		RemoteAddress:     "8.8.8.8",
		OriginType:        "user",
		OriginName:        "katco",
		Operation:         "status",
		Data: map[string]interface{}{
			"foo": "bar",
			"baz": 1,
		},
	}

	openAuditEntries := func(done <-chan struct{}) <-chan auditing.AuditEntryRecord {
		records := make(chan auditing.AuditEntryRecord)
		go func() {
			defer close(records)
			select {
			case <-done:
				return
			case records <- auditing.AuditEntryRecord{Value: entryToSend}:
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
		JujuServerVersion: entryToSend.JujuServerVersion,
		ModelUUID:         entryToSend.ModelUUID,
		Timestamp:         entryToSend.Timestamp.Format(time.RFC3339),
		RemoteAddress:     entryToSend.RemoteAddress,
		OriginType:        entryToSend.OriginType,
		OriginName:        entryToSend.OriginName,
		Operation:         entryToSend.Operation,
		Data:              entryToSend.Data,
	})
}

func (*auditingSuite) TestNewConnHandler_SendsErrorAndExits(c *gc.C) {
	done := make(chan struct{})
	defer close(done)

	var wg sync.WaitGroup
	openAuditEntries := func(done <-chan struct{}) <-chan auditing.AuditEntryRecord {
		records := make(chan auditing.AuditEntryRecord)
		wg.Add(1)
		go func() {
			defer close(records)
			defer wg.Done()
			// Simulate streaming records until we're told to stop.
			for {
				select {
				case <-done:
					return
				case records <- auditing.AuditEntryRecord{Error: fmt.Errorf("doh")}:
				}
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
		c.Fatalf("cannot open handler: %v", err)
	}

	conn := fakeconnection.Instance{}
	handler(&conn)
	wg.Wait() // Wait until openAuditEntries goroutine is told to stop

	conn.CheckCall(c, 2, "Send", params.ErrorResult{Error: &params.Error{Message: "doh"}})
}
