// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"
)

// StorageConfig contains the resources and information required to create
// a Storage.
type StorageConfig struct {

	// Clock exposes the wall-clock time to a Storage.
	Clock Clock

	// Mongo exposes the mgo[/txn] capabilities required by a Storage.
	Mongo Mongo

	// Collection identifies the MongoDB collection in which lease data is
	// persisted. Multiple storages can use the same collection without
	// interfering with each other, so long as they use different namespaces.
	Collection string

	// Namespace identifies the domain of the lease manager; storage instances
	// which share a namespace and a collection will collaborate.
	Namespace string

	// Instance uniquely identifies the storage instance. Storage instances will
	// fail to collaborate if any two concurrently identify as the same instance.
	Instance string
}

// Validate returns an error if the supplied config is not valid.
func (config StorageConfig) Validate() error {
	return fmt.Errorf("validation!")
}

// NewStorage returns a new Storage using the supplied config. If the config
// fails to validate, or if the collection lacks a clock document for the
// configured namespace and none can be created, it will return an error.
func NewStorage(config StorageConfig) (Storage, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	storage := &storage{
		config:  config,
		entries: make(map[string]entry),
		skews:   make(map[string]entry),
	}
	if err := storage.ensureClockDoc(); err != nil {
		return nil, errors.Trace(err)
	}
	return storage, nil
}

// storage implements the Storage interface.
type storage struct {
	config  StorageConfig
	entries map[string]entry
	skews   map[string]Skew
}

// Refresh is part of the Storage interface.
func (s *storage) Refresh() error {
	// Always read entries before skews, because skews are written before
	// entries; we increase the risk of reading older skew data, but (should)
	// eliminate the risk of reading an entry whose writer is not present
	// in the skews data.
	entries, err := s.readEntries(collection)
	if err != nil {
		return errors.Trace(err)
	}
	skews, err := s.readSkews(collection)
	if err != nil {
		return errors.Trace(err)
	}

	// Check we're not missing any required clock information before
	// updating our local state.
	for lease, entry := range entries {
		if _, found := skews[entry.writer]; !found {
			return errors.Errorf("lease %q invalid: no clock data for %s", lease, entry.writer)
		}
	}
	s.skews = skews
	s.entries = entries
	return nil
}

// Leases is part of the Storage interface.
func (s *storage) Leases() map[string]Info {
	leases := make(map[string]Info)
	for lease, entry := range s.entries {
		skew := s.skews[entry.writer]
		leases[lease] = Info{
			Holder:         entry.holder,
			AssertOp:       s.assertOp(lease, entry.holder),
			EarliestExpiry: skew.Earliest(entry.expiry),
			LatestExpiry:   skew.Latest(entry.expiry),
		}
	}
	return leases
}

// ClaimLease is part of the Storage interface.
func (s *storage) ClaimLease(lease, holder string, duration time.Duration) error {

	// Close over cacheEntry to record in case of success.
	var cacheEntry entry
	err := s.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {

		// On the first attempt, assume cache is good.
		if attempt > 0 {
			if err := s.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// No special error handling here.
		ops, nextEntry, err := s.claimLeaseOps(lease, holder, duration)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cacheEntry = nextEntry
		return ops, nil
	})

	// Unwrap ErrInvalid if necessary.
	if errors.Cause(err) == ErrInvalid {
		return ErrInvalid
	} else if err != nil {
		return errors.Trace(err)
	}

	// Update the cache for this lease only.
	s.entries[lease] = cacheEntry
	return nil
}

// ExtendLease is part of the Storage interface.
func (s *storage) ExtendLease(lease, holder string, duration time.Duration) error {

	// Close over cacheEntry to record in case of success.
	var cacheEntry entry
	err := s.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {

		// On the first attempt, assume cache is good.
		if attempt > 0 {
			if err := s.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// It's possible that the "extension" isn't an extension at all; this
		// isn't a problem, but does require separate handling.
		ops, nextEntry, err := s.extendLeaseOps(lease, holder, duration)
		if errors.Cause(err) == errNoExtension {
			cacheEntry = lastEntry
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		cacheEntry = nextEntry
		return ops, nil
	})

	// Unwrap ErrInvalid if necessary.
	if errors.Cause(err) == ErrInvalid {
		return ErrInvalid
	} else if err != nil {
		return errors.Trace(err)
	}

	// Update the cache for this lease only.
	s.entries[lease] = cacheEntry
	return nil
}

// ExpireLease is part of the Storage interface.
func (s *storage) ExpireLease(lease string) error {

	// No cache updates needed, only deletes; no closure here.
	err := s.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {

		// On the first attempt, assume cache is good.
		if attempt > 0 {
			if err := s.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// No special error handling here.
		ops, err := s.expireLeaseOps(lease, holder)
		if err != nil {
			return errors.Trace(err)
		}
		return ops, nil
	})

	// Unwrap ErrInvalid if necessary.
	if errors.Cause(err) == ErrInvalid {
		return ErrInvalid
	} else if err != nil {
		return errors.Trace(err)
	}

	// Uncache this lease entry.
	delete(s.entries, lease)
	return nil
}

// readEntries reads all lease data for the storage's namespace.
func (s *storage) readEntries(collection *mgo.Collection) (map[string]entry, error) {

	// Read all lease documents in the storage's namespace.
	query := bson.M{
		fieldType:      typeLease,
		fieldNamespace: s.config.Namespace,
	}
	iter := collection.Find(query).Iter()

	// Extract valid entries for each one.
	entries := make(map[string]entry)
	var leaseDoc leaseDoc
	for iter.Next(&leaseDoc) {
		if lease, entry, err := leaseDoc.entry(); err != nil {
			return nil, errors.Trace(err)
		} else {
			entries[lease] = entry
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return entries, nil
}

// readSkews reads all clock data for the storage's namespace.
func (s *storage) readSkews(collection *mgo.Collection) (map[string]Skew, error) {

	// Read the clock document, recording the time before and after completion.
	readBefore := s.config.Clock.Now()
	var clockDoc clockDoc
	if err := collection.FindId(s.clockDocId()).One(&clockDoc); err != nil {
		return nil, errors.Trace(err)
	}
	readAfter := s.config.Clock.Now()

	// Create skew entries for each known writer...
	skews, err := clockDoc.skews(readAfter, readBefore)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// ...and overwrite our own with a zero skew, which will DTRT.
	skews[s.config.Instance] = Skew{}
	return skews, nil
}

// claimLeaseOps returns the []txn.Op necessary to claim the supplied lease
// until duration in the future, and a cache entry corresponding to the values
// that will be written if the transaction succeeds. If the claim would conflict
// with cached state, it returns ErrInvalid.
func (s *storage) claimLeaseOps(lease, holder string, duration time.Duration) ([]txn.Op, entry, error) {

	// We can't claim a lease that's already held.
	if _, found := s.entries[lease]; found {
		return nil, entry{}, ErrInvalid
	}

	// According to the local clock, we want the lease to extend until
	// <duration> in the future.
	now := s.config.Clock.Now()
	expiry := now.Add(duration)
	entry := entry{
		holder:  holder,
		expiry:  expiry,
		writer:  s.config.Instance,
		written: now,
	}

	// We need to write the entry to the database in a specific format.
	leaseDoc, err := newLeaseDoc(namespace, lease, entry)
	if err != nil {
		return nil, entry{}, errors.Trace(err)
	}
	extendLeaseOp := txn.Op{
		C:      s.config.Collection,
		Id:     leaseDoc.Id,
		Assert: txn.DocMissing,
		Insert: leaseDoc,
	}

	// We always write a clock-update operation *before* writing lease info.
	writeClockOp := s.writeClockOp(now)
	ops := []txn.Op{writeClockOp, extendLeaseOp}
	return ops, nextEntry, nil
}

// extendLeaseOps returns the []txn.Op necessary to extend the supplied lease
// until duration in the future, and a cache entry corresponding to the values
// that will be written if the transaction succeeds. If the supplied lease
// already extends far enough that no operations are required, it will return
// errNoExtension. If the extension would conflict with cached state, it will
// return ErrInvalid.
func (s *storage) extendLeaseOps(lease, holder string, duration time.Duration) ([]txn.Op, entry, error) {

	// Reject extensions when there's no lease, or the holder doesn't match.
	lastEntry, found := s.entries[lease]
	if !found {
		return nil, ErrInvalid
	}
	if holder != lastEntry.holder {
		return nil, ErrInvalid
	}

	// According to the local clock, we want the lease to extend until
	// <duration> in the future.
	now := s.config.Clock.Now()
	expiry := now.Add(duration)

	// We don't know what time the original writer thinks it is, but we
	// can figure out the earliest and latest local times at which it
	// could be expecting its original lease to expire.
	skew := s.skews[lastEntry.writer]
	if expiry.Before(skew.Earliest(lastEntry.expiry)) {
		// The "extended" lease will certainly expire before the
		// existing lease could. Done.
		return nil, entry{}, errNoExtension
	}
	latestExpiry := skew.Latest(lastEntry.expiry)
	if expiry.Before(latestExpiry) {
		// The lease might be long enough, but we're not sure, so we'll
		// write a new one that definitely is long enough; but we must
		// be sure that the new lease has an expiry time such that no
		// other writer can consider it to have expired before the
		// original writer considers its own lease to have expired.
		expiry = latestExpiry
	}

	// We know we need to write a lease; we know when it needs to expire; we
	// know what needs to go into the local cache:
	nextEntry := entry{
		holder:  lastEntry.holder,
		expiry:  expiry,
		writer:  s.config.Instance,
		written: now,
	}

	// ...and what needs to change in the database, and how to ensure the
	// change is still valid when it's executed.
	extendLeaseOp := txn.Op{
		C:  s.config.Collection,
		Id: s.leaseDocId(lease),
		Assert: bson.M{
			fieldLeaseHolder: lastEntry.holder,
			fieldLeaseExpiry: lastEntry.expiry.UnixNano(),
			fieldLeaseWriter: lastEntry.writer,
		},
		Update: bson.M{
			fieldLeaseExpiry:  expiry.UnixNano(),
			fieldLeaseWriter:  s.config.Instance,
			fieldLeaseWritten: nextEntry.written.UnixNano(),
		},
	}

	// We always write a clock-update operation *before* writing lease info.
	writeClockOp := s.writeClockOp(now)
	ops := []txn.Op{writeClockOp, extendLeaseOp}
	return ops, nextEntry, nil
}

// expireLeaseOps returns the []txn.Op necessary to vacate the lease. If the
// expiration would conflict with cached state, it will return ErrInvalid.
func (s *storage) expireLeaseOps(lease string) ([]txn.Op, error) {

	// We can't expire a lease that doesn't exist.
	lastEntry, found := s.entries[lease]
	if !found {
		return nil, ErrInvalid
	}

	// We also can't expire a lease that hasn't actually expired.
	skew := s.skews[lastEntry.writer]
	latestExpiry := skew.Latest(lastEntry.expiry)
	now := s.config.Clock.Now()
	if !now.After(latestExpiry) {
		return nil, ErrInvalid
	}

	// The database change is simple, and depends on the lease doc being
	// untouched since we looked:
	expireLeaseOp := txn.Op{
		C:  s.config.Collection,
		Id: s.leaseDocId(lease),
		Assert: bson.M{
			fieldLeaseHolder: lastEntry.holder,
			fieldLeaseExpiry: lastEntry.expiry.UnixNano(),
			fieldLeaseWriter: lastEntry.Writer,
		},
		Remove: true,
	}

	// We always write a clock-update operation *before* writing lease info.
	// Removing a lease document counts as writing lease info.
	writeClockOp := s.writeClockOp(now)
	ops := []txn.Op{writeClockOp, expireLeaseOp}
	return ops, nil
}

// ensureClockDoc returns an error if it can neither find nor create a
// valid clock document for the storage's namespace.
func (s *storage) ensureClockDoc() error {

	collection, closer := s.config.Mongo.GetCollection(s.config.Collection)
	defer closer()

	clockDocId := s.clockDocId()
	err := s.config.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
		var clockDoc clockDoc
		err := collection.FindId(clockDocId).One(&clockDoc)
		if err == nil {
			if err := clockDoc.validate(); err != nil {
				return nil, errors.Trace(err)
			}
			return nil, jujutxn.ErrNoOperations
		} else if err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		newClockDoc, err := newClockDoc(s.config.Namespace)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []txn.Op{{
			C:      s.config.Collection,
			Id:     clockDocId,
			Assert: txn.DocMissing,
			Insert: newClockDoc,
		}}, nil
	})
	return errors.Trace(err)
}

// writeClockOp returns a txn.Op which writes the supplied time to the writer's
// field in the skew doc, and aborts if a more recent time has been recorded for
// that writer.
func (s *storage) writeClockOp(now time.Time) txn.Op {
	nowUnix := now.UnixNano()
	return txn.Op{
		C:  s.config.Collection,
		Id: s.clockDocId(),
		Assert: bson.M{
			s.config.Instance: bson.M{"$lt": nowUnix},
		},
		Update: bson.M{
			"$set": bson.M{s.config.Instance: nowUnix},
		},
	}
}

// assertOp returns a txn.Op which will succeed only if holder holds lease.
func (s *storage) assertOp(lease, holder string) txn.Op {
	return txn.Op{
		C:  s.config.Collection,
		Id: s.leaseDocId(lease),
		Assert: bson.M{
			fieldLeaseHolder: holder,
		},
	}
}

// clockDocId returns the id of the clock document in the storage's namespace.
func (s *storage) clockDocId() string {
	return clockDocId(s.config.Namespace)
}

// leaseDocId returns the id of the named lease document in the storage's
// namespace.
func (s *storage) leaseDocId(lease string) string {
	return leaseDocId(s.config.Namespace, lease)
}

// entry holds the details of a lease and how it was written. The time values
// are always expressed in the writer's local time.
type entry struct {
	// holder identifies the current holder of the lease.
	holder string

	// expiry is the time at which the lease is safe to remove.
	expiry time.Time

	// writer identifies the storage instance that wrote the lease.
	writer string

	// written is the earliest possible time the lease could have been written.
	written time.Time
}

// errNoExtension is used internally to avoid running unnecessary transactions.
var errNoExtension = errors.New("lease needs no extension")
