// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
)

// These constants define the field names and type values used by documents in
// a lease collection. They *must* remain in sync with the bson marshalling
// annotations in leaseDoc.
const (
	// field* identify the fields in a leaseDoc.
	fieldNamespace = "namespace"
	fieldHolder    = "holder"
	fieldStart     = "start"
	fieldDuration  = "duration"
	fieldWriter    = "writer"
)

// toInt64 converts a local time.Time into a database value that doesn't
// silently lose precision.
func toInt64(t time.Time) int64 {
	return t.UnixNano()
}

// toTime converts a toInt64 result, as loaded from the db, back to a time.Time.
func toTime(v int64) time.Time {
	return time.Unix(0, v)
}

// leaseDocId returns the _id for the document holding details of the supplied
// namespace and lease.
func leaseDocId(namespace, lease string) string {
	return fmt.Sprintf("%s#%s#", namespace, lease)
}

// leaseDoc is used to serialise lease entries.
type leaseDoc struct {
	// Id is always "<Namespace>#<Name>#", so that we can extract useful
	// information from a stream of watcher events without incurring extra
	// DB hits. Apart from checking validity on load, though, there's
	// little reason to use Id elsewhere; Namespace and Name are the
	// sources of truth.
	Id        string `bson:"_id"`
	Namespace string `bson:"namespace"`
	Name      string `bson:"name"`

	// Holder, Expiry, and Writer map directly to entry.
	Holder   string        `bson:"holder"`
	Start    int64         `bson:"start"`
	Duration time.Duration `bson:"duration"`
	Writer   string        `bson:"writer"`
}

// validate returns an error if any fields are invalid or inconsistent.
func (doc leaseDoc) validate() error {
	// state.multiModelRunner prepends environ ids in our documents, and
	// state.modelStateCollection does not strip them out.
	if !strings.HasSuffix(doc.Id, leaseDocId(doc.Namespace, doc.Name)) {
		return errors.Errorf("inconsistent _id")
	}
	if err := lease.ValidateString(doc.Holder); err != nil {
		return errors.Annotatef(err, "invalid holder")
	}
	if doc.Start < 0 {
		return errors.Errorf("invalid start time")
	}
	if doc.Duration <= 0 {
		return errors.Errorf("invalid duration")
	}
	if err := lease.ValidateString(doc.Writer); err != nil {
		return errors.Annotatef(err, "invalid writer")
	}
	return nil
}

// entry returns the lease name and an entry corresponding to the document. If
// the document cannot be validated, it returns an error.
func (doc leaseDoc) entry() (string, entry, error) {
	if err := doc.validate(); err != nil {
		return "", entry{}, errors.Trace(err)
	}
	entry := entry{
		holder:   doc.Holder,
		start:    toTime(doc.Start),
		duration: doc.Duration,
		writer:   doc.Writer,
	}
	return doc.Name, entry, nil
}

// newLeaseDoc returns a valid lease document encoding the supplied lease and
// entry in the supplied namespace, or an error.
func newLeaseDoc(namespace, name string, entry entry) (*leaseDoc, error) {
	doc := &leaseDoc{
		Id:        leaseDocId(namespace, name),
		Namespace: namespace,
		Name:      name,
		Holder:    entry.holder,
		Start:     toInt64(entry.start),
		Duration:  entry.duration,
		Writer:    entry.writer,
	}
	if err := doc.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return doc, nil
}
