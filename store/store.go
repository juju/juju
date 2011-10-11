package store

import (
	"io"
	"launchpad.net/gobson/bson"
	"launchpad.net/juju/go/charm"
	"launchpad.net/mgo"
	"os"
	"sort"
)

type Store struct {
	session storeSession
}

func New(mongoAddr string) (store *Store, err os.Error) {
	store = &Store{}
	session, err := mgo.Mongo(mongoAddr)
	if err != nil {
		return nil, err
	}
	store = &Store{session: storeSession{session}}
	charms := store.session.Charms()
	err = charms.EnsureIndex(mgo.Index{Key: []string{"url", "revision"}, Unique: true})
	if err != nil {
		session.Close()
		return nil, err
	}
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

func (s *Store) AddCharm(urls []*charm.URL) (wc io.WriteCloser, err os.Error) {
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

	maxrev := -1
	charms := session.Charms()
	err = charms.EnsureIndexKey([]string{"url", "revision"})
	if err != nil {
		return nil, err
	}
	charm := infoDoc{}
	for i := range urls {
		err := charms.Find(bson.D{{"url", urls[i].String()}}).Sort(bson.D{{"revision", -1}}).One(&charm)
		if err == mgo.NotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		if charm.Revision > maxrev {
			maxrev = charm.Revision
		}
	}
	return &writer{session, nil, urls, lock, maxrev+1}, nil
}

type writer struct {
	session storeSession
	file *mgo.GridFile
	urls []*charm.URL
	lock *updateMutex
	revision int
}

func (w *writer) Write(data []byte) (n int, err os.Error) {
	if w.file == nil {
		w.file, err = w.session.CharmFS().Create("")
		if err != nil {
			return 0, err
		}
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
		return err
	}
	charms := w.session.Charms()
	for _, url := range w.urls {
		err := charms.Insert(&infoDoc{url.String(), w.revision, id.(bson.ObjectId)})
		if err != nil {
			return maybeConflict(err)
		}
	}
	return nil
}

type CharmInfo struct {
	Revision int
}

func (s *Store) OpenCharm(url *charm.URL) (rc io.ReadCloser, info *CharmInfo, err os.Error) {
	session := s.session.Copy()

	rev := url.Revision
	url = url.WithRevision(-1)

	charms := session.Charms()
	var charm infoDoc
	var qdoc interface{}
	if rev == -1 {
		qdoc = bson.D{{"url", url.String()}}
	} else {
		qdoc = bson.D{{"url", url.String()}, {"revision", rev}}
	}
	err = charms.Find(qdoc).Sort(bson.D{{"revision", -1}}).One(&charm)
	if err != nil {
		session.Close()
		return nil, nil, err
	}

	file, err := session.CharmFS().OpenId(charm.FileID)
	if err != nil {
		session.Close()
		return nil, nil, err
	}
	return &reader{session, file}, &CharmInfo{charm.Revision}, nil
}

type reader struct {
	session storeSession
	file *mgo.GridFile
}

func (r *reader) Read(buf []byte) (n int, err os.Error) {
	return r.file.Read(buf)
}

func (r *reader) Close() os.Error {
	err := r.file.Close()
	r.session.Close()
	return err
}

type infoDoc struct {
	URL string
	Revision int
	FileID bson.ObjectId
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
	keys []string
	locks mgo.Collection
	time bson.Timestamp
}

func (m *updateMutex) Unlock() {
	for i := len(m.keys)-1; i >= 0; i-- {
		// Using time ensures only the proper lock is removed.
		m.locks.Remove(bson.D{{"_id", m.keys[i]}, {"time", m.time}})
	}
}

func (m *updateMutex) tryLock() os.Error {
	for i := range m.keys {
		doc := bson.D{{"_id", m.keys[i]}, {"time", m.time}}
		err := m.locks.Insert(doc)
		if err == nil {
			continue
		}
		if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
			// It's locked. Try to expire the lock and try again.
			m.tryExpire(m.keys[i])
			err = m.locks.Insert(doc)
			if err == nil {
				continue
			}
		}
		// Couldn't lock everyone. Undo previous locks.
		for i--; i >= 0; i-- {
			// Can't do anything about errors here. They'll expire anyway.
			// Using time should be unnecessary, but it's an extra check.
			m.locks.Remove(bson.D{{"_id", m.keys[i]}, {"time", m.time}})
		}
		return maybeConflict(err)
	}
	return nil
}

const UpdateTimeout = 600e9

func (m *updateMutex) tryExpire(key string) {
	// Ignore errors. If nothing happens the key will continue locked.
	m.locks.Remove(bson.D{{"_id", key}, {"time", bson.D{{"$lt", bson.Now()-UpdateTimeout}}}})
}

var UpdateConflict = os.NewError("update already in progress")

func maybeConflict(err os.Error) os.Error {
	if lerr, ok := err.(*mgo.LastError); ok && lerr.Code == 11000 {
		return UpdateConflict
	}
	return err
}
