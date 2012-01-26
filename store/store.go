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
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/log"
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
	"sort"
	"time"
)

// The following MongoDB collections are currently used:
//
//     juju.events    - Events relative to the lifecycle of the charm
//     juju.charms    - Information about the stored charms
//     juju.charmfs.* - GridFS with the charm files
//     juju.locks     - Has unique keys with url of updating charms

var (
	ErrUpdateConflict  = errors.New("charm update in progress")
	ErrRedundantUpdate = errors.New("charm is up-to-date")
	ErrNotFound        = errors.New("entry not found")
)

const (
	UpdateTimeout = 600e9
)

// Store holds a connection to a charm store.
type Store struct {
	session *storeSession
}

// Open creates a new session with the store. It connects to the MongoDB
// server at the given address (as expected by the Mongo function in the
// launchpad.net/mgo package).
func Open(mongoAddr string) (store *Store, err error) {
	log.Printf("Store opened. Connecting to: %s", mongoAddr)
	store = &Store{}
	session, err := mgo.Dial(mongoAddr)
	if err != nil {
		log.Printf("Error connecting to MongoDB: %v", err)
		return nil, err
	}
	store = &Store{&storeSession{session}}
	charms := store.session.Charms()
	err = charms.EnsureIndex(mgo.Index{Key: []string{"url", "revision"}, Unique: true})
	if err != nil {
		log.Printf("Error ensuring charms index: %v", err)
		session.Close()
		return nil, err
	}
	// Put the socket we used on EnsureIndex back in the pool.
	session.Refresh()
	return store, nil
}

// Close terminates the connection with the store.
func (s *Store) Close() {
	s.session.Close()
}

// AddCharm prepares the store to have charm added to it at all of
// the provided urls. The revisionKey parameter must contain the
// unique identifier that represents the current charm content
// (e.g. the VCS revision sha1). ErrRedundantUpdate is returned if
// all of the provided urls are already associated to that revision key.
//
// On success, wc must be used to stream the charm bundle onto the
// store, and once wc is closed successfully the content will be
// available at all of the provided urls, and a new CharmEvent
// saved to record the successful publishing atomically (it doesn't
// get saved if there are any errors).
//
// The returned revision number will be assigned to the charm in the
// store. The revision will be the maximum current revision from
// all of the urls plus one. It is the caller's responsibility to ensure
// that the charm content written to wc represents that revision.
func (s *Store) AddCharm(charm charm.Charm, urls []*charm.URL, revisionKey string) (wc io.WriteCloser, revision int, err error) {
	log.Printf("Trying to add charms %v with key %q...", urls, revisionKey)
	if err = mustLackRevision("AddCharm", urls...); err != nil {
		return
	}
	session := s.session.Copy()
	lock, err := session.LockUpdates(urls)
	if err != nil {
		session.Close()
		return
	}
	defer func() {
		if err != nil {
			lock.Unlock()
			session.Close()
		}
	}()

	maxRev := -1
	newKey := false
	charms := session.Charms()
	doc := charmDoc{}
	for i := range urls {
		urlStr := urls[i].String()
		err = charms.Find(bson.D{{"url", urlStr}}).Sort(bson.D{{"revision", -1}}).One(&doc)
		if err == mgo.NotFound {
			log.Printf("Charm %s not yet in the store.", urlStr)
			newKey = true
			continue
		}
		if doc.RevisionKey != revisionKey {
			log.Printf("Charm %s is out of date with revision key %q.", urlStr, revisionKey)
			newKey = true
		}
		if err != nil {
			log.Printf("Unknown error looking for charm %s: %s", urlStr, err)
			return
		}
		if doc.Revision > maxRev {
			maxRev = doc.Revision
		}
	}
	if !newKey {
		log.Printf("All charms have revision key %q. Nothing to update.", revisionKey)
		err = ErrRedundantUpdate
		return
	}
	revision = maxRev + 1
	log.Printf("Preparing writer to add charms with revision %d.", revision)
	w := &writer{
		store:    s,
		session:  session,
		charm:    charm,
		urls:     urls,
		lock:     lock,
		revision: revision,
		revKey:   revisionKey,
	}
	return w, revision, nil
}

type writer struct {
	store    *Store
	session  *storeSession
	file     *mgo.GridFile
	sha256   hash.Hash
	charm    charm.Charm
	urls     []*charm.URL
	lock     *updateMutex
	revision int
	revKey   string
}

// Write appends data to the charm bundle payload in the store.
func (w *writer) Write(data []byte) (n int, err error) {
	if w.file == nil {
		w.file, err = w.session.CharmFS().Create("")
		if err != nil {
			log.Printf("Failed to create GridFS file: %v", err)
			return 0, err
		}
		w.sha256 = sha256.New()
		log.Printf("Creating GridFS file with id %q...", w.file.Id().(bson.ObjectId).Hex())
	}
	_, err = w.sha256.Write(data)
	if err != nil {
		panic("hash.Hash should never error")
	}
	return w.file.Write(data)
}

// Close finishes the charm writing process and inserts the final metadata.
// After it completes the charm will be available for consumption.
func (w *writer) Close() error {
	defer w.session.Close()
	defer w.lock.Unlock()
	if w.file == nil {
		return nil
	}
	id := w.file.Id()
	err := w.file.Close()
	if err != nil {
		log.Printf("Failed to close GridFS file: %v", err)
		return err
	}
	charms := w.session.Charms()
	sha256 := hex.EncodeToString(w.sha256.Sum(nil))
	for _, url := range w.urls {
		urlStr := url.String()
		charm := charmDoc{
			urlStr,
			w.revision,
			w.revKey,
			sha256,
			id.(bson.ObjectId),
			w.charm.Meta(),
			w.charm.Config(),
		}
		err := charms.Insert(&charm)
		if err != nil {
			err = maybeConflict(err)
			log.Printf("Failed to insert new revision of charm %s: %v", urlStr, err)
			return err
		}
	}
	status := &CharmEvent{
		Kind:        EventPublishDone,
		RevisionKey: w.revKey,
		URLs:        w.urls,
	}
	return w.store.logCharmEvent(status, w.session, false)
}

type CharmInfo struct {
	revision int
	sha256   string
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

// Revision returns the sha256 checksum for the stored charm bundle.
func (ci *CharmInfo) Sha256() string {
	return ci.sha256
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

	log.Debugf("Retrieving charm info for %s", url)
	rev := url.Revision
	url = url.WithRevision(-1)

	charms := session.Charms()
	var cdoc charmDoc
	var qdoc interface{}
	if rev == -1 {
		qdoc = bson.D{{"url", url.String()}}
	} else {
		qdoc = bson.D{{"url", url.String()}, {"revision", rev}}
	}
	err = charms.Find(qdoc).Sort(bson.D{{"revision", -1}}).One(&cdoc)
	if err != nil {
		log.Printf("Failed to find charm %s: %v", url, err)
		return nil, ErrNotFound
	}
	return &CharmInfo{cdoc.Revision, cdoc.Sha256, cdoc.FileId, cdoc.Meta, cdoc.Config}, nil
}

// OpenCharm opens for reading via rc the charm currently available at url.
// rc must be closed after dealing with it or resources will leak.
func (s *Store) OpenCharm(url *charm.URL) (info *CharmInfo, rc io.ReadCloser, err error) {
	log.Debugf("Opening charm %s", url)
	info, err = s.CharmInfo(url)
	if err != nil {
		return nil, nil, err
	}
	session := s.session.Copy()
	file, err := session.CharmFS().OpenId(info.fileId)
	if err != nil {
		log.Printf("Failed to open GridFS file for charm %s: %v", url, err)
		session.Close()
		return nil, nil, err
	}
	rc = &reader{session, file}
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

// charmDoc represents the document stored in MongoDB for a single charm revision.
type charmDoc struct {
	URL         string
	Revision    int
	RevisionKey string
	Sha256      string
	FileId      bson.ObjectId
	Meta        *charm.Meta
	Config      *charm.Config
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

// LockUpdates acquires a server-side lock for updating a single charm
// that is supposed to be made available in all of the provided urls.
// If the lock can't be acquired in any of the urls, an error will be
// immediately returned.
// In the usual case, any locking done is undone when an error happens,
// or when m.Unlock is called. If something else goes wrong, the locks
// will also expire after the period defined in UpdateTimeout.
func (s *storeSession) LockUpdates(urls []*charm.URL) (m *updateMutex, err error) {
	keys := make([]string, len(urls))
	for i := range urls {
		keys[i] = urls[i].String()
	}
	sort.Strings(keys)
	m = &updateMutex{keys, s.DB("juju").C("locks"), bson.Now()}
	err = m.tryLock()
	if err != nil {
		return nil, err
	}
	return m, nil
}

// updateMutex manages the logic around locking and is used
// via storeSession.LockUpdates.
type updateMutex struct {
	keys  []string
	locks *mgo.Collection
	time  bson.Timestamp
}

// Unlock removes previously acquired locks on all m.keys.
func (m *updateMutex) Unlock() {
	log.Debugf("Unlocking charms for future updates: %v", m.keys)
	for i := len(m.keys) - 1; i >= 0; i-- {
		// Using time below ensures only the proper lock is removed.
		// Can't do much about errors here. Locks will expire anyway.
		m.locks.Remove(bson.D{{"_id", m.keys[i]}, {"time", m.time}})
	}
}

// tryLock tries locking m.keys, one at a time, and succeeds only if it
// can lock all of them in order. The keys should be pre-sorted so that
// two-way conflicts can't happen. If any of the keys fail to be locked,
// and expiring the old lock doesn't work, tryLock undoes all previous
// locks and aborts with an error.
func (m *updateMutex) tryLock() error {
	for i, key := range m.keys {
		log.Debugf("Trying to lock charm %s for updates...", key)
		doc := bson.D{{"_id", key}, {"time", m.time}}
		err := m.locks.Insert(doc)
		if err == nil {
			log.Debugf("Charm %s is now locked for updates.", key)
			continue
		}
		if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
			log.Debugf("Charm %s is locked. Trying to expire lock.", key)
			m.tryExpire(key)
			err = m.locks.Insert(doc)
			if err == nil {
				log.Debugf("Charm %s is now locked for updates.", key)
				continue
			}
		}
		// Couldn't lock everyone. Undo previous locks.
		for j := i - 1; j >= 0; j-- {
			// Using time below should be unnecessary, but it's an extra check.
			// Can't do anything about errors here. Lock will expire anyway.
			m.locks.Remove(bson.D{{"_id", m.keys[j]}, {"time", m.time}})
		}
		err = maybeConflict(err)
		log.Printf("Can't lock charms %v for updating: %v", m.keys, err)
		return err
	}
	return nil
}

// tryExpire attempts to remove outdated locks from the database.
func (m *updateMutex) tryExpire(key string) {
	// Ignore errors. If nothing happens the key will continue locked.
	m.locks.Remove(bson.D{{"_id", key}, {"time", bson.D{{"$lt", bson.Now() - UpdateTimeout}}}})
}

// maybeConflict returns an ErrUpdateConflict if err is a mgo
// insert conflict LastError, or err itself otherwise.
func maybeConflict(err error) error {
	if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
		return ErrUpdateConflict
	}
	return err
}

type CharmEventKind int

const (
	EventPublishStarted CharmEventKind = iota + 1
	EventPublishFailed
	EventPublishDone
)

// CharmEvent is a record for an event relative to one or more charm URLs.
type CharmEvent struct {
	Kind        CharmEventKind
	RevisionKey string
	Revision    int
	URLs        []*charm.URL
	Errors      []string
	Warnings    []string
	Time        time.Time
}

// eventDoc is a shadow of CharmEvent for marshalling purposes.
type eventDoc struct {
	Kind        CharmEventKind
	RevisionKey string
	Revision    *int ",omitempty"
	URLs        []string
	Errors      []string ",omitempty"
	Warnings    []string ",omitempty"
	// TODO: Fix bson time.Time marshalling.
	Time bson.Timestamp
}

// LogCharmEvent records an event related to one or more charm URLs.
func (s *Store) LogCharmEvent(event *CharmEvent) (err error) {
	session := s.session.Copy()
	defer session.Close()
	return s.logCharmEvent(event, session, true)
}

// addCharmEvent is the implementation of LogCharmEvent, and allows
// logging an event with a pre-established session and without locking
// the store first.
func (s *Store) logCharmEvent(event *CharmEvent, session *storeSession, lockUrls bool) (err error) {
	log.Printf("Adding charm event for %v with key %q: %s", event.URLs, event.RevisionKey, event.Kind)
	if err = mustLackRevision("LogCharmEvent", event.URLs...); err != nil {
		return
	}
	if event.Kind == 0 || event.RevisionKey == "" || len(event.URLs) == 0 {
		return fmt.Errorf("LogCharmEvent: need valid Kind, RevisionKey and URLs")
	}
	if lockUrls {
		lock, err := session.LockUpdates(event.URLs)
		if err != nil {
			return err
		}
		defer lock.Unlock()
	}
	urlStrs := make([]string, len(event.URLs))
	for i, url := range event.URLs {
		urlStrs[i] = url.String()
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	doc := &eventDoc{
		Kind:        event.Kind,
		URLs:        urlStrs,
		RevisionKey: event.RevisionKey,
		Errors:      event.Errors,
		Warnings:    event.Warnings,
		Time:        bson.Timestamp(event.Time.UnixNano()),
	}
	if event.Kind == EventPublishDone {
		doc.Revision = &event.Revision
	}
	events := session.Events()
	return events.Insert(doc)
}

// CharmEvent returns the attempted event associated with url
// and revisionKey.  If the specified event isn't found the
// error ErrUnknownChange will be returned.
func (s *Store) CharmEvent(url *charm.URL, revisionKey string) (*CharmEvent, error) {
	// TODO: It'd actually make sense to find the charm event after the
	// revision id, but since we don't care about that now, just make sure
	// we don't write bad code.
	if err := mustLackRevision("CharmEvent", url); err != nil {
		return nil, err
	}
	session := s.session.Copy()
	defer session.Close()

	events := session.Events()
	doc := eventDoc{}
	err := events.Find(bson.D{{"urls", url.String()}, {"revisionkey", revisionKey}}).One(&doc)
	if err == mgo.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	urls := make([]*charm.URL, len(doc.URLs))
	for i, url := range doc.URLs {
		urls[i], err = charm.ParseURL(url)
		if err != nil {
			return nil, err
		}
	}
	event := &CharmEvent{
		Kind:        doc.Kind,
		RevisionKey: revisionKey,
		URLs:        urls,
		Errors:      doc.Errors,
		Warnings:    doc.Warnings,
		Time:        time.Unix(0, int64(doc.Time)),
	}
	if doc.Revision != nil {
		event.Revision = *doc.Revision
	}
	return event, nil
}

// mustLackRevision returns an error if any of the urls has a revision.
func mustLackRevision(context string, urls ...*charm.URL) error {
	for _, url := range urls {
		if url.Revision != -1 {
			err := fmt.Errorf("%s: got charm URL with revision: %s", context, url)
			log.Printf("%v", err)
			return err
		}
	}
	return nil
}
