// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
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

// For simplicity's sake, we impose the same restrictions on all strings used
// with the lease package: they may not be empty, and none of the following
// characters are allowed.
//   * '.' and '$' mean things to mongodb; we don't want to risk seeing them
//     in key names.
//   * '#' means something to the lease package and we don't want to risk
//     confusing ourselves.
//   * whitespace just seems like a bad idea.
const badCharacters = ".$# \t\r\n"

// validateString returns an error if the string is not valid.
func validateString(s string) error {
	if s == "" {
		return errors.New("string is empty")
	}
	if strings.ContainsAny(s, badCharacters) {
		return errors.New("string contains forbidden characters")
	}
	return nil
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

	// EnvUUID exists because state.multiEnvRunner can't handle structs
	// without `bson:"env-uuid"` fields. It's not necessary for the logic
	// in this package, though.
	EnvUUID string `bson:"env-uuid"`

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
	// state.multiEnvRunner prepends environ ids in our documents, and
	// state.envStateCollection does not strip them out.
	if !strings.HasSuffix(doc.Id, leaseDocId(doc.Namespace, doc.Name)) {
		return errors.Errorf("inconsistent _id")
	}
	if err := validateString(doc.Holder); err != nil {
		return errors.Annotatef(err, "invalid holder")
	}
	if doc.Expiry == 0 {
		return errors.Errorf("invalid expiry")
	}
	if err := validateString(doc.Writer); err != nil {
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

	// EnvUUID exists because state.multiEnvRunner can't handle structs
	// without `bson:"env-uuid"` fields. It's not necessary for the logic
	// in this package, though.
	EnvUUID string `bson:"env-uuid"`

	// Writers holds a the latest acknowledged time for every known client.
	Writers map[string]int64 `bson:"writers"`
}

// validate returns an error if any fields are invalid or inconsistent.
func (doc clockDoc) validate() error {
	if doc.Type != typeClock {
		return errors.Errorf("invalid type %q", doc.Type)
	}
	// state.multiEnvRunner prepends environ ids in our documents, and
	// state.envStateCollection does not strip them out.
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
func (doc clockDoc) skews(readAfter, readBefore time.Time) (map[string]Skew, error) {
	if err := doc.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if readBefore.Before(readAfter) {
		return nil, errors.New("end of read window preceded beginning")
	}
	skews := make(map[string]Skew)
	for writer, written := range doc.Writers {
		skews[writer] = Skew{
			LastWrite:  toTime(written),
			ReadAfter:  readAfter,
			ReadBefore: readBefore,
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
