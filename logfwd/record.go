// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"time"

	"github.com/juju/errors"
)

// These are the recognized kinds of log forwarding record.
const (
	recordKindNotSet RecordKind = iota

	RecordKindLog
	RecordKindAudit
)

// RecordKind identifies a kind of log forwarding record.
type RecordKind int

// Record exposes the common information between different kinds of
// log forwarding record. The kind may be used to determine to which
// type the record may be converted:
//
//  RecordKindLog   -> LogRecord
//  RecordKindAudit -> AuditRecord
type Record interface {
	// Base returns the record's base record.
	Base() BaseRecord

	// Kind returns the value that identifies the records kind.
	Kind() RecordKind
}

// BaseRecord holds all the information for a single forwarding record
// which the different record kinds have in common.
type BaseRecord struct {
	// ID identifies the record and its position in a sequence
	// of records.
	ID int64

	// Origin describes what created the record.
	Origin Origin

	// Timestamp is when the record was created.
	Timestamp time.Time

	// Message is the record's body. It may be empty.
	Message string
}

// Base implements Record. It returns the base record, which is useful
// when BaseRecord is embedded.
func (rec BaseRecord) Base() BaseRecord {
	return rec
}

// Validate ensures that the record is correct.
func (rec BaseRecord) Validate() error {
	if err := rec.Origin.Validate(); err != nil {
		return errors.Annotate(err, "invalid Origin")
	}

	if rec.Timestamp.IsZero() {
		return errors.NewNotValid(nil, "empty Timestamp")
	}

	// rec.Message may be anything, so we don't check it.

	return nil
}
