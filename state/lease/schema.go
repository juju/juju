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
// annotations in leaseDoc and clockDoc.
const (
	// fieldType and fieldNamespace identify the Type and Namespace fields in
	// both leaseDoc and clockDoc.
	fieldType      = "type"
	fieldNamespace = "namespace"

	// typeLease and typeClock are the acceptable values for fieldType.
	typeLease = "lease"
	typeClock = "clock"

	// fieldLease* identify the fields in a leaseDoc.
	fieldLeaseName   = "name"
	fieldLeaseHolder = "holder"
	fieldLeaseExpiry = "expiry"
	fieldLeaseWriter = "writer"

	// fieldClock* identify the fields in a clockDoc.
	fieldClockWriters = "writers"
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
	return fmt.Sprintf("%s#%s#%s#", typeLease, namespace, lease)
}

// leaseDoc is used to serialise lease entries.
type leaseDoc struct {
	// Id is always "<Type>#<Namespace>#<Name>#", and <Type> is always "lease",
	// so that we can extract useful information from a stream of watcher events
	// without incurring extra DB hits.
	// Apart from checking validity on load, though, there's little reason
	// to use Id elsewhere; Namespace and Name are the sources of truth.
	Id        string `bson:"_id"`
	Type      string `bson:"type"`
	Namespace string `bson:"namespace"`
	Name      string `bson:"name"`

	// Holder, Expiry, and Writer map directly to entry.
	Holder string `bson:"holder"`
	Expiry int64  `bson:"expiry"`
	Writer string `bson:"writer"`
}

// validate returns an error if any fields are invalid or inconsistent.
func (doc leaseDoc) validate() error {
	if doc.Type != typeLease {
		return errors.Errorf("invalid type %q", doc.Type)
	}
	// state.multiModelRunner prepends environ ids in our documents, and
	// state.modelStateCollection does not strip them out.
	if !strings.HasSuffix(doc.Id, leaseDocId(doc.Namespace, doc.Name)) {
		return errors.Errorf("inconsistent _id")
	}
	if err := lease.ValidateString(doc.Holder); err != nil {
		return errors.Annotatef(err, "invalid holder")
	}
	if doc.Expiry == 0 {
		return errors.Errorf("invalid expiry")
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
		holder: doc.Holder,
		expiry: toTime(doc.Expiry),
		writer: doc.Writer,
	}
	return doc.Name, entry, nil
}

// newLeaseDoc returns a valid lease document encoding the supplied lease and
// entry in the supplied namespace, or an error.
func newLeaseDoc(namespace, name string, entry entry) (*leaseDoc, error) {
	doc := &leaseDoc{
		Id:        leaseDocId(namespace, name),
		Type:      typeLease,
		Namespace: namespace,
		Name:      name,
		Holder:    entry.holder,
		Expiry:    toInt64(entry.expiry),
		Writer:    entry.writer,
	}
	if err := doc.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return doc, nil
}

// clockDocId returns the _id for the document holding clock skew information
// for clients that have written in the supplied namespace.
func clockDocId(namespace string) string {
	return fmt.Sprintf("%s#%s#", typeClock, namespace)
}

// clockDoc is used to synchronise clients.
type clockDoc struct {
	// Id is always "<Type>#<Namespace>#", and <Type> is always "clock", for
	// consistency with leaseDoc and ease of querying within the collection.
	Id        string `bson:"_id"`
	Type      string `bson:"type"`
	Namespace string `bson:"namespace"`

	// Writers holds the latest acknowledged time for every known client.
	Writers map[string]int64 `bson:"writers"`
}

// validate returns an error if any fields are invalid or inconsistent.
func (doc clockDoc) validate() error {
	if doc.Type != typeClock {
		return errors.Errorf("invalid type %q", doc.Type)
	}
	// state.multiModelRunner prepends environ ids in our documents, and
	// state.modelStateCollection does not strip them out.
	if !strings.HasSuffix(doc.Id, clockDocId(doc.Namespace)) {
		return errors.Errorf("inconsistent _id")
	}
	for writer, written := range doc.Writers {
		if written == 0 {
			return errors.Errorf("invalid time for writer %q", writer)
		}
	}
	return nil
}

// skews returns clock skew information for all writers recorded in the
// document, given that the document was read between the supplied local
// times. It will return an error if the clock document is not valid, or
// if the times don't make sense.
func (doc clockDoc) skews(beginning, end time.Time) (map[string]Skew, error) {
	if err := doc.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	// beginning is expected to be earlier than end.
	// If it isn't, it could be ntp rolling the clock back slowly, so we add
	// a little wiggle room here.
	if end.Before(beginning) {
		// A later time, subtract an earlier time will give a positive duration.
		difference := beginning.Sub(end)
		if difference > 10*time.Millisecond {
			return nil, errors.Errorf("end of read window preceded beginning (%s)", difference)

		}
		beginning = end
	}
	skews := make(map[string]Skew)
	for writer, written := range doc.Writers {
		skews[writer] = Skew{
			LastWrite: toTime(written),
			Beginning: beginning,
			End:       end,
		}
	}
	return skews, nil
}

// newClockDoc returns an empty clockDoc for the supplied namespace.
func newClockDoc(namespace string) (*clockDoc, error) {
	doc := &clockDoc{
		Id:        clockDocId(namespace),
		Type:      typeClock,
		Namespace: namespace,
		Writers:   make(map[string]int64),
	}
	if err := doc.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return doc, nil
}
