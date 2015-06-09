package lease

type Collection interface {
	//...
}
type Closer func()
type GetCollection func(string) (Collection, Closer)
type RunTransaction func(jujutxn.TransactionSource) error

func NewManager() Manager {

}

type manager struct {
	tomb    tomb.Tomb
	storage *storage
	claims  chan claim
}

func (m *manager) ClaimLease(namespace, holder string, duration time.Duration) error {
	response := make(chan bool)
	select {
	case <-m.tomb.Dying():
		return worker.ErrStopped
	case m.claims <- claim{namespace, holder, duration, response}:
	}
	select {
	case <-m.tomb.Dying():
		return worker.ErrStopped
	case success := <-response:
		if !success {
			return lease.ErrClaimDenied
		}
	}
	return nil
}

type leaseDoc struct {
	Namespace string `bson:"_id"`
	Holder    string `bson:"holder"`
	Expiry    int64  `bson:"expiry"`
	Writer    string `bson:"writer"`
	Written   int64  `bson:"written"`
}

// entry holds the details of the current lease for a namespace.
type entry struct {
	holder  string
	expiry  time.Time
	writer  string
	written time.Time
}

// skew holds information about a writer's idea of the current time.
type skew struct {
	lastWrite  time.Time
	readAfter  time.Time
	readBefore time.Time
}

// Earliest returns the earliest local time at which the skew might consider
// the supplied time to have passed.
func (skew skew) Earliest(t time.Time) time.Time {
	minDelta := t.Sub(skew.readBefore)
	return skew.lastWrite.Add(minDelta)
}

// Latest returns the latest local time at which the skew might consider the
// supplied time to have passed.
func (skew skew) Latest(t time.Time) time.Time {
	maxDelta := t.Sub(skew.readAfter)
	return skew.lastWrite.Add(maxDelta)
}

type storage struct {
	writer     string
	prefix     string
	collection string
	mongo      Mongo
	cache      map[string]entry
	skews      map[string]skew
}

func (s *storage) claimLease(namespace, holder string, duration time.Duration) error {

	buildTransaction := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// One of our assumptions last time round turned out to be false.
			// Doesn't matter which one, really; just grab fresh state and retry.
			if err := s.refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if _, found := s.cache[namespace]; found {
			return errLeaseHeld
		}
	}
}

func (s *storage) extendLease(namespace, holder string, duration time.Duration) error {

	buildTransaction := func(attempt int) ([]txn.Op, error) {

		// First time round, assume local state is good enough.
		if attempt > 0 {
			if err := s.refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// We can only extend a lease that already exists.
		entry, err := s.getEntry(namespace)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if holder != entry.holder {
			return nil, errLeaseHeld
		}

		// According to the local clock, we want the lease to extend until
		// <duration> in the future.
		proposedExpiry := time.Now().Add(duration)

		// We don't know what time the original writer thinks it is, but we
		// can figure out the earliest and latest local times at which it
		// could expect its lease to expire.
		skew, err := s.getSkew(entry.writer)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if proposedExpiry.Before(skew.Earliest(entry.expiry)) {
			// The "extended" lease would expire before the existing lease. Done.
			return nil, jujutxn.ErrNoOperations
		}
		if proposedExpiry.Before(skew.Latest(entry.expiry)) {
			// The lease might be long enough, but we're not sure, so we have to write a
			// new lease; but we must be sure that the new lease does not expire until
			// after
			proposedExpiry = latestExpiry
		}
		switch {
		case proposedExpiry.Before(earliestExpiry):
		case proposedExpiry.After(latestExpiry):
			// We unquestionably want to write a new lease.
		default:
		}

		// Ensure that we're writing to the same entry that we read, and update the expiry
		// and information necessary for others to determine our own relative clock skew.
		now := time.Now().Unix()
		return []txn.Op{{
			C:  s.collection,
			Id: s.id(namespace),
			Assert: bson.M{
				"holder": entry.holder,
				"expiry": entry.expiry,
			},
			Update: bson.M{
				"expiry":  proposedExpiry,
				"writer":  s.writer,
				"written": now,
			},
		}, {
			C:  s.collection,
			Id: s.prefix,
			Assert: bson.M{
				s.writer: bson.M{"$lt": now},
			},
			Update: bson.M{
				"$set": bson.M{s.writer: now},
			},
		}}, nil
	}
}

func (s *storage) expireLease(namespace, holder string, expiry time.Time) error {

}
