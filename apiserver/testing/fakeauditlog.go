// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"

	"github.com/juju/juju/core/auditlog"
)

// FakeAuditLog implements auditlog.AuditLog.
type FakeAuditLog struct {
	testing.Stub
}

func (l *FakeAuditLog) AddConversation(m auditlog.Conversation) error {
	l.Stub.AddCall("AddConversation", m)
	return l.Stub.NextErr()
}

func (l *FakeAuditLog) AddRequest(m auditlog.Request) error {
	l.Stub.AddCall("AddRequest", m)
	return l.Stub.NextErr()
}

func (l *FakeAuditLog) AddResponse(m auditlog.ResponseErrors) error {
	l.Stub.AddCall("AddResponse", m)
	return l.Stub.NextErr()
}

func (l *FakeAuditLog) Close() error {
	l.Stub.AddCall("Close")
	return l.Stub.NextErr()
}
