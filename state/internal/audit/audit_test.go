// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/audit"
	mongoutils "github.com/juju/juju/mongo/utils"
	stateaudit "github.com/juju/juju/state/internal/audit"
	"github.com/juju/utils"
)

type AuditSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AuditSuite{})

func (*AuditSuite) TestPutAuditEntry_PersistAuditEntry(c *gc.C) {

	requested := audit.AuditEntry{
		JujuServerVersion: version.MustParse("1.0.0"),
		ModelUUID:         utils.MustNewUUID().String(),
		Timestamp:         time.Now().UTC(),
		RemoteAddress:     "8.8.8.8",
		OriginType:        "user",
		OriginName:        "bob",
		Operation:         "status",
		Data: map[string]interface{}{
			"a": "b",
			"$a.b": map[string]interface{}{
				"b.$a": "c",
			},
		},
	}

	var insertDocsCalled bool
	insertDocs := func(collectionName string, docs ...interface{}) error {
		insertDocsCalled = true
		c.Check(collectionName, gc.Equals, "audit.log")
		c.Assert(docs, gc.HasLen, 1)

		serializedAuditDoc, err := bson.Marshal(docs[0])
		c.Assert(err, jc.ErrorIsNil)

		requestedTimeBlob, err := requested.Timestamp.MarshalText()
		c.Assert(err, jc.ErrorIsNil)

		c.Check(string(serializedAuditDoc), jc.BSONEquals, map[string]interface{}{
			"juju-server-version": requested.JujuServerVersion,
			"model-uuid":          requested.ModelUUID,
			"timestamp":           string(requestedTimeBlob),
			"remote-address":      "8.8.8.8",
			"origin-type":         requested.OriginType,
			"origin-name":         requested.OriginName,
			"operation":           requested.Operation,
			"data":                mongoutils.EscapeKeys(requested.Data),
		})

		return nil
	}

	putAuditEntry := stateaudit.PutAuditEntryFn("audit.log", insertDocs)
	err := putAuditEntry(requested)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(insertDocsCalled, jc.IsTrue)
}

func (*AuditSuite) TestPutAuditEntry_PropagatesWriteError(c *gc.C) {
	const errMsg = "my error"
	insertDocs := func(string, ...interface{}) error {
		return errors.New(errMsg)
	}
	putAuditEntry := stateaudit.PutAuditEntryFn("audit.log", insertDocs)

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	auditEntry := audit.AuditEntry{
		JujuServerVersion: version.MustParse("1.0.0"),
		ModelUUID:         uuid.String(),
		Timestamp:         time.Now().UTC(),
		RemoteAddress:     "8.8.8.8",
		OriginType:        "user",
		OriginName:        "bob",
		Operation:         "status",
	}
	c.Assert(auditEntry.Validate(), jc.ErrorIsNil)

	err = putAuditEntry(auditEntry)
	c.Check(err, gc.ErrorMatches, errMsg)
}

func (*AuditSuite) TestPutAuditEntry_ValidateAuditEntry(c *gc.C) {
	var auditEntry audit.AuditEntry

	// Don't care what the error is; just that it's not valid.
	validationErr := auditEntry.Validate()
	c.Assert(validationErr, gc.NotNil)

	putAuditEntry := stateaudit.PutAuditEntryFn("", nil)
	err := putAuditEntry(auditEntry)
	c.Check(err, gc.ErrorMatches, validationErr.Error())
}
