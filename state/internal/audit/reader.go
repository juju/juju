// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/audit"
	"github.com/juju/juju/mongo/utils"
)

// FetchedAuditEntry encapsulates a fetch of an audit entry or an
// error that occured during fetching.
type FetchedAuditEntry struct {

	// Value is the audit entry that was fetched.
	Value audit.AuditEntry

	// Error contains any errors that occurred.
	Error error
}

// Iterator defines a type which will be used to loop over database
// records.
type Iterator interface {

	// Next persists the next record to the memory address provided in
	// the argument, and returns whether or not the fetch was
	// successful.
	Next(result interface{}) bool

	// Close closes the iterator and returns any errors that occurred
	// during iteration.
	Close() error
}

// AuditTailerContext provides information necessary for
// NewAuditTailer to function correctly.
type AuditTailerContext struct {

	// Done signals the AuditTailer to interrupt and tear everything
	// down cleanly.
	Done <-chan struct{}

	// Logger is the log instance to write to.
	Logger loggo.Logger

	// OpenAuditIter is a function which will open a new Iterator for
	// looping over audit entries with timestamps after the given
	// time.
	OpenAuditIter func(time.Time) Iterator
}

// NewAuditTailer returns a channel of FetchedAuditEntry and returns
// values on this channel asynchronously.
//
// The values returned will be pulled from an Iterator created by the
// OpenAuditIter function passed in. The reader will re-open an
// Iterator should it be necessary to provide a continuous stream of
// records to listeners. A high-watermark, or max timestamp seen,
// value is tracked so that if the iterator need be reopened, it will
// do so utilizing the last seen value. All operations can be
// interrupted and cleanly torn down by closing the done channel
// passed in.
func NewAuditTailer(ctx AuditTailerContext, after time.Time) <-chan FetchedAuditEntry {
	records := make(chan FetchedAuditEntry)
	highWatermark := after // Rename for readability
	openAuditIter := func() Iterator {
		return cancellable(ctx.Done, ctx.OpenAuditIter, highWatermark)
	}
	go func() {
		defer close(records)

		auditCollIter := openAuditIter()

		// Loop infinitely until we're told to stop by closing the
		// done channel. Even though the Iterator's Next method will
		// block until more records are returned, if it fails for some
		// reason, we will open a new iterator at the high watermark
		// and keep streaming.
		for {
			var auditRecord FetchedAuditEntry
			var doc auditEntryDoc
			if auditCollIter.Next(&doc) {

				auditRecord.Value, auditRecord.Error = auditEntryFromDoc(doc)
				auditRecord.Error = errors.Annotate(auditRecord.Error, "cannot convert audit doc")

				if auditRecord.Error == nil && highWatermark.Before(auditRecord.Value.Timestamp) {
					highWatermark = auditRecord.Value.Timestamp
				}

			} else {

				auditRecord.Error = errors.Annotate(auditCollIter.Close(), "cannot convert audit doc")

				select {
				case <-ctx.Done:
					return
				default:
					// Open a new cursor where we left off
					auditCollIter = openAuditIter()

					// We have nothing to send; we've reopened the
					// Iterator so we should try reading again.
					if auditRecord.Error == nil {
						continue
					}
				}
			}

			select {
			case <-ctx.Done:
				return
			case records <- auditRecord:
			}
		}
	}()
	return records
}

func cancellable(done <-chan struct{}, openIter func(time.Time) Iterator, after time.Time) Iterator {
	iter := openIter(after)
	go func() {
		<-done
		iter.Close()
	}()
	return iter
}

func auditEntryFromDoc(doc auditEntryDoc) (audit.AuditEntry, error) {
	var timestamp time.Time
	if err := timestamp.UnmarshalText([]byte(doc.Timestamp)); err != nil {
		return audit.AuditEntry{}, errors.Trace(err)
	}

	return audit.AuditEntry{
		JujuServerVersion: doc.JujuServerVersion,
		ModelUUID:         doc.ModelUUID,
		Timestamp:         timestamp,
		RemoteAddress:     doc.RemoteAddress,
		OriginType:        doc.OriginType,
		OriginName:        doc.OriginName,
		Operation:         doc.Operation,
		Data:              utils.UnescapeKeys(doc.Data),
	}, nil
}
