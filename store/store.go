// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The store package is capable of storing and updating charms in a MongoDB
// database, as well as maintaining further information about them such as
// the VCS revision the charm was loaded from and the URLs for the charms.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
)

// The following MongoDB collections are currently used:
//
//     juju.events        - Log of events relating to the lifecycle of charms
//     juju.charms        - Information about the stored charms
//     juju.charmfs.*     - GridFS with the charm files
//     juju.locks         - Has unique keys with url of updating charms
//     juju.stat.counters - Counters for statistics
//     juju.stat.tokens   - Tokens used in statistics counter keys

var (
	ErrUpdateConflict  = errors.New("charm update in progress")
	ErrRedundantUpdate = errors.New("charm is up-to-date")

	// Note that this error message is part of the API, since it's sent
	// both in charm-info and charm-event responses as errors indicating
	// that the given charm or charm event wasn't found.
	ErrNotFound = errors.New("entry not found")
)

const (
	UpdateTimeout = 600e9
)

// Store holds a connection to a charm store.
type Store struct {
	session *storeSession

	// Cache for statistics key words (two generations).
	cacheMu       sync.RWMutex
	statsIdNew    map[string]int
	statsIdOld    map[string]int
	statsTokenNew map[int]string
	statsTokenOld map[int]string
}

// Open creates a new session with the store. It connects to the MongoDB
// server at the given address (as expected by the Mongo function in the
// labix.org/v2/mgo package).
func Open(mongoAddr string) (store *Store, err error) {
	log.Infof("store: Store opened. Connecting to: %s", mongoAddr)
	store = &Store{}
	session, err := mgo.Dial(mongoAddr)
	if err != nil {
		log.Errorf("store: Error connecting to MongoDB: %v", err)
		return nil, err
	}

	store = &Store{session: &storeSession{session}}

	// Ignore error. It'll always fail after created.
	// TODO Check the error once mgo hands it to us.
	_ = store.session.DB("juju").Run(bson.D{{"create", "stat.counters"}, {"autoIndexId", false}}, nil)

	if err := store.ensureIndexes(); err != nil {
		session.Close()
		return nil, err
	}

	// Put the used socket back in the pool.
	session.Refresh()
	return store, nil
}

func (s *Store) ensureIndexes() error {
	session := s.session
	indexes := []struct {
		c *mgo.Collection
		i mgo.Index
	}{{
		session.StatCounters(),
		mgo.Index{Key: []string{"k", "t"}, Unique: true},
	}, {
		session.StatTokens(),
		mgo.Index{Key: []string{"t"}, Unique: true},
	}, {
		session.Charms(),
		mgo.Index{Key: []string{"urls", "revision"}, Unique: true},
	}, {
		session.Events(),
		mgo.Index{Key: []string{"urls", "digest"}},
	}}
	for _, idx := range indexes {
		err := idx.c.EnsureIndex(idx.i)
		if err != nil {
			log.Errorf("store: Error ensuring stat.counters index: %v", err)
			return err
		}
	}
	return nil
}

// Close terminates the connection with the store.
func (s *Store) Close() {
	s.session.Close()
}

// statsKey returns the compound statistics identifier that represents key.
// If write is true, the identifier will be created if necessary.
// Identifiers have a form similar to "ab:c:def:", where each section is a
// base-32 number that represents the respective word in key. This form
// allows efficiently indexing and searching for prefixes, while detaching
// the key content and size from the actual words used in key.
func (s *Store) statsKey(session *storeSession, key []string, write bool) (string, error) {
	if len(key) == 0 {
		return "", fmt.Errorf("store: empty statistics key")
	}
	tokens := session.StatTokens()
	skey := make([]byte, 0, len(key)*4)
	// Retry limit is mainly to prevent infinite recursion in edge cases,
	// such as if the database is ever run in read-only mode.
	// The logic below should deteministically stop in normal scenarios.
	var err error
	for i, retry := 0, 30; i < len(key) && retry > 0; retry-- {
		err = nil
		id, found := s.statsTokenId(key[i])
		if !found {
			var t tokenId
			err = tokens.Find(bson.D{{"t", key[i]}}).One(&t)
			if err == mgo.ErrNotFound {
				if !write {
					return "", ErrNotFound
				}
				t.Id, err = tokens.Count()
				if err != nil {
					continue
				}
				t.Id++
				t.Token = key[i]
				err = tokens.Insert(&t)
			}
			if err != nil {
				continue
			}
			s.cacheStatsTokenId(t.Token, t.Id)
			id = t.Id
		}
		skey = strconv.AppendInt(skey, int64(id), 32)
		skey = append(skey, ':')
		i++
	}
	if err != nil {
		return "", err
	}
	return string(skey), nil
}

const statsTokenCacheSize = 1024

type tokenId struct {
	Id    int    "_id"
	Token string "t"
}

// cacheStatsTokenId adds the id for token into the cache.
// The cache has two generations so that the least frequently used
// tokens are evicted regularly.
func (s *Store) cacheStatsTokenId(token string, id int) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	// Can't possibly be >, but reviews want it for defensiveness.
	if len(s.statsIdNew) >= statsTokenCacheSize {
		s.statsIdOld = s.statsIdNew
		s.statsIdNew = nil
		s.statsTokenOld = s.statsTokenNew
		s.statsTokenNew = nil
	}
	if s.statsIdNew == nil {
		s.statsIdNew = make(map[string]int, statsTokenCacheSize)
		s.statsTokenNew = make(map[int]string, statsTokenCacheSize)
	}
	s.statsIdNew[token] = id
	s.statsTokenNew[id] = token
}

// statsTokenId returns the id for token from the cache, if found.
func (s *Store) statsTokenId(token string) (id int, found bool) {
	s.cacheMu.RLock()
	id, found = s.statsIdNew[token]
	if found {
		s.cacheMu.RUnlock()
		return
	}
	id, found = s.statsIdOld[token]
	s.cacheMu.RUnlock()
	if found {
		s.cacheStatsTokenId(token, id)
	}
	return
}

// statsIdToken returns the token for id from the cache, if found.
func (s *Store) statsIdToken(id int) (token string, found bool) {
	s.cacheMu.RLock()
	token, found = s.statsTokenNew[id]
	if found {
		s.cacheMu.RUnlock()
		return
	}
	token, found = s.statsTokenOld[id]
	s.cacheMu.RUnlock()
	if found {
		s.cacheStatsTokenId(token, id)
	}
	return
}

var counterEpoch = time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

func timeToStamp(t time.Time) int32 {
	return int32(t.Unix() - counterEpoch)
}

// IncCounter increases by one the counter associated with the composed key.
func (s *Store) IncCounter(key []string) error {
	session := s.session.Copy()
	defer session.Close()

	skey, err := s.statsKey(session, key, true)
	if err != nil {
		return err
	}

	t := time.Now().UTC()
	// Round to the start of the minute so we get one document per minute at most.
	t = t.Add(-time.Duration(t.Second()) * time.Second)
	counters := session.StatCounters()
	_, err = counters.Upsert(bson.D{{"k", skey}, {"t", timeToStamp(t)}}, bson.D{{"$inc", bson.D{{"c", 1}}}})
	return err
}

// CounterRequest represents a request to aggregate counter values.
type CounterRequest struct {
	// Key and Prefix determine the counter keys to match.
	// If Prefix is false, Key must match exactly. Otherwise, counters
	// must begin with Key and have at least one more key token.
	Key    []string
	Prefix bool

	// If List is true, matching counters are aggregated under their
	// prefixes instead of being returned as a single overall sum.
	//
	// For example, given the following counts:
	//
	//   {"a", "b"}: 1,
	//   {"a", "c"}: 3
	//   {"a", "c", "d"}: 5
	//   {"a", "c", "e"}: 7
	//
	// and assuming that Prefix is true, the following keys will
	// present the respective results if List is true:
	//
	//        {"a"} => {{"a", "b"}, 1, false},
	//                 {{"a", "c"}, 3, false},
	//                 {{"a", "c"}, 12, true}
	//   {"a", "c"} => {{"a", "c", "d"}, 3, false},
	//                 {{"a", "c", "e"}, 5, false}
	//
	// If List is false, the same key prefixes will present:
	//
	//        {"a"} => {{"a"}, 16, true}
	//   {"a", "c"} => {{"a", "c"}, 12, false}
	//
	List bool

	// By defines the period covered by each aggregated data point.
	// If unspecified, it defaults to ByAll, which aggregates all
	// matching data points in a single entry.
	By CounterRequestBy

	// Start, if provided, changes the query so that only data points
	// ocurring at the given time or afterwards are considered.
	Start time.Time

	// Stop, if provided, changes the query so that only data points
	// ocurring at the given time or before are considered.
	Stop time.Time
}

type CounterRequestBy int

const (
	ByAll CounterRequestBy = iota
	ByDay
	ByWeek
)

type Counter struct {
	Key    []string
	Prefix bool
	Count  int64
	Time   time.Time
}

// Counters aggregates and returns counter values according to the provided request.
func (s *Store) Counters(req *CounterRequest) ([]Counter, error) {
	session := s.session.Copy()
	defer session.Close()

	tokensColl := session.StatTokens()
	countersColl := session.StatCounters()

	searchKey, err := s.statsKey(session, req.Key, false)
	if err == ErrNotFound {
		if !req.List {
			return []Counter{{Key: req.Key, Prefix: req.Prefix, Count: 0}}, nil
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var regex string
	if req.Prefix {
		regex = "^" + searchKey + ".+"
	} else {
		regex = "^" + searchKey + "$"
	}

	// This reduce function simply sums, for each emitted key, all the values found under it.
	job := mgo.MapReduce{Reduce: "function(key, values) { return Array.sum(values); }"}
	var emit string
	switch req.By {
	case ByDay:
		emit = "emit(k+'@'+NumberInt(this.t/86400), this.c);"
	case ByWeek:
		emit = "emit(k+'@'+NumberInt(this.t/604800), this.c);"
	default:
		emit = "emit(k, this.c);"
	}
	if req.List && req.Prefix {
		// For a search key "a:b:" matching a key "a:b:c:d:e:", this map function emits "a:b:c:*".
		// For a search key "a:b:" matching a key "a:b:c:", it emits "a:b:c:".
		// For a search key "a:b:" matching a key "a:b:", it emits "a:b:".
		job.Scope = bson.D{{"searchKeyLen", len(searchKey)}}
		job.Map = fmt.Sprintf(`
			function() {
				var k = this.k;
				var i = k.indexOf(':', searchKeyLen)+1;
				if (k.length > i)  { k = k.substr(0, i)+'*'; }
				%s
			}`, emit)
	} else {
		// For a search key "a:b:" matching a key "a:b:c:d:e:", this map function emits "a:b:*".
		// For a search key "a:b:" matching a key "a:b:c:", it also emits "a:b:*".
		// For a search key "a:b:" matching a key "a:b:", it emits "a:b:".
		emitKey := searchKey
		if req.Prefix {
			emitKey += "*"
		}
		job.Scope = bson.D{{"emitKey", emitKey}}
		job.Map = fmt.Sprintf(`
			function() {
				var k = emitKey;
				%s
			}`, emit)
	}

	var result []struct {
		Key   string `bson:"_id"`
		Value int64
	}
	var query, tquery bson.D
	if !req.Start.IsZero() {
		tquery = append(tquery, bson.DocElem{"$gte", timeToStamp(req.Start)})
	}
	if !req.Stop.IsZero() {
		tquery = append(tquery, bson.DocElem{"$lte", timeToStamp(req.Stop)})
	}
	if len(tquery) == 0 {
		query = bson.D{{"k", bson.D{{"$regex", regex}}}}
	} else {
		query = bson.D{{"k", bson.D{{"$regex", regex}}}, {"t", tquery}}
	}
	_, err = countersColl.Find(query).MapReduce(&job, &result)
	if err != nil {
		return nil, err
	}
	var counters []Counter
	for i := range result {
		key := result[i].Key
		when := time.Time{}
		if req.By != ByAll {
			var stamp int64
			if at := strings.Index(key, "@"); at != -1 && len(key) > at+1 {
				stamp, _ = strconv.ParseInt(key[at+1:], 10, 32)
				key = key[:at]
			}
			if stamp == 0 {
				return nil, fmt.Errorf("internal error: bad aggregated key: %q", result[i].Key)
			}
			switch req.By {
			case ByDay:
				stamp = stamp * 86400
			case ByWeek:
				// The +1 puts it at the end of the period.
				stamp = (stamp + 1) * 604800
			}
			when = time.Unix(counterEpoch+stamp, 0).In(time.UTC)
		}
		ids := strings.Split(key, ":")
		tokens := make([]string, 0, len(ids))
		for i := 0; i < len(ids)-1; i++ {
			if ids[i] == "*" {
				continue
			}
			id, err := strconv.ParseInt(ids[i], 32, 32)
			if err != nil {
				return nil, fmt.Errorf("store: invalid id: %q", ids[i])
			}
			token, found := s.statsIdToken(int(id))
			if !found {
				var t tokenId
				err = tokensColl.FindId(id).One(&t)
				if err == mgo.ErrNotFound {
					return nil, fmt.Errorf("store: internal error; token id not found: %d", id)
				}
				s.cacheStatsTokenId(t.Token, t.Id)
				token = t.Token
			}
			tokens = append(tokens, token)
		}
		counter := Counter{
			Key:    tokens,
			Prefix: len(ids) > 0 && ids[len(ids)-1] == "*",
			Count:  result[i].Value,
			Time:   when,
		}
		counters = append(counters, counter)
	}
	if !req.List && len(counters) == 0 {
		counters = []Counter{{Key: req.Key, Prefix: req.Prefix, Count: 0}}
	} else if len(counters) > 1 {
		sort.Sort(sortableCounters(counters))
	}
	return counters, nil
}

type sortableCounters []Counter

func (s sortableCounters) Len() int      { return len(s) }
func (s sortableCounters) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sortableCounters) Less(i, j int) bool {
	// Earlier times first.
	if !s[i].Time.Equal(s[j].Time) {
		return s[i].Time.Before(s[j].Time)
	}
	// Then larger counts first.
	if s[i].Count != s[j].Count {
		return s[j].Count < s[i].Count
	}
	// Then smaller/shorter keys first.
	ki := s[i].Key
	kj := s[j].Key
	for n := range ki {
		if n >= len(kj) {
			return false
		}
		if ki[n] != kj[n] {
			return ki[n] < kj[n]
		}
	}
	if len(ki) < len(kj) {
		return true
	}
	// Then full keys first.
	return !s[i].Prefix && s[j].Prefix
}

// A CharmPublisher is responsible for importing a charm dir onto the store.
type CharmPublisher struct {
	revision int
	w        *charmWriter
}

// Revision returns the revision that will be assigned to the published charm.
func (p *CharmPublisher) Revision() int {
	return p.revision
}

// CharmDir matches the part of the interface of *charm.Dir that is necessary
// to publish a charm. Using this interface rather than *charm.Dir directly
// makes testing some aspects of the store possible.
type CharmDir interface {
	Meta() *charm.Meta
	Config() *charm.Config
	SetRevision(revision int)
	BundleTo(w io.Writer) error
}

// Statically ensure that *charm.Dir is indeed a CharmDir.
var _ CharmDir = (*charm.Dir)(nil)

// Publish bundles charm and writes it to the store. The written charm
// bundle will have its revision set to the result of Revision.
// Publish must be called only once for a CharmPublisher.
func (p *CharmPublisher) Publish(charm CharmDir) error {
	w := p.w
	if w == nil {
		panic("CharmPublisher already published a charm")
	}
	p.w = nil
	w.charm = charm
	// TODO: Refactor to BundleTo(w, revision)
	charm.SetRevision(p.revision)
	err := charm.BundleTo(w)
	if err == nil {
		err = w.finish()
	} else {
		w.abort()
	}
	return err
}

// CharmPublisher returns a new CharmPublisher for importing a charm that
// will be made available in the store at all of the provided URLs.
// The digest parameter must contain the unique identifier that
// represents the charm data being imported (e.g. the VCS revision sha1).
// ErrRedundantUpdate is returned if all of the provided urls are
// already associated to that digest.
func (s *Store) CharmPublisher(urls []*charm.URL, digest string) (p *CharmPublisher, err error) {
	log.Infof("store: Trying to add charms %v with key %q...", urls, digest)
	if err = mustLackRevision("CharmPublisher", urls...); err != nil {
		return
	}
	session := s.session.Copy()
	defer session.Close()

	maxRev := -1
	newKey := false
	charms := session.Charms()
	doc := charmDoc{}
	for i := range urls {
		urlStr := urls[i].String()
		err = charms.Find(bson.D{{"urls", urlStr}}).Sort("-revision").One(&doc)
		if err == mgo.ErrNotFound {
			log.Infof("store: Charm %s not yet in the store.", urls[i])
			newKey = true
			continue
		}
		if doc.Digest != digest {
			log.Infof("store: Charm %s is out of date with revision key %q.", urlStr, digest)
			newKey = true
		}
		if err != nil {
			log.Errorf("store: Unknown error looking for charm %s: %s", urlStr, err)
			return
		}
		if doc.Revision > maxRev {
			maxRev = doc.Revision
		}
	}
	if !newKey {
		log.Infof("store: All charms have revision key %q. Nothing to update.", digest)
		err = ErrRedundantUpdate
		return
	}
	revision := maxRev + 1
	log.Infof("store: Preparing writer to add charms with revision %d.", revision)
	w := &charmWriter{
		store:    s,
		urls:     urls,
		revision: revision,
		digest:   digest,
	}
	return &CharmPublisher{revision, w}, nil
}

// charmWriter is an io.Writer that writes charm bundles to the charms GridFS.
type charmWriter struct {
	store    *Store
	session  *storeSession
	file     *mgo.GridFile
	sha256   hash.Hash
	charm    CharmDir
	urls     []*charm.URL
	revision int
	digest   string
}

// Write creates an entry in the charms GridFS when first called,
// and streams all written data into it.
func (w *charmWriter) Write(data []byte) (n int, err error) {
	if w.file == nil {
		w.session = w.store.session.Copy()
		w.file, err = w.session.CharmFS().Create("")
		if err != nil {
			log.Errorf("store: Failed to create GridFS file: %v", err)
			return 0, err
		}
		w.sha256 = sha256.New()
		log.Infof("store: Creating GridFS file with id %q...", w.file.Id().(bson.ObjectId).Hex())
	}
	_, err = w.sha256.Write(data)
	if err != nil {
		panic("hash.Hash should never error")
	}
	return w.file.Write(data)
}

// abort cancels the charm writing.
func (w *charmWriter) abort() {
	if w.file != nil {
		// Ignore error. Already aborting due to a preceding bad situation
		// elsewhere. This error is not important right now.
		_ = w.file.Close()
		w.session.Close()
	}
}

// finish completes the charm writing process and inserts the final metadata.
// After it completes the charm will be available for consumption.
func (w *charmWriter) finish() error {
	if w.file == nil {
		return nil
	}
	defer w.session.Close()
	id := w.file.Id()
	size := w.file.Size()
	err := w.file.Close()
	if err != nil {
		log.Errorf("store: Failed to close GridFS file: %v", err)
		return err
	}
	charms := w.session.Charms()
	sha256 := hex.EncodeToString(w.sha256.Sum(nil))
	charm := charmDoc{
		w.urls,
		w.revision,
		w.digest,
		sha256,
		size,
		id.(bson.ObjectId),
		w.charm.Meta(),
		w.charm.Config(),
	}
	if err = charms.Insert(&charm); err != nil {
		err = maybeConflict(err)
		log.Errorf("store: Failed to insert new revision of charm %v: %v", w.urls, err)
		return err
	}
	return nil
}

type CharmInfo struct {
	revision int
	digest   string
	sha256   string
	size     int64
	fileId   bson.ObjectId
	meta     *charm.Meta
	config   *charm.Config
}

// Statically ensure CharmInfo is a charm.Charm.
var _ charm.Charm = (*CharmInfo)(nil)

// Revision returns the store charm's revision.
func (ci *CharmInfo) Revision() int {
	return ci.revision
}

// BundleSha256 returns the sha256 checksum for the stored charm bundle.
func (ci *CharmInfo) BundleSha256() string {
	return ci.sha256
}

// BundleSize returns the size for the stored charm bundle.
func (ci *CharmInfo) BundleSize() int64 {
	return ci.size
}

// Digest returns the unique identifier that represents the charm
// data imported. This is typically set to the VCS revision digest.
func (ci *CharmInfo) Digest() string {
	return ci.digest
}

// Meta returns the charm.Meta details for the stored charm.
func (ci *CharmInfo) Meta() *charm.Meta {
	return ci.meta
}

// Config returns the charm.Config details for the stored charm.
func (ci *CharmInfo) Config() *charm.Config {
	return ci.config
}

// CharmInfo retrieves the CharmInfo value for the charm at url.
func (s *Store) CharmInfo(url *charm.URL) (info *CharmInfo, err error) {
	session := s.session.Copy()
	defer session.Close()

	log.Debugf("store: Retrieving charm info for %s", url)
	rev := url.Revision
	url = url.WithRevision(-1)

	charms := session.Charms()
	var cdoc charmDoc
	var qdoc interface{}
	if rev == -1 {
		qdoc = bson.D{{"urls", url}}
	} else {
		qdoc = bson.D{{"urls", url}, {"revision", rev}}
	}
	err = charms.Find(qdoc).Sort("-revision").One(&cdoc)
	if err != nil {
		log.Errorf("store: Failed to find charm %s: %v", url, err)
		return nil, ErrNotFound
	}
	info = &CharmInfo{
		cdoc.Revision,
		cdoc.Digest,
		cdoc.Sha256,
		cdoc.Size,
		cdoc.FileId,
		cdoc.Meta,
		cdoc.Config,
	}
	return info, nil
}

// OpenCharm opens for reading via rc the charm currently available at url.
// rc must be closed after dealing with it or resources will leak.
func (s *Store) OpenCharm(url *charm.URL) (info *CharmInfo, rc io.ReadCloser, err error) {
	log.Debugf("store: Opening charm %s", url)
	info, err = s.CharmInfo(url)
	if err != nil {
		return nil, nil, err
	}
	session := s.session.Copy()
	file, err := session.CharmFS().OpenId(info.fileId)
	if err != nil {
		log.Errorf("store: Failed to open GridFS file for charm %s: %v", url, err)
		session.Close()
		return nil, nil, err
	}
	rc = &reader{session, file}
	return
}

// DeleteCharm deletes the charm currently available at url.
func (s *Store) DeleteCharm(url *charm.URL) (info *CharmInfo, err error) {
	log.Debugf("store: Deleting charm %s", url)
	info, err = s.CharmInfo(url)
	if err != nil {
		return nil, err
	}
	session := s.session.Copy()
	defer session.Close()
	err = session.CharmFS().RemoveId(info.fileId)
	if err != nil {
		log.Errorf("store: Failed to delete GridFS file for charm %s: %v", url, err)
		return nil, err
	}
	err = session.Charms().Remove(bson.D{{"urls", url}})
	if err != nil {
		log.Errorf("store: Failed to delete metadata for charm %s: %v", url, err)
		return nil, err
	}
	return
}

type reader struct {
	session *storeSession
	file    *mgo.GridFile
}

// Read consumes data from the opened charm.
func (r *reader) Read(buf []byte) (n int, err error) {
	return r.file.Read(buf)
}

// Close closes the opened charm and frees associated resources.
func (r *reader) Close() error {
	err := r.file.Close()
	r.session.Close()
	return err
}

// charmDoc represents the document stored in MongoDB for a charm.
type charmDoc struct {
	URLs     []*charm.URL
	Revision int
	Digest   string
	Sha256   string
	Size     int64
	FileId   bson.ObjectId
	Meta     *charm.Meta
	Config   *charm.Config
}

// LockUpdates acquires a server-side lock for updating a single charm
// that is supposed to be made available in all of the provided urls.
// If the lock can't be acquired in any of the urls, an error will be
// immediately returned.
// In the usual case, any locking done is undone when an error happens,
// or when l.Unlock is called. If something else goes wrong, the locks
// will also expire after the period defined in UpdateTimeout.
func (s *Store) LockUpdates(urls []*charm.URL) (l *UpdateLock, err error) {
	session := s.session.Copy()
	keys := make([]string, len(urls))
	for i := range urls {
		keys[i] = urls[i].String()
	}
	sort.Strings(keys)
	l = &UpdateLock{keys, session.Locks(), bson.Now()}
	if err = l.tryLock(); err != nil {
		session.Close()
		return nil, err
	}
	return l, nil
}

// UpdateLock represents an acquired update lock over a set of charm URLs.
type UpdateLock struct {
	keys  []string
	locks *mgo.Collection
	time  time.Time
}

// Unlock removes the previously acquired server-side lock that prevents
// other processes from attempting to update a set of charm URLs.
func (l *UpdateLock) Unlock() {
	log.Debugf("store: Unlocking charms for future updates: %v", l.keys)
	defer l.locks.Database.Session.Close()
	for i := len(l.keys) - 1; i >= 0; i-- {
		// Using time below ensures only the proper lock is removed.
		// Can't do much about errors here. Locks will expire anyway.
		l.locks.Remove(bson.D{{"_id", l.keys[i]}, {"time", l.time}})
	}
}

// tryLock tries locking l.keys, one at a time, and succeeds only if it
// can lock all of them in order. The keys should be pre-sorted so that
// two-way conflicts can't happen. If any of the keys fail to be locked,
// and expiring the old lock doesn't work, tryLock undoes all previous
// locks and aborts with an error.
func (l *UpdateLock) tryLock() error {
	for i, key := range l.keys {
		log.Debugf("store: Trying to lock charm %s for updates...", key)
		doc := bson.D{{"_id", key}, {"time", l.time}}
		err := l.locks.Insert(doc)
		if err == nil {
			log.Debugf("store: Charm %s is now locked for updates.", key)
			continue
		}
		if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
			log.Debugf("store: Charm %s is locked. Trying to expire lock.", key)
			l.tryExpire(key)
			err = l.locks.Insert(doc)
			if err == nil {
				log.Debugf("store: Charm %s is now locked for updates.", key)
				continue
			}
		}
		// Couldn't lock everyone. Undo previous locks.
		for j := i - 1; j >= 0; j-- {
			// Using time below should be unnecessary, but it's an extra check.
			// Can't do anything about errors here. Lock will expire anyway.
			l.locks.Remove(bson.D{{"_id", l.keys[j]}, {"time", l.time}})
		}
		err = maybeConflict(err)
		log.Errorf("store: Can't lock charms %v for updating: %v", l.keys, err)
		return err
	}
	return nil
}

// tryExpire attempts to remove outdated locks from the database.
func (l *UpdateLock) tryExpire(key string) {
	// Ignore errors. If nothing happens the key will continue locked.
	l.locks.Remove(bson.D{{"_id", key}, {"time", bson.D{{"$lt", bson.Now().Add(-UpdateTimeout)}}}})
}

// maybeConflict returns an ErrUpdateConflict if err is a mgo
// insert conflict LastError, or err itself otherwise.
func maybeConflict(err error) error {
	if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
		return ErrUpdateConflict
	}
	return err
}

// storeSession wraps a mgo.Session ands adds a few convenience methods.
type storeSession struct {
	*mgo.Session
}

// Copy copies the storeSession and its underlying mgo session.
func (s *storeSession) Copy() *storeSession {
	return &storeSession{s.Session.Copy()}
}

// Charms returns the mongo collection where charms are stored.
func (s *storeSession) Charms() *mgo.Collection {
	return s.DB("juju").C("charms")
}

// Charms returns a mgo.GridFS to read and write charms.
func (s *storeSession) CharmFS() *mgo.GridFS {
	return s.DB("juju").GridFS("charmfs")
}

// Events returns the mongo collection where charm events are stored.
func (s *storeSession) Events() *mgo.Collection {
	return s.DB("juju").C("events")
}

// Locks returns the mongo collection where charm locks are stored.
func (s *storeSession) Locks() *mgo.Collection {
	return s.DB("juju").C("locks")
}

// StatTokens returns the mongo collection for storing key tokens
// for statistics collection.
func (s *storeSession) StatTokens() *mgo.Collection {
	return s.DB("juju").C("stat.tokens")
}

// StatCounters returns the mongo collection for counter values.
func (s *storeSession) StatCounters() *mgo.Collection {
	return s.DB("juju").C("stat.counters")
}

type CharmEventKind int

const (
	EventPublished CharmEventKind = iota + 1
	EventPublishError

	EventKindCount
)

func (k CharmEventKind) String() string {
	switch k {
	case EventPublished:
		return "published"
	case EventPublishError:
		return "publish-error"
	}
	panic(fmt.Errorf("unknown charm event kind %d", k))
}

// CharmEvent is a record for an event relating to one or more charm URLs.
type CharmEvent struct {
	Kind     CharmEventKind
	Digest   string
	Revision int
	URLs     []*charm.URL
	Errors   []string `bson:",omitempty"`
	Warnings []string `bson:",omitempty"`
	Time     time.Time
}

// LogCharmEvent records an event related to one or more charm URLs.
func (s *Store) LogCharmEvent(event *CharmEvent) (err error) {
	log.Infof("store: Adding charm event for %v with key %q: %s", event.URLs, event.Digest, event.Kind)
	if err = mustLackRevision("LogCharmEvent", event.URLs...); err != nil {
		return
	}
	session := s.session.Copy()
	defer session.Close()
	if event.Kind == 0 || event.Digest == "" || len(event.URLs) == 0 {
		return fmt.Errorf("LogCharmEvent: need valid Kind, Digest and URLs")
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	events := session.Events()
	return events.Insert(event)
}

// CharmEvent returns the most recent event associated with url
// and digest.  If the specified event isn't found the error
// ErrUnknownChange will be returned.  If digest is empty, any
// digest will match.
func (s *Store) CharmEvent(url *charm.URL, digest string) (*CharmEvent, error) {
	// TODO: It'd actually make sense to find the charm event after the
	// revision id, but since we don't care about that now, just make sure
	// we don't write bad code.
	if err := mustLackRevision("CharmEvent", url); err != nil {
		return nil, err
	}
	session := s.session.Copy()
	defer session.Close()

	events := session.Events()
	event := &CharmEvent{Digest: digest}
	var query *mgo.Query
	if digest == "" {
		query = events.Find(bson.D{{"urls", url}})
	} else {
		query = events.Find(bson.D{{"urls", url}, {"digest", digest}})
	}
	err := query.Sort("-time").One(&event)
	if err == mgo.ErrNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return event, nil
}

// mustLackRevision returns an error if any of the urls has a revision.
func mustLackRevision(context string, urls ...*charm.URL) error {
	for _, url := range urls {
		if url.Revision != -1 {
			err := fmt.Errorf("%s: got charm URL with revision: %s", context, url)
			log.Errorf("store: %v", err)
			return err
		}
	}
	return nil
}
