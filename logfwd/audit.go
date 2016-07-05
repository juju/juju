// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"reflect"

	"github.com/juju/errors"
)

// AuditRecord holds all the information for a single log record.
type AuditRecord struct {
	BaseRecord

	// Audit holds controller activity audit info.
	Audit Audit
}

// Kind implements Record by returning RecordKindAudit.
func (rec AuditRecord) Kind() RecordKind {
	return RecordKindAudit
}

// Validate ensures that the record is correct.
func (rec AuditRecord) Validate() error {
	if err := rec.BaseRecord.Validate(); err != nil {
		return errors.Trace(err)
	}

	if err := rec.Audit.Validate(); err != nil {
		return errors.Annotate(err, "invalid Audit")
	}

	return nil
}

// Audit holds audit information about an externally-initiated
// operation in the controller/model.
type Audit struct {
	// Operation identifies the actual audited operation.
	Operation string

	// Args are the arguments used for the operation.
	Args map[string]string
}

// IsZero indicates whether or not it is the zero value.
func (audit Audit) IsZero() bool {
	return reflect.DeepEqual(audit, Audit{})
}

// Validate ensures that the audit information is correct.
func (audit Audit) Validate() error {
	if audit.Operation == "" {
		return errors.NewNotValid(nil, "empty Operation")
	}

	if _, ok := audit.Args[""]; ok {
		return errors.NewNotValid(nil, "empty arg name not allowed")
	}

	return nil
}
