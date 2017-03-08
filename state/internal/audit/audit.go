// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit

import (
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/audit"
	"github.com/juju/juju/mongo/utils"
)

// auditEntryDoc is the doc that is persisted to the audit collection.
type auditEntryDoc struct {

	// JujuServerVersion is the version of jujud that recorded this
	// entry.
	JujuServerVersion version.Number `bson:"juju-server-version"`

	// ModelID is the ID of the model the audit entry was written on.
	ModelUUID string `bson:"model-uuid"`

	// Timestamp is when the audit entry was written. It is marshaled
	// to a bytestream via time.Time::MarshalText and can be
	// unmarshaled via time.Time::UnmarshalText.
	Timestamp string `bson:"timestamp"`

	// RemoteAddress is the IP of the machine from which the
	// audit-event was triggered.
	RemoteAddress string `bson:"remote-address"`

	// OriginType is the type of entity (e.g. model, user, action)
	// which triggered the audit event.
	OriginType string `bson:"origin-type"`

	// OriginName is the name of the origin which triggered the
	// audit-event.
	OriginName string `bson:"origin-name"`

	// Operation is the operation that was performed that triggered
	// the audit event.
	Operation string `bson:"operation"`

	// Data is a catch-all for storing random data.
	Data map[string]interface{} `bson:"data"`
}

// PutAuditEntryFn creates a closure which when passed an AuditEntry
// will write it to the audit collection.
func PutAuditEntryFn(
	collectionName string,
	insertDoc func(string, ...interface{}) error,
) func(audit.AuditEntry) error {
	return func(auditEntry audit.AuditEntry) error {
		if err := auditEntry.Validate(); err != nil {
			return errors.Trace(err)
		}
		auditEntryDoc, err := auditEntryDocFromAuditEntry(auditEntry)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(insertDoc(collectionName, auditEntryDoc))
	}
}

func auditEntryDocFromAuditEntry(auditEntry audit.AuditEntry) (auditEntryDoc, error) {

	timeAsBlob, err := auditEntry.Timestamp.MarshalText()
	if err != nil {
		return auditEntryDoc{}, errors.Trace(err)
	}

	return auditEntryDoc{
		JujuServerVersion: auditEntry.JujuServerVersion,
		ModelUUID:         auditEntry.ModelUUID,
		Timestamp:         string(timeAsBlob),
		RemoteAddress:     auditEntry.RemoteAddress,
		OriginType:        auditEntry.OriginType,
		OriginName:        auditEntry.OriginName,
		Operation:         auditEntry.Operation,
		Data:              utils.EscapeKeys(auditEntry.Data),
	}, nil
}
