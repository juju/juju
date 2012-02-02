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
//     juju.events    - Log of events relating to the lifecycle of charms
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

// A CharmPublisher is responsible for importing a charm bundle onto the store.
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

// Publish bundles charm and writes it to the store. It will also log events
// recording the success or failure of the operation.
// The written charm bundle will have the revision returned by Revision.
// Publish must be called only once for a CharmPublisher.
func (p *CharmPublisher) Publish(charm CharmDir) error {
	w := p.w
	if w == nil {
		panic("CharmPublisher already published a charm")
	}
	p.w = nil
	w.charm = charm
	event := &CharmEvent{
		Kind:     EventPublished,
		Digest:   w.digest,
		URLs:     w.urls,
		Revision: w.revision, // TESTME
	}
	// TODO: Refactor to BundleTo(w, revision)
	charm.SetRevision(p.revision)
	err := charm.BundleTo(w)
	if err == nil {
		err = w.finish()
	}
	if err != nil {
		w.abort()
		event.Kind = EventPublishError
		event.Errors = []string{err.Error()}
	}
	logErr := w.store.LogCharmEvent(event)
	if err == nil {
		return logErr
	}
	return err
}

// CharmPublisher returns a new CharmPublisher for importing a charm that
// must be made available in the store at all of the provided URLs.
// The digest parameter must contain the unique identifier that
// represents the current charm content (e.g. the VCS revision sha1).
// ErrRedundantUpdate is returned if all of the provided urls are
// already associated to that digest.
func (s *Store) CharmPublisher(urls []*charm.URL, digest string) (p *CharmPublisher, err error) {
	log.Printf("Trying to add charms %v with key %q...", urls, digest)
	if err = mustLackRevision("CharmPublisher", urls...); err != nil {
		return
	}
	session := s.session.Copy()
	defer func() {
		if err != nil {
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
		if doc.Digest != digest {
			log.Printf("Charm %s is out of date with revision key %q.", urlStr, digest)
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
		log.Printf("All charms have revision key %q. Nothing to update.", digest)
		err = ErrRedundantUpdate
		return
	}
	revision := maxRev + 1
	log.Printf("Preparing writer to add charms with revision %d.", revision)
	w := &charmWriter{
		store:    s,
		session:  session,
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

// abort cancels the charm writing.
func (w *charmWriter) abort() {
	if w.file != nil {
		// Ignore error. Already aborting due to a preceding bad situation
		// elsewhere. This error is not important right now.
		_ = w.file.Close()
	}
	w.session.Close()
}

// finish completes the charm writing process and inserts the final metadata.
// After it completes the charm will be available for consumption.
func (w *charmWriter) finish() error {
	defer w.session.Close()
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
			w.digest,
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
	event := &CharmEvent{
		Kind:   EventPublished,
		Digest: w.digest,
		// Revision: w.revision, TESTME
		URLs: w.urls,
	}
	return w.store.LogCharmEvent(event)
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
	URL      string
	Revision int
	Digest   string
	Sha256   string
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
	log.Debugf("Unlocking charms for future updates: %v", l.keys)
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
		log.Debugf("Trying to lock charm %s for updates...", key)
		doc := bson.D{{"_id", key}, {"time", l.time}}
		err := l.locks.Insert(doc)
		if err == nil {
			log.Debugf("Charm %s is now locked for updates.", key)
			continue
		}
		if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
			log.Debugf("Charm %s is locked. Trying to expire lock.", key)
			l.tryExpire(key)
			err = l.locks.Insert(doc)
			if err == nil {
				log.Debugf("Charm %s is now locked for updates.", key)
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
		log.Printf("Can't lock charms %v for updating: %v", l.keys, err)
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

type CharmEventKind int

const (
	EventPublished CharmEventKind = iota + 1
	EventPublishError
)

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
	log.Printf("Adding charm event for %v with key %q: %s", event.URLs, event.Digest, event.Kind)
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
// ErrUnknownChange will be returned.
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
	query := events.Find(bson.D{{"urls", url.String()}, {"digest", digest}})
	query.Sort(bson.D{{"time", -1}})
	err := query.One(&event)
	if err == mgo.NotFound {
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
			log.Printf("%v", err)
			return err
		}
	}
	return nil
}
