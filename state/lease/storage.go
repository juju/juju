package lease

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"
)

// Info is the information a Storage is willing to give out about leases.
type Info struct {
	// Holder is the name of the current lease holder.
	Holder string

	// EarliestExpiry is the earliest time at which it's possible the lease
	// might expire.
	EarliestExpiry time.Time

	// LatestExpiry is the latest time at which it's possible the lease might
	// still be valid.
	LatestExpiry time.Time

	// AssertOp is filthy abstraction-breaking garbage that is necessary to
	// allow us to make mgo/txn assertions about leases in the state package;
	// and which thus allows us to gate certain state changes on a particular
	// unit's leadership of a service, for example.
	AssertOp txn.Op
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

var ErrLeaseFree = errors.New("lease not held")
var ErrLeaseHeld = errors.New("lease already held")

var errNoExtension = errors.New("lease needs no extension")

// storage exposes lease management functionality on top of mongodb.
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
	entries, err := s.readEntries()
	if err != nil {
		return errors.Trace(err)
	}
	skews, err := s.readSkews()
	if err != nil {
		return errors.Trace(err)
	}

	// Check we're not missing any required clock information before
	// updating our local state.
	for lease, entry := range entries {
		if entry.writer != s.config.Instance {
			if _, found := skews[entry.writer]; !found {
				return errors.Errorf("lease %q invalid: no clock data for %s", lease, entry.writer)
			}
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
	if err != nil {
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
		switch errors.Cause(err) {
		case nil:
			cacheEntry = nextEntry
			return ops, nil
		case errNoExtension:
			cacheEntry = lastEntry
			return nil, jujutxn.ErrNoOperations
		}
		return nil, errors.Trace(err)
	})
	if err != nil {
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
	if err != nil {
		return errors.Trace(err)
	}

	// Uncache this lease entry.
	delete(s.entries, lease)
	return nil
}

// claimLeaseOps returns the []txn.Op necessary to claim the supplied lease
// until duration in the future, and a cache entry corresponding to the values
// that will be written if the transaction succeeds.
func (s *storage) claimLeaseOps(lease, holder string, duration time.Duration) ([]txn.Op, entry, error) {

	// We can't claim a lease that's already held.
	if _, found := s.entries[lease]; found {
		return nil, entry{}, ErrLeaseHeld
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
	leaseDoc, err := s.leaseDoc(lease, entry)
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
	return []txn.Op{writeClockOp, extendLeaseOp}, nextEntry, nil
}

// extendLeaseOps returns the []txn.Op necessary to extend the supplied lease
// until duration in the future, and a cache entry corresponding to the values
// that will be written if the transaction succeeds. If the supplied lease
// already extends far enough that no operations are required, it will return
// errNoExtension.
func (s *storage) extendLeaseOps(lease, holder string, duration time.Duration) ([]txn.Op, entry, error) {

	// Reject extensions when there's no lease, or the holder doesn't match.
	lastEntry, found := s.entries[lease]
	if !found {
		return nil, ErrLeaseFree
	}
	if holder != lastEntry.holder {
		return nil, ErrLeaseHeld
	}

	// According to the local clock, we want the lease to extend until
	// <duration> in the future.
	now := s.config.Clock.Now()
	expiry := now.Add(duration)

	// We don't know what time the original writer thinks it is, but we
	// can figure out the earliest and latest local times at which it
	// could be expecting its lease to expire.
	skew := s.skews[lastEntry.writer]
	if expiry.Before(skew.Earliest(lastEntry.expiry)) {
		// The "extended" lease will certainly expire before the
		// existing lease would. Done.
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
			"holder": lastEntry.holder,
			"expiry": lastEntry.expiry.UnixNano(),
		},
		Update: bson.M{
			"expiry":  expiry.UnixNano(),
			"writer":  s.config.Instance,
			"written": nextEntry.written.UnixNano(),
		},
	}

	// We always write a clock-update operation *before* writing lease info.
	writeClockOp := s.writeClockOp(now)
	return []txn.Op{writeClockOp, extendLeaseOp}, nextEntry, nil
}

func (s *storage) expireLeaseOps(lease string) ([]txn.Op, error) {

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

func (s *storage) assertOp(lease, holder string) txn.Op {
	return txn.Op{
		C:      s.config.Collection,
		Id:     s.leaseDocId(lease),
		Assert: bson.M{"holder": holder},
	}
}

// clockDocId returns the id of the clock document in the storage's namespace.
func (s *storage) clockDocId() string {
	return fmt.Sprintf("clock#%s#")
}

// leaseDocId returns the id of the named lease document in the storage's
// namespace.
func (s *storage) leaseDocId(lease string) string {
	return fmt.Sprintf("lease#%s#%s#", s.config.Namespace, lease)
}

// leaseDoc returns a valid lease document encoding the supplied lease and
// holder in the storage's namespace, or an error.
func (s *storage) leaseDoc(lease string, entry entry) (*leaseDoc, error) {
	leaseDoc := &leaseDoc{
		Id:        s.leaseDocId(lease),
		Namespace: s.config.Namespace,
		Lease:     lease,
		Holder:    entry.holder,
		Expiry:    entry.expiry.UnixNano(),
		Writer:    entry.writer,
		Written:   entry.written.UnixNano(),
	}
	if err := leaseDoc.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return leaseDoc, nil
}

// leaseDoc is used to serialise lease entries.
type leaseDoc struct {
	// Id is always "lease#<namespace>#<lease>", so that we can extract useful
	// information from a stream of watcher events without extra DB hits.
	// Apart from checking validity on load, though, there's little reason
	// to use Id elsewhere; Namespace and Lease are the sources of truth.
	Id        string `bson:"_id"`
	Namespace string `bson:"namespace"` // TODO(fwereade) definitely add index
	Lease     string `bson:"lease"`     // TODO(fwereade) maybe add index

	// Holder, Expiry, Writer and Written map directly to entry. The time values
	// are stored as UnixNano; not so much because we *need* the precision, as
	// because it's yucky when serialisation throws precision away, and life is
	// easier when we can compare leases exactly.
	Holder  string `bson:"holder"`
	Expiry  int64  `bson:"expiry"`
	Writer  string `bson:"writer"`
	Written int64  `bson:"written"`
}

// Validate returns an error if the document appears to be invalid.
func (doc leaseDoc) Validate() error {
	return fmt.Errorf("validation!")
}
