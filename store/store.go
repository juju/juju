package store

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"launchpad.net/gobson/bson"
	"launchpad.net/juju/go/charm"
	"launchpad.net/mgo"
	"os"
	"sort"
)

// TODO:
// - Document it.
// - Add meta and config information.

const (
	UpdateTimeout = 600e9
)

var (
	UpdateConflict  = os.NewError("charm update already in progress")
	UpdateIsCurrent = os.NewError("charm is already up-to-date")
)

type Store struct {
	session storeSession
}

func New(mongoAddr string) (store *Store, err os.Error) {
	logf("New store created. Connecting to: %s", mongoAddr)
	store = &Store{}
	session, err := mgo.Mongo(mongoAddr)
	if err != nil {
		logf("Error connecting to MongoDB: %s", err)
		return nil, err
	}
	store = &Store{session: storeSession{session}}
	charms := store.session.Charms()
	err = charms.EnsureIndex(mgo.Index{Key: []string{"url", "revision"}, Unique: true})
	if err != nil {
		logf("Error ensuring charms index: %s", err)
		session.Close()
		return nil, err
	}
	// Put the socket we used on EnsureIndex back in the pool.
	session.Refresh()
	return store, nil
}

func (s *Store) Close() {
	s.session.Close()
}

func dropRevisions(urls []*charm.URL) []*charm.URL {
	norev := make([]*charm.URL, len(urls))
	for i := range norev {
		norev[i] = urls[i].WithRevision(-1)
	}
	return norev
}

func (s *Store) AddCharm(urls []*charm.URL, revKey string) (wc io.WriteCloser, err os.Error) {
	logf("Trying to add charms %v with key %q...", urls, revKey)
	urls = dropRevisions(urls)
	session := s.session.Copy()
	lock, err := session.LockUpdates(urls)
	if err != nil {
		session.Close()
		return nil, err
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
	charm := charmDoc{}
	for i := range urls {
		urlStr := urls[i].String()
		err := charms.Find(bson.D{{"url", urlStr}}).Sort(bson.D{{"revision", -1}}).One(&charm)
		if err == mgo.NotFound {
			logf("Charm %s not yet in the store.", urlStr)
			newKey = true
			continue
		}
		if charm.RevisionKey != revKey {
			logf("Charm %s is out of date with revision key %q.", urlStr, revKey)
			newKey = true
		}
		if err != nil {
			logf("Unknown error looking for charm %s: %s", urlStr, err)
			return nil, err
		}
		if charm.Revision > maxRev {
			maxRev = charm.Revision
		}
	}
	if !newKey {
		logf("All charms have revision key %q. Nothing to update.", revKey)
		return nil, UpdateIsCurrent
	}
	logf("Preparing writer to add charms with revision %d.", maxRev+1)
	return &writer{session, nil, nil, urls, lock, maxRev + 1, revKey}, nil
}

type writer struct {
	session  storeSession
	file     *mgo.GridFile
	sha256   hash.Hash
	urls     []*charm.URL
	lock     *updateMutex
	revision int
	revKey   string
}

func (w *writer) Write(data []byte) (n int, err os.Error) {
	if w.file == nil {
		w.file, err = w.session.CharmFS().Create("")
		if err != nil {
			logf("Failed to create GridFS file: %s", err)
			return 0, err
		}
		w.sha256 = sha256.New()
		logf("Creating GridFS file with id %q...", w.file.Id().(bson.ObjectId).Hex())
	}
	_, err = w.sha256.Write(data)
	if err != nil {
		panic("hash.Hash should never error")
	}
	return w.file.Write(data)
}

func (w *writer) Close() os.Error {
	defer w.session.Close()
	defer w.lock.Unlock()
	if w.file == nil {
		return nil
	}
	id := w.file.Id()
	err := w.file.Close()
	if err != nil {
		logf("Failed to close GridFS file: %s", err)
		return err
	}
	charms := w.session.Charms()
	for _, url := range w.urls {
		urlStr := url.String()
		charm := charmDoc{
			urlStr,
			w.revision,
			w.revKey,
			hex.EncodeToString(w.sha256.Sum()),
			id.(bson.ObjectId),
		}
		err := charms.Insert(&charm)
		if err != nil {
			err = maybeConflict(err)
			logf("Failed to insert new revision of charm %s: %s", urlStr, err)
			return err
		}
	}
	return nil
}

type CharmInfo struct {
	Revision int
	Sha256   string
}

func (s *Store) OpenCharm(url *charm.URL) (rc io.ReadCloser, info *CharmInfo, err os.Error) {
	session := s.session.Copy()

	debugf("Opening charm %s.", url)
	rev := url.Revision
	url = url.WithRevision(-1)

	charms := session.Charms()
	var charm charmDoc
	var qdoc interface{}
	if rev == -1 {
		qdoc = bson.D{{"url", url.String()}}
	} else {
		qdoc = bson.D{{"url", url.String()}, {"revision", rev}}
	}
	err = charms.Find(qdoc).Sort(bson.D{{"revision", -1}}).One(&charm)
	if err != nil {
		logf("Failed to find charm %s: %s", url, err)
		session.Close()
		return nil, nil, err
	}

	file, err := session.CharmFS().OpenId(charm.FileID)
	if err != nil {
		logf("Failed to open GridFS file for charm %s: %s", url, err)
		session.Close()
		return nil, nil, err
	}
	return &reader{session, file}, &CharmInfo{charm.Revision, charm.Sha256}, nil
}

type reader struct {
	session storeSession
	file    *mgo.GridFile
}

func (r *reader) Read(buf []byte) (n int, err os.Error) {
	return r.file.Read(buf)
}

func (r *reader) Close() os.Error {
	err := r.file.Close()
	r.session.Close()
	return err
}

type charmDoc struct {
	URL         string
	Revision    int
	RevisionKey string
	Sha256      string
	FileID      bson.ObjectId
}

type storeSession struct {
	*mgo.Session
}

func (s storeSession) Copy() storeSession {
	return storeSession{s.Session.Copy()}
}

func (s storeSession) Charms() mgo.Collection {
	return s.DB("juju").C("charms")
}

func (s storeSession) CharmFS() *mgo.GridFS {
	return s.DB("juju").GridFS("charmfs")
}

func (s storeSession) LockUpdates(urls []*charm.URL) (m *updateMutex, err os.Error) {
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

type updateMutex struct {
	keys  []string
	locks mgo.Collection
	time  bson.Timestamp
}

func (m *updateMutex) Unlock() {
	debugf("Unlocking charms for future updates: %v", m.keys)
	for i := len(m.keys) - 1; i >= 0; i-- {
		// Using time below ensures only the proper lock is removed.
		// Can't do much about errors here. Locks will expire anyway.
		m.locks.Remove(bson.D{{"_id", m.keys[i]}, {"time", m.time}})
	}
}

func (m *updateMutex) tryLock() os.Error {
	for i, key := range m.keys {
		debugf("Trying to lock charm %s for updates...", key)
		doc := bson.D{{"_id", key}, {"time", m.time}}
		err := m.locks.Insert(doc)
		if err == nil {
			debugf("Charm %s is now locked for updates.", key)
			continue
		}
		if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
			debugf("Charm %s is locked. Trying to expire lock.", key)
			m.tryExpire(key)
			err = m.locks.Insert(doc)
			if err == nil {
				debugf("Charm %s is now locked for updates.", key)
				continue
			}
		}
		// Couldn't lock everyone. Undo previous locks.
		for j := i-1; j >= 0; j-- {
			// Using time below should be unnecessary, but it's an extra check.
			// Can't do anything about errors here. Lock will expire anyway.
			m.locks.Remove(bson.D{{"_id", m.keys[j]}, {"time", m.time}})
		}
		err = maybeConflict(err)
		logf("Can't lock charms %v for updating: %s", m.keys, err)
		return err
	}
	return nil
}

func (m *updateMutex) tryExpire(key string) {
	// Ignore errors. If nothing happens the key will continue locked.
	m.locks.Remove(bson.D{{"_id", key}, {"time", bson.D{{"$lt", bson.Now() - UpdateTimeout}}}})
}

func maybeConflict(err os.Error) os.Error {
	if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
		return UpdateConflict
	}
	return err
}
