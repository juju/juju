// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit

import (
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/mongo"
)

// CollectionName is the name of the collection for the audit log in
// mongoDB.
const CollectionName = "audit.log"

// AuditEntryDoc is the doc that is persisted to the audit collection.
type AuditEntryDoc struct {
	// ModelID is the ID of the model the audit entry was written on.
	ModelID names.ModelTag `bson:"modelid"`
	// Timestamp is when the audit entry was written.
	Timestamp time.Time `bson:"timestamp"`
	// OriginIP is the IP of the machine from which the audit-event
	// was triggered.
	OriginIP net.IP `bson:"originip"`
	// OriginType is the type of entity (e.g. model, user, action)
	// which triggered the audit event.
	OriginType string `bson:"origintype"`
	// OriginName is the name of the origin which triggered the
	// audit-event.
	OriginName string `bson:"originname"`
	// Operation is the operation that was performed that triggered
	// the audit event.
	Operation string `bson:"operation"`
}

// PutAuditEntryFn creates a closure which when passed an
// AuditEntryDoc will write it to the audit collection.
func PutAuditEntryFn(
	getCollection func(string) (mongo.Collection, func()),
) func(AuditEntryDoc) error {
	return func(doc AuditEntryDoc) error {
		auditCollection, close := getCollection(CollectionName)
		defer close()

		writeableAuditCollection := auditCollection.Writeable()

		err := writeableAuditCollection.Insert(doc)
		return errors.Trace(err)
	}
}
