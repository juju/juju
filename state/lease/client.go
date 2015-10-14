// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
)

// NewClient returns a new Client using the supplied config, or an error. Any
// of the following situations will prevent client creation:
//  * invalid config
//  * invalid clock data stored in the namespace
//  * invalid lease data stored in the namespace
// ...but a returned Client will hold a recent cache of lease data and be ready
// to use.
// Clients do not need to be cleaned up themselves, but they will not function
// past the lifetime of their configured Mongo.
func NewClient(config ClientConfig) (Client, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	loggerName := fmt.Sprintf("state.lease.%s.%s", config.Namespace, config.Id)
	logger := loggo.GetLogger(loggerName)
	client := &client{
		config: config,
		logger: logger,
	}
	if err := client.ensureClockDoc(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := client.Refresh(); err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

// client implements the Client interface.
type client struct {

	// config holds resources and configuration necessary to store leases.
	config ClientConfig

	// logger holds a logger unique to this lease Client.
	logger loggo.Logger

	// entries records recent information about leases.
	entries map[string]entry

	// skews records recent information about remote writers' clocks.
	skews map[string]Skew
}

// Leases is part of the Client interface.
func (client *client) Leases() map[string]Info {
	leases := make(map[string]Info)
	for name, entry := range client.entries {
		skew := client.skews[entry.writer]
		leases[name] = Info{
			Holder:   entry.holder,
			Expiry:   skew.Latest(entry.expiry),
			AssertOp: client.assertOp(name, entry.holder),
		}
	}
	return leases
}

// ClaimLease is part of the Client interface.
func (client *client) ClaimLease(name string, request Request) error {
	return client.request(name, request, client.claimLeaseOps, "claiming")
}

// ExtendLease is part of the Client interface.
func (client *client) ExtendLease(name string, request Request) error {
	return client.request(name, request, client.extendLeaseOps, "extending")
}

// opsFunc is used to make the signature of the request method somewhat readable.
type opsFunc func(name string, request Request) ([]txn.Op, entry, error)

// request implements ClaimLease and ExtendLease.
func (client *client) request(name string, request Request, getOps opsFunc, verb string) error {
	if err := validateString(name); err != nil {
		return errors.Annotatef(err, "invalid name")
	}
	if err := request.Validate(); err != nil {
		return errors.Annotatef(err, "invalid request")
	}

	// Close over cacheEntry to record in case of success.
	var cacheEntry entry
	err := client.config.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
		client.logger.Tracef("%s lease %q for %s (attempt %d)", verb, name, request, attempt)

		// On the first attempt, assume cache is good.
		if attempt > 0 {
			if err := client.Refresh(); err != nil {
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

	// Unwrap ErrInvalid if necessary.
	if errors.Cause(err) == ErrInvalid {
		return ErrInvalid
	}
	if err != nil {
		return errors.Trace(err)
	}

	// Update the cache for this lease only.
	client.entries[name] = cacheEntry
	return nil
}

// ExpireLease is part of the Client interface.
func (client *client) ExpireLease(name string) error {
	if err := validateString(name); err != nil {
		return errors.Annotatef(err, "invalid name")
	}

	// No cache updates needed, only deletes; no closure here.
	err := client.config.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
		client.logger.Tracef("expiring lease %q (attempt %d)", name, attempt)

		// On the first attempt, assume cache is good.
		if attempt > 0 {
			if err := client.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// No special error handling here.
		ops, err := client.expireLeaseOps(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	})

	// Unwrap ErrInvalid if necessary.
	if errors.Cause(err) == ErrInvalid {
		return ErrInvalid
	}
	if err != nil {
		return errors.Trace(err)
	}

	// Uncache this lease entry.
	delete(client.entries, name)
	return nil
}

// Refresh is part of the Client interface.
func (client *client) Refresh() error {
	client.logger.Tracef("refreshing")

	// Always read entries before skews, because skews are written before
	// entries; we increase the risk of reading older skew data, but (should)
	// eliminate the risk of reading an entry whose writer is not present
	// in the skews data.
	collection, closer := client.config.Mongo.GetCollection(client.config.Collection)
	defer closer()
	entries, err := client.readEntries(collection)
	if err != nil {
		return errors.Trace(err)
	}
	skews, err := client.readSkews(collection)
	if err != nil {
		return errors.Trace(err)
	}

	// Check we're not missing any required clock information before
	// updating our local state.
	for name, entry := range entries {
		if _, found := skews[entry.writer]; !found {
			return errors.Errorf("lease %q invalid: no clock data for %s", name, entry.writer)
		}
	}
	client.skews = skews
	client.entries = entries
	return nil
}

// ensureClockDoc returns an error if it can neither find nor create a
// valid clock document for the client's namespace.
func (client *client) ensureClockDoc() error {
	collection, closer := client.config.Mongo.GetCollection(client.config.Collection)
	defer closer()

	clockDocId := client.clockDocId()
	err := client.config.Mongo.RunTransaction(func(attempt int) ([]txn.Op, error) {
		client.logger.Tracef("checking clock %q (attempt %d)", clockDocId, attempt)
		var clockDoc clockDoc
		err := collection.FindId(clockDocId).One(&clockDoc)
		if err == nil {
			client.logger.Tracef("clock already exists")
			if err := clockDoc.validate(); err != nil {
				return nil, errors.Annotatef(err, "corrupt clock document")
			}
			return nil, jujutxn.ErrNoOperations
		}
		if err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		client.logger.Tracef("creating clock")
		newClockDoc, err := newClockDoc(client.config.Namespace)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []txn.Op{{
			C:      client.config.Collection,
			Id:     clockDocId,
			Assert: txn.DocMissing,
			Insert: newClockDoc,
		}}, nil
	})
	return errors.Trace(err)
}

// readEntries reads all lease data for the client's namespace.
func (client *client) readEntries(collection mongo.Collection) (map[string]entry, error) {

	// Read all lease documents in the client's namespace.
	query := bson.M{
		fieldType:      typeLease,
		fieldNamespace: client.config.Namespace,
	}
	iter := collection.Find(query).Iter()

	// Extract valid entries for each one.
	entries := make(map[string]entry)
	var leaseDoc leaseDoc
	for iter.Next(&leaseDoc) {
		name, entry, err := leaseDoc.entry()
		if err != nil {
			return nil, errors.Annotatef(err, "corrupt lease document %q", leaseDoc.Id)
		}
		entries[name] = entry
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return entries, nil
}

// readSkews reads all clock data for the client's namespace.
func (client *client) readSkews(collection mongo.Collection) (map[string]Skew, error) {

	// Read the clock document, recording the time before and after completion.
	readBefore := client.config.Clock.Now()
	var clockDoc clockDoc
	if err := collection.FindId(client.clockDocId()).One(&clockDoc); err != nil {
		return nil, errors.Trace(err)
	}
	readAfter := client.config.Clock.Now()
	if err := clockDoc.validate(); err != nil {
		return nil, errors.Annotatef(err, "corrupt clock document")
	}

	// Create skew entries for each known writer...
	skews, err := clockDoc.skews(readBefore, readAfter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If a writer was previously known to us, and has not written since last
	// time we read, we should keep the original skew, which is more accurate.
	for writer, skew := range client.skews {
		if skews[writer].LastWrite == skew.LastWrite {
			skews[writer] = skew
		}
	}

	// ...and overwrite our own with a zero skew, which will DTRT (assuming
	// nobody's reusing client ids across machines with different clocks,
	// which *should* never happen).
	skews[client.config.Id] = Skew{}
	return skews, nil
}

// claimLeaseOps returns the []txn.Op necessary to claim the supplied lease
// until duration in the future, and a cache entry corresponding to the values
// that will be written if the transaction succeeds. If the claim would conflict
// with cached state, it returns ErrInvalid.
func (client *client) claimLeaseOps(name string, request Request) ([]txn.Op, entry, error) {

	// We can't claim a lease that's already held.
	if _, found := client.entries[name]; found {
		return nil, entry{}, ErrInvalid
	}

	// According to the local clock, we want the lease to extend until
	// <duration> in the future.
	now := client.config.Clock.Now()
	expiry := now.Add(request.Duration)
	nextEntry := entry{
		holder: request.Holder,
		expiry: expiry,
		writer: client.config.Id,
	}

	// We need to write the entry to the database in a specific format.
	leaseDoc, err := newLeaseDoc(client.config.Namespace, name, nextEntry)
	if err != nil {
		return nil, entry{}, errors.Trace(err)
	}
	extendLeaseOp := txn.Op{
		C:      client.config.Collection,
		Id:     leaseDoc.Id,
		Assert: txn.DocMissing,
		Insert: leaseDoc,
	}

	// We always write a clock-update operation *before* writing lease info.
	writeClockOp := client.writeClockOp(now)
	ops := []txn.Op{writeClockOp, extendLeaseOp}
	return ops, nextEntry, nil
}

// extendLeaseOps returns the []txn.Op necessary to extend the supplied lease
// until duration in the future, and a cache entry corresponding to the values
// that will be written if the transaction succeeds. If the supplied lease
// already extends far enough that no operations are required, it will return
// errNoExtension. If the extension would conflict with cached state, it will
// return ErrInvalid.
func (client *client) extendLeaseOps(name string, request Request) ([]txn.Op, entry, error) {

	// Reject extensions when there's no lease, or the holder doesn't match.
	lastEntry, found := client.entries[name]
	if !found {
		return nil, entry{}, ErrInvalid
	}
	if lastEntry.holder != request.Holder {
		return nil, entry{}, ErrInvalid
	}

	// According to the local clock, we want the lease to extend until
	// <duration> in the future.
	now := client.config.Clock.Now()
	expiry := now.Add(request.Duration)

	// We don't know what time the original writer thinks it is, but we
	// can figure out the earliest and latest local times at which it
	// could be expecting its original lease to expire.
	skew := client.skews[lastEntry.writer]
	if expiry.Before(skew.Earliest(lastEntry.expiry)) {
		// The "extended" lease will certainly expire before the
		// existing lease could. Done.
		return nil, lastEntry, errNoExtension
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
		holder: lastEntry.holder,
		expiry: expiry,
		writer: client.config.Id,
	}

	// ...and what needs to change in the database, and how to ensure the
	// change is still valid when it's executed.
	extendLeaseOp := txn.Op{
		C:  client.config.Collection,
		Id: client.leaseDocId(name),
		Assert: bson.M{
			fieldLeaseHolder: lastEntry.holder,
			fieldLeaseExpiry: toInt64(lastEntry.expiry),
			fieldLeaseWriter: lastEntry.writer,
		},
		Update: bson.M{"$set": bson.M{
			fieldLeaseExpiry: toInt64(expiry),
			fieldLeaseWriter: client.config.Id,
		}},
	}

	// We always write a clock-update operation *before* writing lease info.
	writeClockOp := client.writeClockOp(now)
	ops := []txn.Op{writeClockOp, extendLeaseOp}
	return ops, nextEntry, nil
}

// expireLeaseOps returns the []txn.Op necessary to vacate the lease. If the
// expiration would conflict with cached state, it will return ErrInvalid.
func (client *client) expireLeaseOps(name string) ([]txn.Op, error) {

	// We can't expire a lease that doesn't exist.
	lastEntry, found := client.entries[name]
	if !found {
		return nil, ErrInvalid
	}

	// We also can't expire a lease whose expiry time may be in the future.
	skew := client.skews[lastEntry.writer]
	latestExpiry := skew.Latest(lastEntry.expiry)
	now := client.config.Clock.Now()
	if !now.After(latestExpiry) {
		client.logger.Tracef("lease %q expires in the future", name)
		return nil, ErrInvalid
	}

	// The database change is simple, and depends on the lease doc being
	// untouched since we looked:
	expireLeaseOp := txn.Op{
		C:  client.config.Collection,
		Id: client.leaseDocId(name),
		Assert: bson.M{
			fieldLeaseHolder: lastEntry.holder,
			fieldLeaseExpiry: toInt64(lastEntry.expiry),
			fieldLeaseWriter: lastEntry.writer,
		},
		Remove: true,
	}

	// We always write a clock-update operation *before* writing lease info.
	// Removing a lease document counts as writing lease info.
	writeClockOp := client.writeClockOp(now)
	ops := []txn.Op{writeClockOp, expireLeaseOp}
	return ops, nil
}

// writeClockOp returns a txn.Op which writes the supplied time to the writer's
// field in the skew doc, and aborts if a more recent time has been recorded for
// that writer.
func (client *client) writeClockOp(now time.Time) txn.Op {
	dbNow := toInt64(now)
	dbKey := fmt.Sprintf("%s.%s", fieldClockWriters, client.config.Id)
	return txn.Op{
		C:  client.config.Collection,
		Id: client.clockDocId(),
		Assert: bson.M{
			"$or": []bson.M{{
				dbKey: bson.M{"$lte": dbNow},
			}, {
				dbKey: bson.M{"$exists": false},
			}},
		},
		Update: bson.M{
			"$set": bson.M{dbKey: dbNow},
		},
	}
}

// assertOp returns a txn.Op which will succeed only if holder holds the
// named lease.
func (client *client) assertOp(name, holder string) txn.Op {
	return txn.Op{
		C:  client.config.Collection,
		Id: client.leaseDocId(name),
		Assert: bson.M{
			fieldLeaseHolder: holder,
		},
	}
}

// clockDocId returns the id of the clock document in the client's namespace.
func (client *client) clockDocId() string {
	return clockDocId(client.config.Namespace)
}

// leaseDocId returns the id of the named lease document in the client's
// namespace.
func (client *client) leaseDocId(name string) string {
	return leaseDocId(client.config.Namespace, name)
}

// entry holds the details of a lease and how it was written.
type entry struct {
	// holder identifies the current holder of the lease.
	holder string

	// expiry is the (writer-local) time at which the lease is safe to remove.
	expiry time.Time

	// writer identifies the client that wrote the lease.
	writer string
}

// errNoExtension is used internally to avoid running unnecessary transactions.
var errNoExtension = errors.New("lease needs no extension")
