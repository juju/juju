// Package entitycache provides a cache of charmstore entities and
// base-entities, designed to be used for individual charmstore API
// requests.
package entitycache

import (
	"sync"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2"

	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
)

// TODO it might be better to represent the field selection with
// a uint64 bitmask instead of a map[string]int.

// Store holds the underlying storage used by the entity cache.
// It is implemented by *charmstore.Store.
type Store interface {
	FindBestEntity(url *charm.URL, fields map[string]int) (*mongodoc.Entity, error)
	FindBaseEntity(url *charm.URL, fields map[string]int) (*mongodoc.BaseEntity, error)
}

const (
	// entityThreshold holds the maximum number
	// of entities that will be batched up before
	// requesting their base entities.
	entityThreshold = 100

	// baseEntityThreshold holds the maximum number
	// of base entities that will be batched up before
	// requesting them.
	baseEntityThreshold = 20
)

// Cache holds a cache of entities and base entities. Whenever an entity
// is fetched, its base entity is fetched too. It is OK to call methods
// on Cache concurrently.
type Cache struct {
	// store holds the store used by the cache.
	store Store

	// wg represents the set of running goroutines.
	wg sync.WaitGroup

	// entities holds all the cached *mongodoc.Entity entries,
	// A given entity always has an entry with its canonical URL as key,
	// but also may have other entries for other unambiguous names.
	//
	// Note that if an entity is in the entities stash, it does
	// not imply that its base entity necessarily in the base entities
	// stash.
	entities stash

	// entities holds all the cached *mongodoc.BaseEntity entries,
	// keyed by the canonical base URL string, and also its
	// promulgated URL.
	baseEntities stash
}

var requiredEntityFields = map[string]int{
	"_id":             1,
	"promulgated-url": 1,
	"baseurl":         1,
}

var requiredBaseEntityFields = map[string]int{
	"_id": 1,
}

// New returns a new cache that uses the given store
// for fetching entities.
func New(store Store) *Cache {
	var c Cache
	c.entities.init(c.getEntity, &c.wg, requiredEntityFields)
	c.baseEntities.init(c.getBaseEntity, &c.wg, requiredBaseEntityFields)
	c.store = store
	return &c
}

// Close closes the cache, ensuring that there are
// no currently outstanding goroutines in progress.
func (c *Cache) Close() {
	c.wg.Wait()
}

// AddEntityFields arranges that any entity subsequently
// returned from Entity will have the given fields populated.
//
// If all the required fields are added before retrieving any entities,
// fewer database round trips will be required.
func (c *Cache) AddEntityFields(fields map[string]int) {
	c.entities.mu.Lock()
	defer c.entities.mu.Unlock()
	c.entities.addFields(fields)
}

// AddBaseEntityFields arranges that any value subsequently
// returned from BaseEntity will have the given fields populated.
//
// If all the required fields are added before retrieving any base entities,
// less database round trips will be required.
func (c *Cache) AddBaseEntityFields(fields map[string]int) {
	c.baseEntities.mu.Lock()
	defer c.baseEntities.mu.Unlock()
	c.baseEntities.addFields(fields)
}

// StartFetch starts to fetch entities for all the given ids. The
// entities can be accessed by calling Entity and their associated base
// entities found by calling BaseEntity.
// This method does not wait for the entities to actually be fetched.
func (c *Cache) StartFetch(ids []*charm.URL) {
	c.entities.mu.Lock()
	for _, id := range ids {
		c.entities.startFetch(id)
	}
	c.entities.mu.Unlock()

	// Start any base entity fetches that we can.
	c.baseEntities.mu.Lock()
	defer c.baseEntities.mu.Unlock()
	for _, id := range ids {
		if id.User != "" {
			c.baseEntities.startFetch(mongodoc.BaseURL(id))
		}
	}
}

// Entity returns the entity with the given id. If the entity is not
// found, it returns an error with a params.ErrNotFound cause.
// The returned entity will have at least the given fields filled out.
func (c *Cache) Entity(id *charm.URL, fields map[string]int) (*mongodoc.Entity, error) {
	// Start the base entity fetch asynchronously if we have
	// an id we can infer the base entity URL from.
	if id.User != "" {
		c.baseEntities.mu.Lock()
		c.baseEntities.startFetch(mongodoc.BaseURL(id))
		c.baseEntities.mu.Unlock()
	}
	e, err := c.entities.entity(id, fields)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return e.(entity).Entity, nil
}

// BaseEntity returns the base entity with the given id. If the entity is not
// found, it returns an error with a params.ErrNotFound cause.
// The returned entity will have at least the given fields filled out.
func (c *Cache) BaseEntity(id *charm.URL, fields map[string]int) (*mongodoc.BaseEntity, error) {
	if id.User == "" {
		return nil, errgo.Newf("cannot get base entity of URL %q with no user", id)
	}
	e, err := c.baseEntities.entity(mongodoc.BaseURL(id), fields)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return e.(baseEntity).BaseEntity, nil
}

// getEntity is used by c.entities to fetch entities.
// Called with no locks held.
func (c *Cache) getEntity(id *charm.URL, fields map[string]int) (stashEntity, error) {
	e, err := c.store.FindBestEntity(id, fields)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	if id.User == "" {
		// The id we used to look up the entity had no user
		// so we were not able to start the base entity fetching
		// concurrently, so start fetching it now, at the soonest
		// possible moment.
		c.baseEntities.mu.Lock()
		c.baseEntities.startFetch(mongodoc.BaseURL(e.URL))
		c.baseEntities.mu.Unlock()
	}
	return entity{e}, nil
}

// getBaseEntity is used by c.baseEntities to fetch entities.
// Called with no locks held.
func (c *Cache) getBaseEntity(id *charm.URL, fields map[string]int) (stashEntity, error) {
	e, err := c.store.FindBaseEntity(id, fields)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return baseEntity{e}, nil
}

// stash holds a set of one kind of entity (either entities or base entities).
type stash struct {
	// get fetches the entity with the given URL.
	get func(id *charm.URL, fields map[string]int) (stashEntity, error)

	// wg represents the set of running goroutines.
	wg *sync.WaitGroup

	// mu guards the fields below
	mu sync.Mutex

	// changed is signalled every time the entities map has changed.
	// This means that each waiter can potentially be woken up many
	// times before it finds the entity that it's waiting for, but
	// saves us having a channel or condition per entity.
	//
	// Note that in the usual pattern we expect to see, callers
	// will ask for entities in the same order that they arrive
	// in the cache, so won't iterate many times.
	changed sync.Cond

	// entities holds at least one entry for each cached entity,
	// keyed by the entity id string. A given entity always has an
	// entry with its canonical URL as key, but also may have other
	// entries for other unambiguous names.
	//
	// A nil entry indicates that the entity has been scheduled to
	// be fetched. Entries that have been fetched but that were not
	// found are indicated with a notFoundEntity value.
	entities map[charm.URL]stashEntity

	// fields holds the set of fields required when fetching an
	// entity. This map is never changed after it is first populated
	// - it is replaced instead, which means that it's OK to pass it
	// to concurrent goroutines that access it without the mutex
	// locked.
	//
	// When it does change, the entity cache is invalidated. Fields
	// are never deleted.
	fields map[string]int

	// version is incremented every time fields is modified.
	version int

	// err holds any database fetch error (other than "not found")
	// that has occurred while fetching entities.
	err error
}

// init initializes the stash with the given entity get function.
func (s *stash) init(get func(id *charm.URL, fields map[string]int) (stashEntity, error), wg *sync.WaitGroup, initialFields map[string]int) {
	s.changed.L = &s.mu
	s.get = get
	s.wg = wg
	s.fields = initialFields
	s.entities = make(map[charm.URL]stashEntity)
}

// entity returns the entity with the given id. If the entity is not
// found, it returns an error with a params.ErrNotFound cause.
func (s *stash) entity(id *charm.URL, fields map[string]int) (stashEntity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addFields(fields)
	e, hasEntry := s.entities[*id]
	for {
		if e != nil {
			if e, ok := e.(*notFoundEntity); ok {
				return nil, errgo.Mask(e.err, errgo.Is(params.ErrNotFound))
			}
			return e, nil
		}
		if s.err != nil {
			return nil, errgo.Notef(s.err, "cannot fetch %q", id)
		}
		if hasEntry {
			// The entity is already being fetched. Wait for the fetch
			// to complete and try again.
			s.changed.Wait()
			e, hasEntry = s.entities[*id]
			continue
		}
		// Fetch synchronously (any other goroutines will be
		// notified when we've retrieved the entity). After the
		// fetch has completed, the entry in the cache will either
		// be set to the retrieved entity, or deleted (if the
		// selected fields have changed).
		s.entities[*id] = nil
		version := s.version
		fields := s.fields
		s.mu.Unlock()
		e = s.fetch(id, fields, version)
		s.mu.Lock()
		// Invariant (from fetch): e != nil || s.err != nil
	}
}

// addFields adds the given fields to those that will be fetched
// when an entity is fetched.
//
// Called with s.mu locked.
func (s *stash) addFields(fields map[string]int) {
	changed := false
	for field := range fields {
		if _, ok := s.fields[field]; !ok {
			changed = true
			break
		}
	}
	if !changed {
		return
	}
	if len(s.entities) > 0 {
		// The fields have changed, invalidating our current
		// cache, so delete all entries.
		s.entities = make(map[charm.URL]stashEntity)
		s.version++
		// There may be several goroutines waiting for pending
		// entities. Notify them so that they can start a new
		// fetch.
		s.changed.Broadcast()
	}
	newFields := make(map[string]int)
	for field := range s.fields {
		newFields[field] = 1
	}
	for field := range fields {
		newFields[field] = 1
	}
	s.fields = newFields
}

// startFetch starts an asynchronous fetch for the given id.
// If a fetch is already in progress, it does nothing.
//
// Called with s.mu locked.
func (s *stash) startFetch(id *charm.URL) {
	if _, ok := s.entities[*id]; ok {
		return
	}
	s.entities[*id] = nil
	// Note that it's only OK to pass s.fields here because
	// it's never mutated, only replaced.
	s.wg.Add(1)
	go s.fetchAsync(id, s.fields, s.version)
}

// fetchAsync is like fetch except that it is expected to be called
// in a separate goroutine, with s.wg.Add called appropriately
// beforehand.
// Called with s.mu unlocked.
func (s *stash) fetchAsync(url *charm.URL, fields map[string]int, version int) stashEntity {
	defer s.wg.Done()
	return s.fetch(url, fields, version)
}

// fetch fetches the entity with the given id, including the given
// fields, adds it to the stash and notifies any waiters that the stash
// has changed.
//
// The given entity version holds the version at the time the
// fetch was started. If the entity version has changed when the result is received,
// the result is discarded.
//
// fetch returns the entity as it would be stored in the cache (notFoundEntity
// implies not found). It returns nil if and only if some other kind of error has
// been encountered (in this case the error will be stored in s.err).
//
// Called with no locks held.
func (s *stash) fetch(url *charm.URL, fields map[string]int, version int) stashEntity {
	e, err := s.get(url, fields)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		if errgo.Cause(err) != params.ErrNotFound {
			if s.err == nil {
				// Only set the error if we haven't encountered one already.
				// We assume that if we're getting several errors, they're
				// almost certainly caused by the same thing, so there's
				// no point in logging them all.
				s.err = errgo.Mask(err)

				// Let other waiters know about the fact that
				// we got an error.
				s.changed.Broadcast()
			}
			return nil
		}
		e = &notFoundEntity{err}
	}
	if s.version != version {
		// The entity version has changed, implying the selected
		// fields have changed, so the entity we've just fetched
		// is not valid to put in the cache because we haven't
		// fetched all the fields that are required.
		//
		// We return the entity that we've just fetched (our
		// caller, at least, wanted the fields we've just got).
		// There's no need to delete the "pending" entry from the
		// cache because all entries will have been cleared out
		// when the version changed.
		return e
	}
	return s.addEntity(e, url)
}

// addEntity adds the given entity to the stash, adds the given lookupId
// as an alias for it, and notifies any listeners if there has been a
// change.
//
// It returns the cached entity - this may be different from e if
// an entry is already present in the cache.
//
// Called with s.mu locked.
func (s *stash) addEntity(e stashEntity, lookupId *charm.URL) stashEntity {
	keys := make([]*charm.URL, 0, 3)
	if _, ok := e.(*notFoundEntity); ok {
		keys = append(keys, lookupId)
	} else {
		keys = append(keys, e.url())
		if u := e.promulgatedURL(); u != nil {
			keys = append(keys, u)
		}
		if lookupId != nil {
			keys = append(keys, lookupId)
		}
	}
	added := false
	for _, key := range keys {
		if old := s.entities[*key]; old == nil {
			s.entities[*key] = e
			added = true
		} else {
			// We've found an old entry - use that instead
			// of the new one if necessary.
			e = old
		}
	}
	if added {
		s.changed.Broadcast()
	}
	return e
}

// notFoundEntity is a sentinel type that is stored
// in the entities map when the value has been fetched
// but was not found.
type notFoundEntity struct {
	// The actual not-found error encountered.
	err error
}

func (*notFoundEntity) url() *charm.URL {
	panic("url called on not-found sentinel value")
}

func (*notFoundEntity) promulgatedURL() *charm.URL {
	panic("promulgatedURL called on not-found sentinel value")
}

// Iter returns an iterator that iterates through
// all the entities found by the given query, which must
// be a query on the entities collection.
// The entities produced by the returned iterator
// will have at least the given fields populated.
func (c *Cache) Iter(q *mgo.Query, fields map[string]int) *Iter {
	return c.CustomIter(mgoQuery{q}, fields)
}

// CustomIter is the same as Iter except that it allows iteration
// through entities that aren't necessarily the direct result of
// a MongoDB query. Care must be taken to ensure that
// the fields returned are valid for the entities they purport
// to represent.
func (c *Cache) CustomIter(q StoreQuery, fields map[string]int) *Iter {
	c.entities.mu.Lock()
	defer c.entities.mu.Unlock()
	c.entities.addFields(fields)
	iter := &Iter{
		iter:    q.Iter(c.entities.fields),
		cache:   c,
		entityc: make(chan *mongodoc.Entity),
		closed:  make(chan struct{}),
		version: c.entities.version,
	}
	iter.runWG.Add(1)
	go iter.run()
	return iter
}

// Iter holds an iterator over a set of entities.
type Iter struct {
	// e holds the current entity. It is nil only
	// if the iterator has terminated.
	e       *mongodoc.Entity
	iter    StoreIter
	cache   *Cache
	entityc chan *mongodoc.Entity
	closed  chan struct{}
	runWG   sync.WaitGroup

	// err holds any error encountered when iterating.
	// It is set only after Next has returned false.
	err error

	// The following fields are owned by Iter.run.

	// entityBatch holds the entities that we have read
	// from the underlying iterator but haven't yet
	// sent on iter.entityc.
	entityBatch []*mongodoc.Entity

	// baseEntityBatch holds the set of base entities that
	// are required by the entities in entityBatch.
	baseEntityBatch []*charm.URL

	// version holds cache.entities.version at the time the iterator
	// was created. If cache.entities.version changes during
	// iteration, we will still deliver entities to the iterator,
	// but we cannot store them in the stash because they won't have
	// the required fields.
	version int
}

// Next reports whether there are any more entities available from the
// iterator. The iterator is automatically closed when Next returns
// false.
func (i *Iter) Next() bool {
	i.e = <-i.entityc
	if i.e != nil {
		return true
	}
	if err := i.iter.Err(); err != nil {
		i.err = errgo.Mask(err)
	}
	return false
}

// Entity returns the current entity, or nil if the iterator has reached
// the end of its iteration. The base entity associated with the entity
// will be available via the EntityFetcher.BaseEntity method.
// The caller should treat the returned entity as read-only.
func (i *Iter) Entity() *mongodoc.Entity {
	return i.e
}

// Close closes the iterator. This must be called if the iterator is
// abandoned without reaching its end.
func (i *Iter) Close() {
	close(i.closed)
	// Wait for the iterator goroutine to complete. Note that we
	// *could* just wait for i.entityc to be closed, but this would
	// mean that it would be possible for i.send to complete
	// successfully even when the iterator has been closed, which
	// compromises test reproducibility. An alternative to the wait
	// group might be for iter.send to do a non-blocking receive on
	// i.closed before trying to send on i.entityc.
	i.runWG.Wait()
	i.e = nil
	if err := i.iter.Err(); err != nil {
		i.err = errgo.Mask(err)
	}
}

// Err returns any error encountered by the the iterator. If the
// iterator has not terminated or been closed, it will always
// return nil.
func (iter *Iter) Err() error {
	return iter.err
}

// run iterates through the underlying iterator, sending
// entities on iter.entityc, first ensuring that their respective base
// entities have also been fetched.
func (iter *Iter) run() {
	defer iter.runWG.Done()
	defer close(iter.entityc)
	defer iter.iter.Close()
	for {
		var e mongodoc.Entity
		if !iter.iter.Next(&e) {
			break
		}
		iter.addEntity(entity{&e})
		if len(iter.baseEntityBatch) >= baseEntityThreshold || len(iter.entityBatch) >= entityThreshold {
			// We've reached one of the thresholds - send the batch.
			if !iter.sendBatch() {
				return
			}
		}
	}
	iter.sendBatch()
}

// addEntity adds an entity that has been received
// from the underlying iterator.
//
// Called from iter.run without any locks held.
func (iter *Iter) addEntity(e entity) {
	iter.entityBatch = append(iter.entityBatch, e.Entity)
	entities := &iter.cache.entities
	entities.mu.Lock()
	defer entities.mu.Unlock()
	if _, ok := entities.entities[*e.url()]; ok {
		// The entity has already been fetched, or is being fetched.
		// This also implies that its base entity has already been added (or
		// is in the process of being added) to the cache.
		return
	}
	if entities.version == iter.version {
		// The entity we have here is valid to put into the cache, so do that.
		// Note: we know from the check above that the entity is not
		// already present in the cache.
		entities.addEntity(e, nil)
	}

	baseEntities := &iter.cache.baseEntities
	baseEntities.mu.Lock()
	defer baseEntities.mu.Unlock()

	baseURL := mongodoc.BaseURL(e.URL)
	if _, ok := baseEntities.entities[*baseURL]; !ok {
		// We need to fetch the base entity, so add it to our
		// batch and signal that it will be fetched by adding it
		// to the map. Note: this assumes that the client doing
		// the iteration will make progress - it could delay
		// other base entity reads arbitrarily by not calling
		// Next. This should not be a problem in practice.
		iter.baseEntityBatch = append(iter.baseEntityBatch, baseURL)
		baseEntities.entities[*baseURL] = nil
	}
	return
}

// sendBatch obtains all the batched base entities and sends all the
// batched entities on iter.entityc. If it encounters an error, or the
// iterator is closed, it sets iter.err and returns false.
//
// Called from iter.run with no locks held.
func (iter *Iter) sendBatch() bool {
	// Start a fetch for all base entities.
	// TODO use actual batch fetch with $in etc rather
	// than starting a goroutine for each base entity.
	baseEntities := &iter.cache.baseEntities
	baseEntities.mu.Lock()
	iter.cache.wg.Add(len(iter.baseEntityBatch))
	for _, id := range iter.baseEntityBatch {
		go baseEntities.fetchAsync(id, baseEntities.fields, baseEntities.version)
	}
	baseEntities.mu.Unlock()
	iter.baseEntityBatch = iter.baseEntityBatch[:0]

	for _, e := range iter.entityBatch {
		if !iter.send(e) {
			return false
		}
	}
	iter.entityBatch = iter.entityBatch[:0]
	return true
}

// send sends the given entity on iter.entityc.
// It reports whether that entity was sent OK (that is,
// the iterator has not been closed).
func (iter *Iter) send(e *mongodoc.Entity) bool {
	select {
	case iter.entityc <- e:
		return true
	case <-iter.closed:
		return false
	}
}

// stashEntity represents an entity stored in a stash.
// It is implemented by the entity and baseEntity types.
type stashEntity interface {
	url() *charm.URL
	promulgatedURL() *charm.URL
}

type entity struct {
	*mongodoc.Entity
}

func (e entity) url() *charm.URL {
	u := *e.URL
	return &u
}

func (e entity) promulgatedURL() *charm.URL {
	if e.PromulgatedURL == nil {
		return nil
	}
	u := *e.PromulgatedURL
	return &u
}

type baseEntity struct {
	*mongodoc.BaseEntity
}

func (e baseEntity) url() *charm.URL {
	return e.URL
}

func (e baseEntity) promulgatedURL() *charm.URL {
	return nil
}

// StoreQuery represents a query on entities in the charm store It is
// represented as an interface rather than using *mgo.Query directly so
// that we can easily fake it in tests, and so that it's possible to use
// other different underlying representations.
type StoreQuery interface {
	// Iter returns an iterator over the query, selecting
	// at least the fields mentioned in the given map.
	Iter(fields map[string]int) StoreIter
}

// StoreIter represents an iterator over entities in the charm store.
type StoreIter interface {
	Next(interface{}) bool
	Err() error
	Close() error
}

type mgoQuery struct {
	query *mgo.Query
}

func (q mgoQuery) Iter(fields map[string]int) StoreIter {
	return q.query.Select(fields).Iter()
}
