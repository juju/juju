// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/wrench"
)

// NewStore returns a new Store using the supplied config, or an error. Any
// of the following situations will prevent store creation:
//  * invalid config
//  * invalid lease data stored in the namespace
// ...but a returned Store will hold a recent cache of lease data and be ready
// to use.
// Stores do not need to be cleaned up themselves, but they will not function
// past the lifetime of their configured Mongo.
func NewStore(config StoreConfig) (lease.Store, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	loggerName := fmt.Sprintf("state.lease.%s.%s", config.Namespace, config.Id)
	logger := loggo.GetLogger(loggerName)
	store := &store{
		config: config,
		logger: logger,
	}
	if err := store.Refresh(); err != nil {
		return nil, errors.Trace(err)
	}
	return store, nil
}

// store implements the lease.Store interface.
type store struct {
	mu sync.Mutex

	// config holds resources and configuration necessary to store leases.
	config StoreConfig

	// logger holds a logger unique to this lease Store.
	logger loggo.Logger

	// entries records recent information about leases.
	entries map[string]entry

	// globalTime records the most recently obtained global clock time.
	globalTime time.Time
}

// Leases is part of the lease.Store interface.
func (store *store) Leases() map[lease.Key]lease.Info {
	store.mu.Lock()
	defer store.mu.Unlock()
	localTime := store.config.LocalClock.Now()
	leases := make(map[lease.Key]lease.Info)
	for name, entry := range store.entries {
		globalExpiry := entry.start.Add(entry.duration)
		remaining := globalExpiry.Sub(store.globalTime)
		localExpiry := localTime.Add(remaining)
		key := lease.Key{
			Namespace: store.config.Namespace,
			ModelUUID: store.config.ModelUUID,
			Lease:     name,
		}
		leases[key] = lease.Info{
			Holder:   entry.holder,
			Expiry:   localExpiry,
			Trapdoor: store.assertOpTrapdoor(name, entry.holder),
		}
	}
	return leases
}

// ClaimLease is part of the lease.Store interface.
func (store *store) ClaimLease(key lease.Key, request lease.Request) error {
	return store.request(key.Lease, request, store.claimLeaseOps, "claiming")
}

// ExtendLease is part of the lease.Store interface.
func (store *store) ExtendLease(key lease.Key, request lease.Request) error {
	return store.request(key.Lease, request, store.extendLeaseOps, "extending")
}

// opsFunc is used to make the signature of the request method somewhat readable.
type opsFunc func(name string, request lease.Request) ([]txn.Op, entry, error)

// request implements ClaimLease and ExtendLease.
func (store *store) request(name string, request lease.Request, getOps opsFunc, verb string) error {
	if err := lease.ValidateString(name); err != nil {
		return errors.Annotatef(err, "invalid name")
	}
	if err := request.Validate(); err != nil {
		return errors.Annotatef(err, "invalid request")
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	// Close over cacheEntry to record in case of success.
	var cacheEntry entry
	err := store.config.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
		store.logger.Tracef("%s lease %q for %s (attempt %d)", verb, name, request, attempt)

		// On the first attempt, assume cache is good.
		if attempt > 0 {
			if err := store.refresh(false); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// It's possible that the request is for an "extension" isn't an
		// extension at all; this isn't a problem, but does require separate
		// handling.
		ops, nextEntry, err := getOps(name, request)
		cacheEntry = nextEntry
		if errors.Cause(err) == errNoExtension {
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	})

	if err != nil {
		if errors.Cause(err) == lease.ErrInvalid {
			return lease.ErrInvalid
		}
		return errors.Annotate(err, "cannot satisfy request")
	}

	// Update the cache for this lease only.
	store.entries[name] = cacheEntry
	return nil
}

// ExpireLease is part of the Store interface.
func (store *store) ExpireLease(key lease.Key) error {
	name := key.Lease
	if err := lease.ValidateString(name); err != nil {
		return errors.Annotatef(err, "invalid name")
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	// No cache updates needed, only deletes; no closure here.
	err := store.config.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
		store.logger.Tracef("expiring lease %q (attempt %d)", name, attempt)

		// On the first attempt, assume cache is good.
		if attempt > 0 {
			if err := store.refresh(false); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// No special error handling here.
		ops, err := store.expireLeaseOps(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	})

	if err != nil {
		if errors.Cause(err) == lease.ErrInvalid {
			return lease.ErrInvalid
		}
		return errors.Trace(err)
	}

	// Uncache this lease entry.
	delete(store.entries, name)
	return nil
}

// PinLease is part of the Store interface.
func (store *store) PinLease(key lease.Key, entity names.Tag) error {
	return errors.NotImplementedf("pinning for legacy leases")
}

// UnpinLease is part of the Store interface.
func (store *store) UnpinLease(key lease.Key, entity names.Tag) error {
	return errors.NotImplementedf("unpinning for legacy leases")
}

// Refresh is part of the Store interface.
func (store *store) Refresh() error {
	store.mu.Lock()
	defer store.mu.Unlock()

	return store.refresh(true)
}

func (store *store) refresh(refreshGlobalTime bool) error {
	store.logger.Tracef("refreshing")
	if wrench.IsActive("lease", "refresh") {
		return errors.New("wrench active")
	}

	collection, closer := store.config.Mongo.GetCollection(store.config.Collection)
	defer closer()
	entries, err := store.readEntries(collection)
	if err != nil {
		return errors.Trace(err)
	}
	if refreshGlobalTime {
		if _, err := store.refreshGlobalTime(); err != nil {
			return errors.Trace(err)
		}
	}
	store.entries = entries
	return nil
}

// readEntries reads all lease data for the store's namespace.
func (store *store) readEntries(collection mongo.Collection) (map[string]entry, error) {

	// Read all lease documents in the store's namespace.
	query := bson.M{
		fieldNamespace: store.config.Namespace,
	}
	iter := collection.Find(query).Iter()

	// Extract valid entries for each one.
	entries := make(map[string]entry)
	var leaseDoc leaseDoc
	for iter.Next(&leaseDoc) {
		name, entry, err := leaseDoc.entry()
		if err != nil {
			if err := iter.Close(); err != nil {
				store.logger.Debugf("failed to close lease docs iterator: %s", err)
			}
			return nil, errors.Annotatef(err, "corrupt lease document %q", leaseDoc.Id)
		}
		entries[name] = entry
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return entries, nil
}

// claimLeaseOps returns the []txn.Op necessary to claim the supplied lease
// until duration in the future, and a cache entry corresponding to the values
// that will be written if the transaction succeeds. If the claim would conflict
// with cached state, it returns lease.ErrInvalid.
func (store *store) claimLeaseOps(name string, request lease.Request) ([]txn.Op, entry, error) {

	// We can't claim a lease that's already held.
	if _, found := store.entries[name]; found {
		return nil, entry{}, lease.ErrInvalid
	}

	globalTime, err := store.refreshGlobalTime()
	if err != nil {
		return nil, entry{}, errors.Annotate(err, "refreshing global time")
	}

	return claimLeaseOps(
		store.config.Namespace, name, request.Holder,
		store.config.Id, store.config.Collection,
		globalTime, request.Duration,
	)
}

// ClaimLeaseOps returns txn.Ops to write a new lease. The txn.Ops
// will fail if the lease document exists, regardless of whether it
// has expired.
func ClaimLeaseOps(
	namespace, name, holder, writer, collection string,
	globalTime time.Time, duration time.Duration,
) ([]txn.Op, error) {
	ops, _, err := claimLeaseOps(
		namespace, name, holder, writer, collection,
		globalTime, duration,
	)
	return ops, errors.Trace(err)
}

func claimLeaseOps(
	namespace, name, holder, writer, collection string,
	globalTime time.Time, duration time.Duration,
) ([]txn.Op, entry, error) {
	newEntry := entry{
		holder:   holder,
		start:    globalTime,
		duration: duration,
		writer:   writer,
	}
	leaseDoc, err := newLeaseDoc(namespace, name, newEntry)
	if err != nil {
		return nil, entry{}, errors.Trace(err)
	}
	claimLeaseOp := txn.Op{
		C:      collection,
		Id:     leaseDoc.Id,
		Assert: txn.DocMissing,
		Insert: leaseDoc,
	}
	return []txn.Op{claimLeaseOp}, newEntry, nil
}

// LookupLease returns a lease claim if it exists.
// If it doesn't exist, expect to get an mgo.NotFoundError, otherwise expect to get
func LookupLease(coll mongo.Collection, namespace, name string) (leaseDoc, error) {
	var doc leaseDoc
	err := coll.FindId(leaseDocId(namespace, name)).One(&doc)
	if err != nil {
		return leaseDoc{}, err
	}
	return doc, nil
}

// extendLeaseOps returns the []txn.Op necessary to extend the supplied lease
// until duration in the future, and a cache entry corresponding to the values
// that will be written if the transaction succeeds. If the supplied lease
// already extends far enough that no operations are required, it will return
// errNoExtension. If the extension would conflict with cached state, it will
// return lease.ErrInvalid.
func (store *store) extendLeaseOps(name string, request lease.Request) ([]txn.Op, entry, error) {

	// Reject extensions when there's no lease, or the holder doesn't match.
	lastEntry, found := store.entries[name]
	if !found {
		return nil, entry{}, lease.ErrInvalid
	}
	if lastEntry.holder != request.Holder {
		return nil, entry{}, lease.ErrInvalid
	}

	globalTime, err := store.refreshGlobalTime()
	if err != nil {
		return nil, entry{}, errors.Annotate(err, "refreshing global time")
	}
	expiry := globalTime.Add(request.Duration)
	if !expiry.After(lastEntry.start.Add(lastEntry.duration)) {
		// The "extended" lease expires at the same time as, or before,
		// the existing lease. Done.
		return nil, lastEntry, errNoExtension
	}

	// We know we need to write a lease; we know when it needs to expire; we
	// know what needs to go into the local cache:
	nextEntry := entry{
		holder:   lastEntry.holder,
		start:    globalTime,
		duration: request.Duration,
		writer:   store.config.Id,
	}

	// ...and what needs to change in the database, and how to ensure the
	// change is still valid when it's executed.
	extendLeaseOp := txn.Op{
		C:  store.config.Collection,
		Id: store.leaseDocId(name),
		Assert: bson.M{
			fieldHolder:   lastEntry.holder,
			fieldStart:    toInt64(lastEntry.start),
			fieldDuration: lastEntry.duration,
			fieldWriter:   lastEntry.writer,
		},
		Update: bson.M{"$set": bson.M{
			fieldStart:    toInt64(globalTime),
			fieldDuration: nextEntry.duration,
			fieldWriter:   store.config.Id,
		}},
	}

	ops := []txn.Op{extendLeaseOp}
	return ops, nextEntry, nil
}

// expireLeaseOps returns the []txn.Op necessary to vacate the lease. If the
// expiration would conflict with cached state, it will return an error with
// a Cause of ErrInvalid.
func (store *store) expireLeaseOps(name string) ([]txn.Op, error) {

	// We can't expire a lease that doesn't exist.
	lastEntry, found := store.entries[name]
	if !found {
		return nil, lease.ErrInvalid
	}

	// We also can't expire a lease whose expiry time may be in the future.
	latestExpiry := lastEntry.start.Add(lastEntry.duration)
	if !store.globalTime.After(latestExpiry) {
		globalTime, err := store.refreshGlobalTime()
		if err != nil {
			return nil, errors.Annotate(err, "refreshing global time")
		}
		if !globalTime.After(latestExpiry) {
			return nil, errors.Annotatef(lease.ErrInvalid, "lease %q expires in the future", name)
		}
	}

	// The database change is simple, and depends on the lease doc being
	// untouched since we looked:
	expireLeaseOp := txn.Op{
		C:  store.config.Collection,
		Id: store.leaseDocId(name),
		Assert: bson.M{
			fieldHolder:   lastEntry.holder,
			fieldStart:    toInt64(lastEntry.start),
			fieldDuration: lastEntry.duration,
			fieldWriter:   lastEntry.writer,
		},
		Remove: true,
	}

	ops := []txn.Op{expireLeaseOp}
	return ops, nil
}

// assertOpTrapdoor returns a lease.Trapdoor that will replace a supplied
// *[]txn.Op with one that asserts that the holder still holds the named lease.
func (store *store) assertOpTrapdoor(name, holder string) lease.Trapdoor {
	op := txn.Op{
		C:  store.config.Collection,
		Id: store.leaseDocId(name),
		Assert: bson.M{
			fieldHolder: holder,
		},
	}
	return func(out interface{}) error {
		outPtr, ok := out.(*[]txn.Op)
		if !ok {
			return errors.NotValidf("expected *[]txn.Op; %T", out)
		}
		*outPtr = []txn.Op{op}
		return nil
	}
}

func (store *store) refreshGlobalTime() (time.Time, error) {
	store.logger.Tracef("refreshing global time")
	globalTime, err := store.config.GlobalClock.Now()
	if err != nil {
		return time.Time{}, errors.Trace(err)
	}
	store.logger.Tracef("global time is %s", globalTime)
	store.globalTime = globalTime
	return globalTime, nil
}

// leaseDocId returns the id of the named lease document in the store's
// namespace.
func (store *store) leaseDocId(name string) string {
	return leaseDocId(store.config.Namespace, name)
}

// entry holds the details of a lease and how it was written.
type entry struct {
	// holder identifies the current holder of the lease.
	holder string

	// start is the global time at which the lease started.
	start time.Time

	// duration is the duration for which the lease is valid,
	// from the start time.
	duration time.Duration

	// writer identifies the store that wrote the lease.
	writer string
}

// errNoExtension is used internally to avoid running unnecessary transactions.
var errNoExtension = errors.New("lease needs no extension")
