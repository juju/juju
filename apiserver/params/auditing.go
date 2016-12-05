// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import "github.com/juju/version"

// AuditEntryParams is the doc that is persisted to the audit collection.
type AuditEntryParams struct {

	// JujuServerVersion is the version of jujud that recorded this
	// entry.
	JujuServerVersion version.Number `json:"jujuServerVersion"`

	// ModelID is the ID of the model the audit entry was written on.
	ModelUUID string `json:"modelUUID"`

	// Timestamp is when the audit entry was written. It is marshaled
	// to a bytestream via time.Time::MarshalText and can be
	// unmarshaled via time.Time::UnmarshalText.
	Timestamp string `json:"timestamp"`

	// RemoteAddress is the IP of the machine from which the
	// audit-event was triggered.
	RemoteAddress string `json:"remoteAddress"`

	// OriginType is the type of entity (e.g. model, user, action)
	// which triggered the audit event.
	OriginType string `json:"originType"`

	// OriginName is the name of the origin which triggered the
	// audit-event.
	OriginName string `json:"originName"`

	// Operation is the operation that was performed that triggered
	// the audit event.
	Operation string `json:"operation"`

	// Data is a catch-all for storing random data.
	Data map[string]interface{} `json:"data"`
}
