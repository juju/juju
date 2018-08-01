// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5/internal/charmstore"

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/parallel"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/mgostorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/natefinch/lumberjack.v2"

	"gopkg.in/juju/charmstore.v5/audit"
	"gopkg.in/juju/charmstore.v5/internal/blobstore"
	"gopkg.in/juju/charmstore.v5/internal/cache"
	"gopkg.in/juju/charmstore.v5/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5/internal/monitoring"
	"gopkg.in/juju/charmstore.v5/internal/router"
)

var logger = loggo.GetLogger("charmstore.internal.charmstore")

var (
	errClosed          = errgo.New("charm store has been closed")
	ErrTooManySessions = errgo.New("too many mongo sessions in use")
)

// Pool holds a connection to the underlying charm and blob
// data stores. Calling its Store method returns a new Store
// from the pool that can be used to process short-lived requests
// to access and modify the store.
type Pool struct {
	db     StoreDatabase
	es     *SearchIndex
	bakery *bakery.Service
	stats  stats
	run    *parallel.Run

	// statsCache holds a cache of AggregatedCounts
	// values, keyed by entity id. When the id has no
	// revision, the counts apply to all revisions of the
	// entity.
	statsCache *cache.Cache

	config ServerParams

	// auditEncoder encodes messages to auditLogger.
	auditEncoder *json.Encoder
	auditLogger  *lumberjack.Logger

	// reqStoreC is a buffered channel that contains allocated
	// stores that are not currently in use.
	reqStoreC chan *Store

	// mu guards the fields following it.
	mu sync.Mutex

	// storeCount holds the number of stores currently allocated.
	storeCount int

	// closed holds whether the handler has been closed.
	closed bool

	// rootKeys holds the cache of macaroon root keys.
	rootKeys *mgostorage.RootKeys
}

// reqStoreCacheSize holds the maximum number of store
// instances to keep around cached when there is no
// limit specified by config.MaxMgoSessions.
const reqStoreCacheSize = 50

// maxAsyncGoroutines holds the maximum number
// of goroutines that will be started by Store.Go.
const maxAsyncGoroutines = 50

// NewPool returns a Pool that uses the given database
// and search index. If bakeryParams is not nil,
// the Bakery field in the resulting Store will be set
// to a new Service that stores macaroons in mongo.
//
// The pool must be closed (with the Close method)
// after use.
func NewPool(db *mgo.Database, si *SearchIndex, bakeryParams *bakery.NewServiceParams, config ServerParams) (*Pool, error) {
	if config.StatsCacheMaxAge == 0 {
		config.StatsCacheMaxAge = time.Hour
	}
	if config.NewBlobBackend == nil {
		config.NewBlobBackend = func(db *mgo.Database) blobstore.Backend {
			return blobstore.NewMongoBackend(db, "entitystore")
		}
	}

	p := &Pool{
		db:          StoreDatabase{db}.copy(),
		es:          si,
		statsCache:  cache.New(config.StatsCacheMaxAge),
		config:      config,
		run:         parallel.NewRun(maxAsyncGoroutines),
		auditLogger: config.AuditLogger,
		rootKeys:    mgostorage.NewRootKeys(100),
	}
	if config.MaxMgoSessions > 0 {
		p.reqStoreC = make(chan *Store, config.MaxMgoSessions)
	} else {
		p.reqStoreC = make(chan *Store, reqStoreCacheSize)
	}
	if bakeryParams != nil {
		bakerySvc, err := bakery.NewService(*bakeryParams)
		if err != nil {
			return nil, errgo.Notef(err, "cannot create bakery service")
		}
		p.bakery = bakerySvc
	}

	if config.AuditLogger != nil {
		p.auditLogger = config.AuditLogger
		p.auditEncoder = json.NewEncoder(p.auditLogger)
	}

	store := p.Store()
	defer store.Close()
	if err := store.ensureIndexes(); err != nil {
		return nil, errgo.Notef(err, "cannot ensure indexes")
	}
	if err := store.ES.ensureIndexes(false); err != nil {
		return nil, errgo.Notef(err, "cannot ensure elasticsearch indexes")
	}
	return p, nil
}

// Close closes the pool. This must be called when the pool
// is finished with.
func (p *Pool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()
	p.run.Wait()
	p.db.Close()
	// Close all cached stores. Any used by
	// outstanding requests will be closed when the
	// requests complete.
loop:
	for {
		select {
		case s := <-p.reqStoreC:
			s.DB.Close()
		default:
			break loop
		}
	}
	if p.auditLogger != nil {
		p.auditLogger.Close()
	}
}

// RequestStore returns a store for a client request. It returns
// an error with a ErrTooManySessions cause
// if too many mongo sessions are in use.
func (p *Pool) RequestStore() (*Store, error) {
	store, err := p.requestStoreNB(false)
	if store != nil {
		return store, nil
	}
	if errgo.Cause(err) != ErrTooManySessions {
		return nil, errgo.Mask(err)
	}
	// No handlers currently available - we've exceeded our concurrency limit
	// so wait for a handler to become available.
	select {
	case store := <-p.reqStoreC:
		return store, nil
	case <-time.After(p.config.HTTPRequestWaitDuration):
		return nil, errgo.Mask(err, errgo.Is(ErrTooManySessions))
	}
}

func (p *Pool) newBlobStore(db StoreDatabase) *blobstore.Store {
	backend := p.config.NewBlobBackend(db.Database)
	bs := blobstore.New(db.Database, "entitystore", backend)
	if p.config.MinUploadPartSize != 0 {
		bs.MinPartSize = p.config.MinUploadPartSize
	}
	if p.config.MaxUploadPartSize != 0 {
		bs.MaxPartSize = p.config.MaxUploadPartSize
	}
	if p.config.MaxUploadParts != 0 {
		bs.MaxParts = p.config.MaxUploadParts
	}
	return bs
}

// Store returns a Store that can be used to access the database.
//
// It must be closed (with the Close method) after use.
func (p *Pool) Store() *Store {
	store, _ := p.requestStoreNB(true)
	return store
}

// requestStoreNB is like RequestStore except that it
// does not block when a Store is not immediately
// available, in which case it returns an error with
// a ErrTooManySessions cause.
//
// If always is true, it will never return an error.
func (p *Pool) requestStoreNB(always bool) (*Store, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed && !always {
		return nil, errClosed
	}
	select {
	case store := <-p.reqStoreC:
		return store, nil
	default:
	}
	if !always && p.config.MaxMgoSessions > 0 && p.storeCount >= p.config.MaxMgoSessions {
		return nil, ErrTooManySessions
	}
	p.storeCount++
	db := p.db.copy()
	store := &Store{
		DB:        db,
		BlobStore: p.newBlobStore(db),
		ES:        p.es,
		stats:     &p.stats,
		pool:      p,
	}
	store.Bakery = store.BakeryWithPolicy(p.config.RootKeyPolicy)
	return store, nil
}

// BakeryWithPolicy returns a copy of the Store's Bakery with a macaroon
// storage that returns root keys conforming to the given policy.
//
// If there is no configured bakery, it returns nil.
func (s *Store) BakeryWithPolicy(policy mgostorage.Policy) *bakery.Service {
	if s.pool.bakery == nil {
		return nil
	}
	return s.pool.bakery.WithStore(s.pool.rootKeys.NewStorage(s.DB.Macaroons(), policy))
}

// Store holds a connection to the underlying charm and blob
// data stores that is appropriate for short term use.
type Store struct {
	DB        StoreDatabase
	BlobStore *blobstore.Store
	ES        *SearchIndex
	Bakery    *bakery.Service
	stats     *stats
	pool      *Pool
}

// Copy returns a new store with a lifetime
// independent of s. Use this method if you
// need to use a store in an independent goroutine.
//
// It must be closed (with the Close method) after use.
func (s *Store) Copy() *Store {
	s1 := *s
	s1.DB = s.DB.clone()
	s1.BlobStore = s.pool.newBlobStore(s1.DB)
	s1.Bakery = s1.BakeryWithPolicy(s.pool.config.RootKeyPolicy)

	s.pool.mu.Lock()
	s.pool.storeCount++
	s.pool.mu.Unlock()

	return &s1
}

// Close closes the store instance.
func (s *Store) Close() {
	// Refresh the mongodb session so that the
	// next time the Store is used, it will acquire
	// a new connection from the pool as if the
	// session had been copied.
	s.DB.Session.Refresh()

	s.pool.mu.Lock()
	defer s.pool.mu.Unlock()
	if !s.pool.closed && (s.pool.config.MaxMgoSessions == 0 || s.pool.storeCount <= s.pool.config.MaxMgoSessions) {
		// The pool isn't overloaded, so put the store
		// back. Note that the default case should
		// never happen when MaxMgoSessions > 0.
		select {
		case s.pool.reqStoreC <- s:
			return
		default:
			// No space for handler - this may happen when
			// the number of actual sessions has exceeded
			// the requested maximum (for example when
			// a request already at the limit uses another session,
			// or when we are imposing no limit).
		}
	}
	s.DB.Close()
	s.pool.storeCount--
}

// SetReconnectTimeout sets the length of time that
// mongo requests will block waiting to reconnect
// to a disconnected mongo server. If it is zero,
// requests may block forever.
func (s *Store) SetReconnectTimeout(d time.Duration) {
	s.DB.Session.SetSyncTimeout(d)
}

// Go runs the given function in a new goroutine,
// passing it a copy of s, which will be closed
// after the function returns.
func (s *Store) Go(f func(*Store)) {
	s = s.Copy()
	s.pool.run.Do(func() error {
		defer s.Close()
		f(s)
		return nil
	})
}

// Pool returns the pool that the store originally
// came from.
func (s *Store) Pool() *Pool {
	return s.pool
}

func (s *Store) ensureIndexes() error {
	indexes := []struct {
		c *mgo.Collection
		i mgo.Index
	}{{
		s.DB.StatCounters(),
		mgo.Index{Key: []string{"k", "t"}, Unique: true},
	}, {
		s.DB.StatTokens(),
		mgo.Index{Key: []string{"t"}, Unique: true},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"baseurl"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"uploadtime"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"promulgated-url"}, Unique: true, Sparse: true},
	}, {
		s.DB.Logs(),
		mgo.Index{Key: []string{"urls"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"user"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"user", "name"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"user", "name", "series"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"series"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"blobhash256"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"_id", "name"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"charmrequiredinterfaces"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"charmprovidedinterfaces"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"bundlecharms"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"name", "published", "-promulgated-revision", "-supportedseries"}},
	}, {
		s.DB.Entities(),
		mgo.Index{Key: []string{"name", "published", "user", "-revision", "-supportedseries"}},
	}, {
		s.DB.BaseEntities(),
		mgo.Index{Key: []string{"name"}},
	}, {
		s.DB.Resources(),
		mgo.Index{Key: []string{"baseurl", "name"}},
	}, {
		s.DB.Resources(),
		mgo.Index{Key: []string{"baseurl", "name", "revision"}, Unique: true},
	}, {
		// TODO this index should be created by the mgo gridfs code.
		s.DB.C("entitystore.files"),
		mgo.Index{Key: []string{"filename"}},
	}, {
		s.DB.Revisions(),
		mgo.Index{Key: []string{"baseurl"}},
	}}
	for _, idx := range indexes {
		err := idx.c.EnsureIndex(idx.i)
		if err != nil {
			return errgo.Notef(err, "cannot ensure index with keys %v on collection %s", idx.i, idx.c.Name)
		}
	}
	if err := s.pool.rootKeys.EnsureIndex(s.DB.Macaroons()); err != nil {
		return errgo.Notef(err, "cannot ensure root keys index")
	}
	return nil
}

// BlobStoreGC runs the blobstore garbage collector,
// deleting all blobs that have not been referenced since
// the given time.
func (s *Store) BlobStoreGC(before time.Time) error {
	// BEWARE: if this code does not add all the relevant blob
	// hashes, they will be removed by the garbage collector!

	// Get entity counts so we can make an approximate
	// measure of hash count.
	entityCount, err := s.DB.Entities().Count()
	if err != nil {
		return errgo.Mask(err)
	}
	resourceCount, err := s.DB.Resources().Count()
	if err != nil {
		return errgo.Mask(err)
	}
	// Assume non-multipart resources, v5 entities that need conversion,
	// and a 20% duplication rate,
	estimatedRefCount := (entityCount*2 + resourceCount) * 4 / 5

	refs := blobstore.NewRefs(estimatedRefCount)
	iter := s.DB.Entities().Find(nil).Select(FieldSelector(
		"prev5blobextrahash",
		"blobhash",
		"size",
	)).Iter()
	var entity mongodoc.Entity
	for iter.Next(&entity) {
		if entity.PreV5BlobExtraHash != "" {
			refs.Add(entity.PreV5BlobExtraHash)
		}
		refs.Add(entity.BlobHash)
	}
	if err := iter.Err(); err != nil {
		return errgo.Mask(err)
	}
	iter = s.DB.Resources().Find(nil).Select(FieldSelector(
		"blobhash",
		"blobindex",
	)).Iter()
	var resource mongodoc.Resource
	for iter.Next(&resource) {
		if resource.BlobIndex == nil {
			refs.Add(resource.BlobHash)
			continue
		}
		for _, hash := range resource.BlobIndex.Hashes {
			refs.Add(hash)
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Mask(err)
	}
	stats, err := s.BlobStore.GC(refs, before)
	if err != nil {
		return errgo.Notef(err, "blobstore GC failed")
	}
	monitoring.SetBlobStoreStats(stats)
	return nil
}

// AddAudit adds the given entry to the audit log.
func (s *Store) AddAudit(entry audit.Entry) {
	s.addAuditAtTime(entry, time.Now())
}

func (s *Store) addAuditAtTime(entry audit.Entry, t time.Time) {
	if s.pool.auditEncoder == nil {
		return
	}
	entry.Time = t
	err := s.pool.auditEncoder.Encode(entry)
	if err != nil {
		logger.Errorf("Cannot write audit log entry: %v", err)
	}
}

// NewRevision returns a new revision number for the
// given entity URL.
func (s *Store) NewRevision(id *charm.URL) (int, error) {
	id = id.WithRevision(-1)
	col := s.DB.Revisions()
	change := mgo.Change{
		Update:    bson.D{{"$inc", bson.D{{"revision", 1}}}},
		ReturnNew: true,
	}
	var doc mongodoc.LatestRevision
	_, err := col.FindId(id).Apply(change, &doc)
	if err == nil {
		return doc.Revision, nil
	}
	if err != mgo.ErrNotFound {
		return 0, errgo.Notef(err, "cannot obtain new revision")
	}
	// This is the first revision of a given name.
	firstRev := 0
	if id.Series == "" {
		// It's multi-series. Choose a revision that's greater
		// than any existing single-series variant.
		err := col.Find(bson.D{{"baseurl", mongodoc.BaseURL(id)}}).Sort("-revision").One(&doc)
		if err == nil {
			firstRev = doc.Revision + 1
		} else if err != mgo.ErrNotFound {
			return 0, errgo.Notef(err, "cannot find latest single-series revision")
		}
	}
	err = col.Insert(mongodoc.LatestRevision{
		URL:      id,
		BaseURL:  mongodoc.BaseURL(id),
		Revision: firstRev,
	})
	if mgo.IsDup(err) {
		// We were in a race and they won. Recur to
		// use the usual increment method to find the id.
		return s.NewRevision(id)
	}
	return firstRev, nil
}

// AddRevision records a new revision of the given id,
// meaning that any subequent NewRevision call
// for the id will return a higher revision number.
func (s *Store) AddRevision(id *router.ResolvedURL) error {
	if err := s.addRevision(&id.URL); err != nil {
		return errgo.Mask(err)
	}
	if purl := id.PromulgatedURL(); purl != nil {
		if err := s.addRevision(purl); err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}

// addRevision is the internal version of AddRevision
// which acts on a single non-resolved URL.
func (s *Store) addRevision(id *charm.URL) error {
	rev := id.Revision
	id = id.WithRevision(-1)
	_, err := s.DB.Revisions().Upsert(bson.D{
		{"_id", id},
		{"revision", bson.D{{"$lt", rev}}},
	}, mongodoc.LatestRevision{
		URL:      id,
		BaseURL:  mongodoc.BaseURL(id),
		Revision: rev,
	})
	if err == nil {
		return nil
	}
	if mgo.IsDup(err) {
		// We didn't find any documents to update
		// but still failed to insert a new document,
		// which implies that there is an existing
		// document with a higher id.
		return nil
	}
	return errgo.Notef(err, "cannot add revision")
}

// FindEntity finds the entity in the store with the given URL, which
// must be fully qualified. If the given URL has no user then it is
// assumed to be a promulgated entity. If fields is not nil, only its
// fields will be populated in the returned entities.
func (s *Store) FindEntity(url *router.ResolvedURL, fields map[string]int) (*mongodoc.Entity, error) {
	q := s.DB.Entities().Find(bson.D{{"_id", &url.URL}})
	if fields != nil {
		q = q.Select(fields)
	}
	var entity mongodoc.Entity
	err := q.One(&entity)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, errgo.WithCausef(nil, params.ErrNotFound, "entity not found")
		}
		return nil, errgo.Mask(err)
	}
	return &entity, nil
}

// FindEntities finds all entities in the store matching the given URL.
// If the given URL has no user then only promulgated entities will be
// queried. If the given URL channel does not represent an entity under
// development then only published entities will be queried. If fields
// is not nil, only its fields will be populated in the returned
// entities.
func (s *Store) FindEntities(url *charm.URL, fields map[string]int) ([]*mongodoc.Entity, error) {
	query := s.EntitiesQuery(url)
	if fields != nil {
		query = query.Select(fields)
	}
	var docs []*mongodoc.Entity
	err := query.All(&docs)
	if err != nil {
		return nil, errgo.Notef(err, "cannot find entities matching %s", url)
	}
	return docs, nil
}

// FindBestEntity finds the entity that provides the preferred match to
// the given URL, on the given channel. If the given URL has no user
// then only promulgated entities will be queried. If fields is not nil,
// only those fields will be populated in the returned entities.
//
// If the URL contains a revision then it is assumed to be fully formed
// and refer to a single entity; the channel is ignored.
//
// If the URL does not contain a revision then the channel is searched
// for the best match, here NoChannel will be treated as
// params.StableChannel.
func (s *Store) FindBestEntity(url *charm.URL, channel params.Channel, fields map[string]int) (*mongodoc.Entity, error) {
	if fields != nil {
		// Make sure we have all the fields we need to make a decision.
		// TODO this would be more efficient if we used bitmasks for field selection.
		nfields := map[string]int{
			"_id":                  1,
			"promulgated-url":      1,
			"promulgated-revision": 1,
			"series":               1,
			"revision":             1,
			"published":            1,
		}
		for f := range fields {
			nfields[f] = 1
		}
		fields = nfields
	}
	if url.Revision != -1 {
		// If the URL contains a revision, then it refers to a single entity.
		entity, err := s.findSingleEntity(url, fields)
		if errgo.Cause(err) == params.ErrNotFound {
			return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching charm or bundle for %s", url)
		} else if err != nil {
			return nil, errgo.Mask(err)
		}
		// If a channel was specified make sure the entity is in that channel.
		// This is crucial because if we don't do this, then the user could choose
		// to use any chosen set of ACLs against any entity.
		if params.ValidChannels[channel] && channel != params.UnpublishedChannel && !entity.Published[channel] {
			return nil, errgo.WithCausef(nil, params.ErrNotFound, "%s not found in %s channel", url, channel)
		}
		return entity, nil
	}

	switch channel {
	case params.UnpublishedChannel:
		return s.findUnpublishedEntity(url, fields)
	case params.NoChannel:
		channel = params.StableChannel
		fallthrough
	default:
		return s.findEntityInChannel(url, channel, fields)
	}
}

// findSingleEntity returns the entity referred to by URL. It is expected
// that the URL refers to only one entity and is fully formed. The url may
// refer to either a user-owned or promulgated charm name.
func (s *Store) findSingleEntity(url *charm.URL, fields map[string]int) (*mongodoc.Entity, error) {
	query := s.EntitiesQuery(url)
	if fields != nil {
		query = query.Select(fields)
	}
	var entity mongodoc.Entity
	err := query.One(&entity)
	if err == nil {
		return &entity, nil
	}
	if err == mgo.ErrNotFound {
		return nil, errgo.WithCausef(err, params.ErrNotFound, "no matching charm or bundle for %s", url)
	}
	return nil, errgo.Notef(err, "cannot find entities matching %s", url)
}

// findEntityInChannel attempts to find an entity on the given channel. The
// base entity for URL is retrieved and the series with the best match to
// URL.Series is used as the resolved entity.
func (s *Store) findEntityInChannel(url *charm.URL, ch params.Channel, fields map[string]int) (*mongodoc.Entity, error) {
	baseEntity, err := s.FindBaseEntity(url, map[string]int{
		"_id":             1,
		"channelentities": 1,
	})
	if errgo.Cause(err) == params.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching charm or bundle for %s", url)
	} else if err != nil {
		return nil, errgo.Mask(err)
	}
	var entityURL *charm.URL
	if url.Series == "" {
		var entitySeries string
		for s, u := range baseEntity.ChannelEntities[ch] {
			// Determine the preferred URL from the available series.
			//
			// Note that because each of the series has a different
			// score the only situation where the score in the URL is
			// where there is more than one series supported by a
			// multi-series charm. In this case the tie is broken by
			// looking for the preferred series from the ones
			// supported by the charm. To save fetching every charm
			// to look at the supported series the key is used,
			// because when a charm is listed as the published
			// version for a series it must support that series.
			if entityURL == nil ||
				seriesScore[u.Series] > seriesScore[entityURL.Series] ||
				// Note that if the two series are the same, they must both be
				// multi-series URLs.
				seriesScore[u.Series] == seriesScore[entityURL.Series] && seriesScore[s] > seriesScore[entitySeries] {
				entityURL = u
				entitySeries = s
			}
		}
	} else {
		entityURL = baseEntity.ChannelEntities[ch][url.Series]
	}
	if entityURL == nil {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching charm or bundle for %s", url)
	}
	return s.findSingleEntity(entityURL, fields)
}

// findUnpublishedEntity attempts to find an entity on the unpublished
// channel. This searches all entities in the store for the best match to
// the URL.
func (s *Store) findUnpublishedEntity(url *charm.URL, fields map[string]int) (*mongodoc.Entity, error) {
	entities, err := s.FindEntities(url, fields)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if len(entities) == 0 {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching charm or bundle for %s", url)
	}
	best := entities[0]
	for _, e := range entities {
		if seriesScore[e.Series] > seriesScore[best.Series] {
			best = e
			continue
		}
		if seriesScore[e.Series] < seriesScore[best.Series] {
			continue
		}
		if url.User == "" {
			if e.PromulgatedRevision > best.PromulgatedRevision {
				best = e
				continue
			}
		} else {
			if e.Revision > best.Revision {
				best = e
				continue
			}
		}
	}
	return best, nil
}

var seriesScore = map[string]int{
	"bundle":  -1,
	"lucid":   1000,
	"precise": 1001,
	"trusty":  1002,
	"xenial":  1003,
	"bionic":  1004,
	"quantal": 1,
	"raring":  2,
	"saucy":   3,
	"utopic":  4,
	"vivid":   5,
	"wily":    6,
	"yakkety": 7,
	"zesty":   8,
	"artful":  9,
	"cosmic":  0, // TODO: make this higher when cosmic is released

	// When we find a multi-series charm (no series) we
	// will always choose it in preference to a series-specific
	// charm
	"": 5000,
}

var seriesBundleOrEmpty = bson.D{{"$or", []bson.D{{{"series", "bundle"}}, {{"series", ""}}}}}

// EntitiesQuery creates a mgo.Query object that can be used to find entities
// matching the given URL. If the given URL has no user then the produced query
// will only match promulgated entities.
func (s *Store) EntitiesQuery(url *charm.URL) *mgo.Query {
	entities := s.DB.Entities()
	query := make(bson.D, 1, 5)
	query[0] = bson.DocElem{"name", url.Name}
	if url.User == "" {
		if url.Revision > -1 {
			query = append(query, bson.DocElem{"promulgated-revision", url.Revision})
		} else {
			query = append(query, bson.DocElem{"promulgated-revision", bson.D{{"$gt", -1}}})
		}
	} else {
		query = append(query, bson.DocElem{"user", url.User})
		if url.Revision > -1 {
			query = append(query, bson.DocElem{"revision", url.Revision})
		}
	}
	if url.Series == "" {
		if url.Revision > -1 {
			// If we're specifying a revision we must be searching
			// for a canonical URL, so search for a multi-series
			// charm or a bundle.
			query = append(query, seriesBundleOrEmpty...)
		}
	} else if url.Series == "bundle" {
		query = append(query, bson.DocElem{"series", "bundle"})
	} else {
		query = append(query, bson.DocElem{"supportedseries", url.Series})
	}
	return entities.Find(query)
}

// FindBaseEntity finds the base entity in the store using the given URL,
// which can either represent a fully qualified entity or a base id.
// If fields is not nil, only those fields will be populated in the
// returned base entity.
func (s *Store) FindBaseEntity(url *charm.URL, fields map[string]int) (*mongodoc.BaseEntity, error) {
	var query *mgo.Query
	if url.User == "" {
		query = s.DB.BaseEntities().Find(bson.D{{"name", url.Name}, {"promulgated", 1}})
	} else {
		query = s.DB.BaseEntities().FindId(mongodoc.BaseURL(url))
	}
	if fields != nil {
		query = query.Select(fields)
	}
	var baseEntity mongodoc.BaseEntity
	if err := query.One(&baseEntity); err != nil {
		if err == mgo.ErrNotFound {
			return nil, errgo.WithCausef(nil, params.ErrNotFound, "base entity not found")
		}
		return nil, errgo.Notef(err, "cannot find base entity %v", url)
	}
	return &baseEntity, nil
}

// FieldSelector returns a field selector that will select
// the given fields, or all fields if none are specified.
func FieldSelector(fields ...string) map[string]int {
	if len(fields) == 0 {
		return nil
	}
	sel := make(map[string]int, len(fields))
	for _, field := range fields {
		sel[field] = 1
	}
	return sel
}

// UpdateEntity applies the provided update to the entity described by
// url. If there are no entries in update then no update is performed,
// and no error is returned.
func (s *Store) UpdateEntity(url *router.ResolvedURL, update bson.D) error {
	if len(update) == 0 {
		return nil
	}
	if err := s.DB.Entities().Update(bson.D{{"_id", &url.URL}}, update); err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(err, params.ErrNotFound, "cannot update %q", url)
		}
		return errgo.Notef(err, "cannot update %q", url)
	}
	return nil
}

// UpdateBaseEntity applies the provided update to the base entity of
// url. If there are no entries in update then no update is performed,
// and no error is returned.
func (s *Store) UpdateBaseEntity(url *router.ResolvedURL, update bson.D) error {
	if len(update) == 0 {
		return nil
	}
	if err := s.DB.BaseEntities().Update(bson.D{{"_id", mongodoc.BaseURL(&url.URL)}}, update); err != nil {
		if err == mgo.ErrNotFound {
			return errgo.WithCausef(err, params.ErrNotFound, "cannot update base entity for %q", url)
		}
		return errgo.Notef(err, "cannot update base entity for %q", url)
	}
	return nil
}

var ErrPublishResourceMismatch = errgo.Newf("charm published with incorrect resources")

// Publish assigns channels to the entity corresponding to the given URL.
// An error is returned if no channels are provided. See params.ValidChannels
// for the list of supported channels. The unpublished channel cannot
// be provided.
//
// If the given resources do not match those expected or they're not
// found, an error with a ErrPublichResourceMismatch cause will be returned.
func (s *Store) Publish(url *router.ResolvedURL, resources map[string]int, channels ...params.Channel) error {
	var updateSearch bool
	// Throw away any channels that we don't like.
	actualChannels := make([]params.Channel, 0, len(channels))
	for _, c := range channels {
		if !params.ValidChannels[c] || c == params.UnpublishedChannel {
			continue
		}
		actualChannels = append(actualChannels, c)
		if c == params.StableChannel {
			updateSearch = true
		}
	}
	channels = actualChannels
	if len(channels) == 0 {
		return errgo.Newf("cannot update %q: no valid channels provided", url)
	}
	entity, err := s.FindEntity(url, FieldSelector("series", "supportedseries", "charmmeta", "baseurl"))
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	resourceDocs := make([]mongodoc.ResourceRevision, 0, len(resources))
	if err = s.checkPublishedResources(entity, resources); err != nil {
		return errgo.WithCausef(err, ErrPublishResourceMismatch, "")
	}
	for name, rev := range resources {
		resourceDocs = append(resourceDocs, mongodoc.ResourceRevision{
			Name:     name,
			Revision: rev,
		})
	}

	series := entity.SupportedSeries
	if len(series) == 0 {
		series = []string{entity.Series}
	}
	// Update the entity's published channels.
	update := make(bson.D, 0, len(channels)*(len(series)+1)) // ...ish.
	for _, c := range channels {
		update = append(update, bson.DocElem{"published." + string(c), true})
	}
	if err := s.UpdateEntity(url, bson.D{{"$set", update}}); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	// Update the base entity.
	update = update[:0]
	for _, c := range channels {
		for _, s := range series {
			update = append(update, bson.DocElem{fmt.Sprintf("channelentities.%s.%s", c, s), entity.URL})
		}
		update = append(update, bson.DocElem{fmt.Sprintf("channelresources.%s", c), resourceDocs})
	}
	if err := s.UpdateBaseEntity(url, bson.D{{"$set", update}}); err != nil {
		return errgo.Mask(err)
	}

	if !updateSearch {
		return nil
	}

	// Add entity to ElasticSearch.
	if err := s.UpdateSearch(url); err != nil {
		return errgo.Notef(err, "cannot index %s to ElasticSearch", url)
	}
	return nil
}

func (s *Store) checkPublishedResources(entity *mongodoc.Entity, resources map[string]int) error {
	knownResources, _, err := s.charmResources(entity.BaseURL)
	if err != nil {
		return errgo.Mask(err)
	}
	for name, rev := range resources {
		if !charmHasResource(entity.CharmMeta, name) {
			return errgo.Newf("charm does not have resource %q", name)
		}
		if _, ok := knownResources[name][rev]; !ok {
			return errgo.Newf("%s resource %q not found", entity.URL, fmt.Sprintf("%s/%d", name, rev))
		}
	}
	if entity.CharmMeta == nil {
		return nil
	}
	var missing []string
	for name := range entity.CharmMeta.Resources {
		if _, ok := resources[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return errgo.Newf("resources are missing from publish request: %s", strings.Join(missing, ", "))
}

// SetPromulgated sets whether the base entity of url is promulgated, If
// promulgated is true it also unsets promulgated on any other base
// entity for entities with the same name. It also calculates the next
// promulgated URL for the entities owned by the new owner and sets those
// entities appropriately.
//
// Note: This code is known to have some unfortunate (but not dangerous)
// race conditions. It is possible that if one or more promulgations
// happens concurrently for the same entity name then it could result in
// more than one base entity being promulgated. If this happens then
// uploads to either user will get promulgated names, these names will
// never clash. This situation is easily remedied by setting the
// promulgated user for this charm again, even to one of the ones that is
// already promulgated. It can also result in the latest promulgated
// revision of the charm not being one created by the promulgated user.
// This will be remedied when a new charm is uploaded by the promulgated
// user. As promulgation is a rare operation, it is considered that the
// chances this will happen are slim.
func (s *Store) SetPromulgated(url *router.ResolvedURL, promulgate bool) error {
	baseEntities := s.DB.BaseEntities()
	base := mongodoc.BaseURL(&url.URL)
	if !promulgate {
		err := baseEntities.UpdateId(
			base,
			bson.D{{"$set", bson.D{{"promulgated", mongodoc.IntBool(false)}}}},
		)
		if err != nil {
			if errgo.Cause(err) == mgo.ErrNotFound {
				return errgo.WithCausef(nil, params.ErrNotFound, "base entity %q not found", base)
			}
			return errgo.Notef(err, "cannot unpromulgate base entity %q", base)
		}
		if err := s.UpdateSearchBaseURL(base); err != nil {
			return errgo.Notef(err, "cannot update search entities for %q", base)
		}
		return nil
	}

	// Find any currently promulgated base entities for this charm name.
	// Under normal circumstances there should be a maximum of one of these,
	// but we should attempt to recover if there is an error condition.
	iter := baseEntities.Find(
		bson.D{
			{"_id", bson.D{{"$ne", base}}},
			{"name", base.Name},
			{"promulgated", mongodoc.IntBool(true)},
		},
	).Iter()
	defer iter.Close()
	var baseEntity mongodoc.BaseEntity
	for iter.Next(&baseEntity) {
		err := baseEntities.UpdateId(
			baseEntity.URL,
			bson.D{{"$set", bson.D{{"promulgated", mongodoc.IntBool(false)}}}},
		)
		if err != nil {
			return errgo.Notef(err, "cannot unpromulgate base entity %q", baseEntity.URL)
		}
		if err := s.UpdateSearchBaseURL(baseEntity.URL); err != nil {
			return errgo.Notef(err, "cannot update search entities for %q", baseEntity.URL)
		}
	}
	if err := iter.Close(); err != nil {
		return errgo.Notef(err, "cannot close mgo iterator")
	}

	// Set the promulgated flag on the base entity.
	err := s.DB.BaseEntities().UpdateId(base, bson.D{{"$set", bson.D{{"promulgated", mongodoc.IntBool(true)}}}})
	if err != nil {
		if errgo.Cause(err) == mgo.ErrNotFound {
			return errgo.WithCausef(nil, params.ErrNotFound, "base entity %q not found", base)
		}
		return errgo.Notef(err, "cannot promulgate base entity %q", base)
	}

	// Find the latest revision in each series of the promulgated entities
	// with the same name as the base entity. Note that this works because:
	//     1) promulgated URLs always have the same charm name as their
	//     non-promulgated counterparts.
	//     2) bundles cannot have names that overlap with charms.
	// Because of 1), we are sure that selecting on the entity name will
	// select all entities with a matching promulgated URL name. Because of
	// 2) we are sure that we are only updating all charms or the single
	// bundle entity.

	iter = s.DB.Entities().Find(bson.D{{
		"promulgated-revision", bson.D{{"$gt", -1}},
	}, {
		"name", base.Name,
	}}).Select(FieldSelector("promulgated-revision", "supportedseries", "series")).Iter()

	latestPromulgated := make(map[string]int)
	oldMultiSeries := false
	var e mongodoc.Entity
	for iter.Next(&e) {
		oldMultiSeries = oldMultiSeries || e.Series == ""
		entitySeries := e.SupportedSeries
		if e.Series == "bundle" {
			entitySeries = []string{"bundle"}
		}
		for _, series := range entitySeries {
			if rev, ok := latestPromulgated[series]; !ok || rev < e.PromulgatedRevision {
				latestPromulgated[series] = e.PromulgatedRevision
			}
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot close mgo iterator")
	}

	// Find the latest revision in each series of entities with the promulgated base URL.
	// After this, latestOwned will have an entry for each series, with multi-series
	// charms having an empty series.
	type result struct {
		URL             *charm.URL
		Series          string `bson:"_id"`
		SupportedSeries []string
		Revision        int
	}
	latestOwned := make(map[string]result)
	iter = s.DB.Entities().Pipe([]bson.D{
		{{"$match", bson.D{{"baseurl", base}}}},
		{{"$sort", bson.D{{"revision", 1}}}},
		{{"$group", bson.D{
			{"_id", "$series"},
			{"url", bson.D{{"$last", "$_id"}}},
			{"supportedseries", bson.D{{"$last", "$supportedseries"}}},
			{"revision", bson.D{{"$last", "$revision"}}},
		}}},
	}).Iter()
	var r result
	for iter.Next(&r) {
		latestOwned[r.Series] = r
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot close mgo iterator")
	}

	// Delete all series we don't want to promulgate.
	if _, ok := latestOwned[""]; ok || oldMultiSeries {
		// The newly promulgated charm will be multi-series or
		// there was previously a multi-series charm, so do not
		// promulgate any single series charms.
		for series, r := range latestOwned {
			if series != "" {
				logger.Infof("ignoring non-multi-series entity for promulgation %v", r.URL)
				delete(latestOwned, series)
			}
		}
	}

	// Update the newest entity in each series with the new base URL to have a
	// promulgated URL if it does not already have one.
	for _, r := range latestOwned {
		// Assign the entity a promulgated revision of one more than the maximum
		// of the promulgated revision of any of the supported
		// series.
		entitySeries := r.SupportedSeries
		if r.Series == "bundle" {
			entitySeries = []string{"bundle"}
		}
		maxRev := -1
		for _, series := range entitySeries {
			if rev, ok := latestPromulgated[series]; ok && rev > maxRev {
				maxRev = rev
			}
		}
		pID := *r.URL
		pID.User = ""
		pID.Revision = maxRev + 1
		logger.Infof("updating promulgation URL of %v to %v", r.URL, &pID)
		err := s.DB.Entities().Update(
			bson.D{
				{"_id", r.URL},
				{"promulgated-revision", -1},
			},
			bson.D{
				{"$set", bson.D{
					{"promulgated-url", &pID},
					{"promulgated-revision", pID.Revision},
				}},
			},
		)
		if err != nil && err != mgo.ErrNotFound {
			// If we get NotFound it is most likely because the latest owned revision is
			// already promulgated, so carry on.
			return errgo.Notef(err, "cannot update promulgated URLs")
		}
	}

	// Update the search record for the newest entity.
	if err := s.UpdateSearchBaseURL(base); err != nil {
		return errgo.Notef(err, "cannot update search entities for %q", base)
	}
	return nil
}

// SetPerms sets the ACL specified by which for the base entity with the
// given id. The which parameter is in the form "channel.operation",
// where channel is the string corresponding to one of the ValidChannels
// and operation is one of "read" or "write". If which does not specify a
// channel then the unpublished ACL is updated.
// This is only provided for testing.
func (s *Store) SetPerms(id *charm.URL, which string, acl ...string) error {
	return s.DB.BaseEntities().UpdateId(mongodoc.BaseURL(id), bson.D{{"$set",
		bson.D{{"channelacls." + which, acl}},
	}})
}

// MatchingInterfacesQuery returns a mongo query
// that will find any charms that require any interfaces
// in the required slice or provide any interfaces in the
// provided slice.
func (s *Store) MatchingInterfacesQuery(required, provided []string) *mgo.Query {
	return s.DB.Entities().Find(bson.D{{
		"$or", []bson.D{{{
			"charmrequiredinterfaces", bson.D{{
				"$elemMatch", bson.D{{
					"$in", required,
				}},
			}},
		}}, {{
			"charmprovidedinterfaces", bson.D{{
				"$elemMatch", bson.D{{
					"$in", provided,
				}},
			}},
		}}},
	}})
}

// AddLog adds a log message to the database.
func (s *Store) AddLog(data *json.RawMessage, logLevel mongodoc.LogLevel, logType mongodoc.LogType, urls []*charm.URL) error {
	// Encode the JSON data.
	b, err := json.Marshal(data)
	if err != nil {
		return errgo.Notef(err, "cannot marshal log data")
	}

	// Add the base URLs to the list of references associated with the log.
	// Also remove duplicate URLs while maintaining the references' order.
	var allUrls []*charm.URL
	urlMap := make(map[string]bool)
	for _, url := range urls {
		urlStr := url.String()
		if ok, _ := urlMap[urlStr]; !ok {
			urlMap[urlStr] = true
			allUrls = append(allUrls, url)
		}
		base := mongodoc.BaseURL(url)
		urlStr = base.String()
		if ok, _ := urlMap[urlStr]; !ok {
			urlMap[urlStr] = true
			allUrls = append(allUrls, base)
		}
	}

	// Add the log to the database.
	log := &mongodoc.Log{
		Data:  b,
		Level: logLevel,
		Type:  logType,
		URLs:  allUrls,
		Time:  time.Now(),
	}
	if err := s.DB.Logs().Insert(log); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// DeleteEntity deletes the entity with the given id from the store. If
// the entity is the current published revision for any channel or the
// last revision with the same base entity, it returns an error with an
// ErrForbidden cause.
func (s *Store) DeleteEntity(id *router.ResolvedURL) error {
	// Find all the entities that use the base URL of id so
	// that we can refuse to delete the last reference to the
	// base URL.
	var entities []*mongodoc.Entity
	err := s.DB.Entities().Find(bson.D{{"baseurl", mongodoc.BaseURL(&id.URL)}}).
		Select(FieldSelector("blobhash", "prev5blobhash")).
		All(&entities)
	if err != nil {
		return errgo.Mask(err)
	}
	var entity *mongodoc.Entity
	for _, e := range entities {
		if *e.URL == id.URL {
			entity = e
			break
		}
	}
	if entity == nil {
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	if len(entities) == 1 {
		return errgo.WithCausef(nil, params.ErrForbidden, "cannot delete last revision of charm or bundle")
	}
	// Find the base entity so that we can check that we're not
	// removing any of the current released revisions.
	baseEntity, err := s.FindBaseEntity(&id.URL, map[string]int{
		"_id":             1,
		"channelentities": 1,
	})
	if err != nil {
		return errgo.Mask(err)
	}
	var published []string
	for ch, ids := range baseEntity.ChannelEntities {
		for _, publishedId := range ids {
			if *publishedId == id.URL {
				published = append(published, string(ch))
			}
		}
	}
	if len(published) > 0 {
		sort.Strings(published)
		return errgo.WithCausef(nil, params.ErrForbidden, "cannot delete %q because it is the current revision in channels %s", &id.URL, published)
	}
	// Remove the entity.
	if err := s.DB.Entities().RemoveId(&id.URL); err != nil {
		if err == mgo.ErrNotFound {
			// Someone else got there first.
			err = params.ErrNotFound
		}
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	return nil
}

// StoreDatabase wraps an mgo.DB ands adds a few convenience methods.
type StoreDatabase struct {
	*mgo.Database
}

// clone copies the StoreDatabase, cloning the underlying mgo session.
func (s StoreDatabase) clone() StoreDatabase {
	return StoreDatabase{
		&mgo.Database{
			Name:    s.Name,
			Session: s.Session.Clone(),
		},
	}
}

// copy copies the StoreDatabase, copying the underlying mgo session.
func (s StoreDatabase) copy() StoreDatabase {
	return StoreDatabase{
		&mgo.Database{
			Name:    s.Name,
			Session: s.Session.Copy(),
		},
	}
}

// Close closes the store database's underlying session.
func (s StoreDatabase) Close() {
	s.Session.Close()
}

// Entities returns the mongo collection where entities are stored.
func (s StoreDatabase) Entities() *mgo.Collection {
	return s.C("entities")
}

// Revisions holds the mongo collection where the latest
// revision numbers are stored.
func (s StoreDatabase) Revisions() *mgo.Collection {
	return s.C("revisions")
}

// BaseEntities returns the mongo collection where base entities are stored.
func (s StoreDatabase) BaseEntities() *mgo.Collection {
	return s.C("base_entities")
}

// Resources returns the mongo collection where resources are stored.
func (s StoreDatabase) Resources() *mgo.Collection {
	return s.C("resources")
}

// Logs returns the Mongo collection where charm store logs are stored.
func (s StoreDatabase) Logs() *mgo.Collection {
	return s.C("logs")
}

// Migrations returns the Mongo collection where the migration info is stored.
func (s StoreDatabase) Migrations() *mgo.Collection {
	return s.C("migrations")
}

func (s StoreDatabase) Macaroons() *mgo.Collection {
	return s.C("macaroons")
}

// allCollections holds for each collection used by the charm store a
// function returns that collection.
var allCollections = []func(StoreDatabase) *mgo.Collection{
	StoreDatabase.BaseEntities,
	StoreDatabase.Entities,
	StoreDatabase.Logs,
	StoreDatabase.Macaroons,
	StoreDatabase.Migrations,
	StoreDatabase.Resources,
	StoreDatabase.Revisions,
	StoreDatabase.StatCounters,
	StoreDatabase.StatTokens,
}

// Collections returns a slice of all the collections used
// by the charm store.
func (s StoreDatabase) Collections() []*mgo.Collection {
	cs := make([]*mgo.Collection, len(allCollections))
	for i, f := range allCollections {
		cs[i] = f(s)
	}
	return cs
}

// readerAtSeeker adapts an io.ReadSeeker to an io.ReaderAt.
type readerAtSeeker struct {
	r   io.ReadSeeker
	off int64
}

// ReadAt implemnts SizeReaderAt.ReadAt.
func (r *readerAtSeeker) ReadAt(buf []byte, off int64) (n int, err error) {
	if off != r.off {
		_, err = r.r.Seek(off, 0)
		if err != nil {
			return 0, err
		}
		r.off = off
	}
	n, err = io.ReadFull(r.r, buf)
	r.off += int64(n)
	return n, err
}

// ReaderAtSeeker adapts r so that it can be used as
// a ReaderAt. Note that, contrary to the io.ReaderAt
// contract, it is not OK to use concurrently.
func ReaderAtSeeker(r io.ReadSeeker) io.ReaderAt {
	return &readerAtSeeker{r, 0}
}

// Search searches the store for the given SearchParams.
// It returns a SearchResult containing the results of the search.
func (store *Store) Search(sp SearchParams) (SearchResult, error) {
	result, err := store.ES.search(sp)
	if err != nil {
		return SearchResult{}, errgo.Mask(err)
	}
	return result, nil
}

var listFilters = map[string]string{
	"name":        "name",
	"owner":       "user",
	"series":      "serties",
	"type":        "type",
	"promulgated": "promulgated-revision",
}

func prepareList(sp SearchParams) (filters map[string]interface{}, err error) {
	if len(sp.Text) > 0 {
		return nil, errgo.New("text not allowed")
	}
	if sp.Limit > 0 {
		return nil, errgo.New("limit not allowed")
	}
	if sp.Skip > 0 {
		return nil, errgo.New("skip not allowed")
	}
	if sp.AutoComplete {
		return nil, errgo.New("autocomplete not allowed")
	}

	filters = make(map[string]interface{})
	for k, v := range sp.Filters {
		switch k {
		case "name":
			filters[k] = v[0]
		case "owner":
			filters["user"] = v[0]
		case "series":
			filters["series"] = v[0]
		case "type":
			if v[0] == "bundle" {
				filters["series"] = "bundle"
			} else {
				filters["series"] = map[string]interface{}{"$ne": "bundle"}
			}
		case "promulgated":
			if v[0] != "0" {
				filters["promulgated-revision"] = map[string]interface{}{"$gte": 0}
			} else {
				filters["promulgated-revision"] = map[string]interface{}{"$lt": 0}
			}
		default:
			return nil, errgo.Newf("filter %q not allowed", k)
		}
	}
	return filters, nil
}

// ListQuery holds a list query from which an iterator
// can be created.
type ListQuery struct {
	store   *Store
	filters map[string]interface{}
	sort    bson.D
}

// ListQuery lists entities in the store that conform to the
// given search parameters. It returns a ListQuery
// that can be used to iterate through the list.
//
// Sort criteria in the search parameters are ignored - the
// results are returned in arbitrary order.
func (store *Store) ListQuery(sp SearchParams) (*ListQuery, error) {
	filters, err := prepareList(sp)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &ListQuery{
		store:   store,
		filters: filters,
	}, nil
}

// listIterGroupIDSpec holds the part of the list aggregration pipeline
// that determines the _id field used for grouping.
// The _id result holds the base URL, the series,
// and the combination of channels that the entity has been
// published to.
var listIterGroupIDSpec = func() bson.Raw {
	concat := []interface{}{
		"$baseurl",
		" ",
		"$series",
	}
	for i, c := range params.OrderedChannels {
		concat = append(concat, bson.D{{
			"$cond", []interface{}{
				"$published." + string(c),
				fmt.Sprint(".", i),
				"",
			},
		}})
	}
	data, err := bson.Marshal(bson.D{{"$concat", concat}})
	if err != nil {
		panic(err)
	}
	var raw bson.Raw
	if err := bson.Unmarshal(data, &raw); err != nil {
		panic(err)
	}
	return raw
}()

func (lq *ListQuery) Iter(fields map[string]int) *mgo.Iter {
	qfields := FieldSelector(
		"promulgated-url",
		"published",
		"name",
		"user",
		"series",
	)
	for f := range fields {
		qfields[f] = 1
	}
	// _id and url have special treatment.
	delete(qfields, "_id")
	delete(qfields, "url")

	group := make(bson.D, 0, 2+len(qfields))
	group = append(group, bson.DocElem{"_id", listIterGroupIDSpec})
	group = append(group, bson.DocElem{"url", bson.D{{"$last", "$_id"}}})
	for field := range qfields {
		group = append(group, bson.DocElem{field, bson.D{{"$last", "$" + field}}})
	}

	project := make(bson.D, 0, len(qfields)+1)
	project = append(project, bson.DocElem{"_id", "$url"})
	for f := range qfields {
		project = append(project, bson.DocElem{f, "$" + f})
	}

	return lq.store.DB.Entities().Pipe([]bson.D{
		{{"$match", lq.filters}},
		{{"$sort", bson.D{{"revision", 1}}}},
		{{"$group", group}},
		{{"$project", project}},
	}).Iter()
}

// SynchroniseElasticsearch creates new indexes in elasticsearch
// and populates them with the current data from the mongodb database.
func (s *Store) SynchroniseElasticsearch() error {
	if err := s.ES.ensureIndexes(true); err != nil {
		return errgo.Notef(err, "cannot create indexes")
	}
	if err := s.syncSearch(); err != nil {
		return errgo.Notef(err, "cannot synchronise indexes")
	}
	return nil
}

// EntityResolvedURL returns the ResolvedURL for the entity. It requires
// that the PromulgatedURL field has been filled out in the entity.
func EntityResolvedURL(e *mongodoc.Entity) *router.ResolvedURL {
	rurl := &router.ResolvedURL{
		URL:                 *e.URL,
		PromulgatedRevision: -1,
	}
	if e.PromulgatedURL != nil {
		rurl.PromulgatedRevision = e.PromulgatedURL.Revision
	}
	return rurl
}
