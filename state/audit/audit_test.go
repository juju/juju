// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit_test

import (
	"net"
	"time"

	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/audit"
)

type AuditSuite struct{}

var _ = gc.Suite(&AuditSuite{})

func (s *AuditSuite) TestPutAuditEntry_PersistRequestedEntry(c *gc.C) {
	var auditEntriesPersisted []interface{}
	auditEntryRequested := audit.AuditEntryDoc{
		ModelID:    names.NewModelTag("my-model"),
		Timestamp:  time.Now(),
		OriginIP:   net.ParseIP("8.8.8.8"),
		OriginType: "user",
		OriginName: "bob",
		Operation:  "status",
	}
	var closeCalled bool

	getCollection := func(name string) (mongo.Collection, func()) {
		insert := func(docs ...interface{}) error {
			auditEntriesPersisted = docs
			return nil
		}
		writeable := func() mongo.WriteCollection {
			return fakeWriteCollection{insert: insert}
		}
		close := func() { closeCalled = true }
		return fakeCollection{FakeName: name, writeable: writeable}, close
	}

	putAuditEntry := audit.PutAuditEntryFn(getCollection)
	putAuditEntry(auditEntryRequested)

	c.Check(closeCalled, gc.Equals, true)
	c.Assert(auditEntriesPersisted, gc.HasLen, 1)
	c.Check(auditEntriesPersisted[0], gc.DeepEquals, auditEntryRequested)
}
