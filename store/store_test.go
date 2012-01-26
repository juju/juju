package store_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/store"
	"launchpad.net/mgo/bson"
	"path/filepath"
	"testing"
	"time"
)

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&S{})

type S struct {
	MgoSuite
	store *store.Store
	charm *charm.Dir
}

func (s *S) SetUpTest(c *C) {
	s.MgoSuite.SetUpTest(c)
	var err error
	s.store, err = store.Open(s.Addr)
	c.Assert(err, IsNil)
	log.Target = c
	log.Debug = true

	// A charm to play around with.
	dir, err := charm.ReadDir(repoDir("dummy"))
	c.Assert(err, IsNil)
	s.charm = dir
}

func (s *S) TearDownTest(c *C) {
	s.store.Close()
	s.MgoSuite.TearDownTest(c)
}

func repoDir(name string) (path string) {
	return filepath.Join("..", "charm", "testrepo", "series", name)
}

func (s *S) TestAddCharmWithRevisionedURL(c *C) {
	urls := []*charm.URL{charm.MustParseURL("cs:oneiric/wordpress-0")}
	wc, revno, err := s.store.AddCharm(s.charm, urls, "key")
	c.Assert(err, ErrorMatches, "AddCharm: got charm URL with revision: cs:oneiric/wordpress-0")
	c.Assert(revno, Equals, 0)
	c.Assert(wc, IsNil)
}

func (s *S) TestAddCharm(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	wc, revno, err := s.store.AddCharm(s.charm, urls, "key")
	c.Assert(err, IsNil)
	c.Assert(revno, Equals, 0)

	err = s.charm.BundleTo(wc)
	c.Assert(err, IsNil)
	err = wc.Close()
	c.Assert(err, IsNil)

	for _, url := range urls {
		info, rc, err := s.store.OpenCharm(url)
		c.Assert(err, IsNil)
		data, err := ioutil.ReadAll(rc)
		err = rc.Close()
		c.Assert(info.Revision(), Equals, 0)
		c.Assert(err, IsNil)
		bundle, err := charm.ReadBundleBytes(data)
		c.Assert(err, IsNil)

		// The same information must be available by reading the
		// full charm data...
		c.Assert(bundle.Meta().Name, Equals, "dummy")
		c.Assert(bundle.Config().Options["title"].Default, Equals, "My Title")

		// ... and the queriable details.
		c.Assert(info.Meta().Name, Equals, "dummy")
		c.Assert(info.Config().Options["title"].Default, Equals, "My Title")

		info2, err := s.store.CharmInfo(url)
		c.Assert(err, IsNil)
		c.Assert(info2, Equals, info)

		// The successful completion is also recorded as a charm event.
		event, err := s.store.CharmEvent(url, "key")
		c.Assert(event.Kind, Equals, store.EventPublishDone)
		c.Assert(event.RevisionKey, Equals, "key")
		c.Assert(event.URLs, Equals, urls)
		c.Assert(event.Errors, IsNil)
		c.Assert(event.Warnings, IsNil)
	}
}

func (s *S) TestCharmInfoNotFound(c *C) {
	info, err := s.store.CharmInfo(charm.MustParseURL("cs:oneiric/wordpress"))
	c.Assert(err == store.ErrNotFound, Equals, true)
	c.Assert(info, IsNil)
}

func (s *S) TestConflictingUpdates(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	// Initiate an update of B only to force a partial conflict.
	wc, revno, err := s.store.AddCharm(s.charm, urls[1:], "key0")
	c.Assert(err, IsNil)
	c.Assert(revno, Equals, 0)

	// Partially conflicts with in-progress update above.
	wc2, revno, err := s.store.AddCharm(s.charm, urls, "key1")

	cerr := wc.Close()
	c.Assert(cerr, IsNil)

	c.Assert(err, ErrorMatches, "charm update in progress")
	c.Assert(revno, Equals, 0)
	c.Assert(wc2, IsNil)

	// Trying again should work since wc was closed.
	wc, revno, err = s.store.AddCharm(s.charm, urls, "key2")
	c.Assert(err, IsNil)
	c.Assert(revno, Equals, 0)
	_, err = wc.Write([]byte("rev0"))
	cerr = wc.Close()
	c.Assert(cerr, IsNil)
	c.Assert(err, IsNil)

	// Must be revision 0 since initial updates didn't write.
	info, rc, err := s.store.OpenCharm(urls[1])
	c.Assert(err, IsNil)
	c.Assert(info.Revision(), Equals, 0)
	err = rc.Close()
	c.Assert(err, IsNil)
}

func (s *S) TestExpiringConflict(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	// Initiate an update of B only to force a partial conflict.
	wc, _, err := s.store.AddCharm(s.charm, urls[1:], "key0")
	c.Assert(err, IsNil)

	_, err = wc.Write([]byte("rev0"))
	c.Check(err, IsNil)

	// Hack time to force an expiration.
	locks := s.Session.DB("juju").C("locks")
	selector := bson.M{"_id": urlB.String()}
	update := bson.M{"time": bson.Now() - store.UpdateTimeout - 10e9}
	err = locks.Update(selector, update)
	c.Check(err, IsNil)

	// Works due to expiration of previous lock.
	wc2, revno, err := s.store.AddCharm(s.charm, urls, "key1")
	c.Check(err, IsNil)
	c.Check(revno, Equals, 0)

	_, err = wc2.Write([]byte("rev0"))
	c.Check(err, IsNil)

	err = wc2.Close()
	c.Check(err, IsNil)

	// Failure. Lost the race.
	err = wc.Close()
	c.Check(err == store.ErrUpdateConflict, Equals, true)

}

func (s *S) TestRevisioning(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	tests := []struct {
		urls []*charm.URL
		data string
	}{
		{urls[0:], "rev0"},
		{urls[1:], "rev1"},
		{urls[0:], "rev2"},
	}

	for i, t := range tests {
		wc, revno, err := s.store.AddCharm(s.charm, t.urls, "key-"+t.data)
		c.Assert(err, IsNil)
		c.Assert(revno, Equals, i)
		_, err = wc.Write([]byte(t.data))
		cerr := wc.Close()
		c.Assert(err, IsNil)
		c.Assert(cerr, IsNil)
	}

	for i, t := range tests {
		for _, url := range t.urls {
			url = url.WithRevision(i)
			info, rc, err := s.store.OpenCharm(url)
			c.Assert(err, IsNil)
			data, err := ioutil.ReadAll(rc)
			cerr := rc.Close()
			c.Assert(info.Revision(), Equals, i)
			c.Assert(url.Revision, Equals, i) // Untouched.
			c.Assert(cerr, IsNil)
			c.Assert(string(data), Equals, string(t.data))
			c.Assert(err, IsNil)
		}
	}

	info, rc, err := s.store.OpenCharm(urlA.WithRevision(1))
	c.Assert(err, Equals, store.ErrNotFound)
	c.Assert(info, IsNil)
	c.Assert(rc, IsNil)
}

func (s *S) TestRedundantUpdate(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	wc, revno, err := s.store.AddCharm(s.charm, urls, "key0")
	c.Assert(err, IsNil)
	c.Assert(revno, Equals, 0)
	_, err = wc.Write([]byte("rev0"))
	c.Assert(err, IsNil)
	err = wc.Close()
	c.Assert(err, IsNil)

	// All charms are already on key1.
	wc, revno, err = s.store.AddCharm(s.charm, urls, "key0")
	c.Assert(err, ErrorMatches, "charm is up-to-date")
	c.Assert(err == store.ErrRedundantUpdate, Equals, true)
	c.Assert(revno, Equals, 0)
	c.Assert(wc, IsNil)

	// Now add a second revision just for wordpress-b.
	wc, revno, err = s.store.AddCharm(s.charm, urls[1:], "key1")
	c.Assert(err, IsNil)
	c.Assert(revno, Equals, 1)
	_, err = wc.Write([]byte("rev1"))
	c.Assert(err, IsNil)
	err = wc.Close()
	c.Assert(err, IsNil)

	// Same key bumps revision because one of them was old.
	wc, revno, err = s.store.AddCharm(s.charm, urls, "key1")
	c.Assert(err, IsNil)
	c.Assert(revno, Equals, 2)
	_, err = wc.Write([]byte("rev2"))
	c.Assert(err, IsNil)
	err = wc.Close()
	c.Assert(err, IsNil)
}

func (s *S) TestSha256(c *C) {
	url := charm.MustParseURL("cs:oneiric/wordpress")
	urls := []*charm.URL{url}

	wc, revno, err := s.store.AddCharm(s.charm, urls, "key")
	c.Assert(err, IsNil)
	c.Assert(revno, Equals, 0)
	_, err = wc.Write([]byte("Hello world!"))
	c.Assert(err, IsNil)
	err = wc.Close()
	c.Assert(err, IsNil)

	info, rc, err := s.store.OpenCharm(url)
	c.Assert(err, IsNil)
	c.Check(info.Sha256(), Equals, "c0535e4be2b79ffd93291305436bf889314e4a3faec05ecffcbb7df31ad9e51a")
	err = rc.Close()
	c.Check(err, IsNil)
}

func (s *S) TestLogCharmEventWithRevisionedURL(c *C) {
	url := charm.MustParseURL("cs:oneiric/wordpress-0")
	event := &store.CharmEvent{
		Kind:        store.EventPublishFailed,
		RevisionKey: "key",
		URLs:        []*charm.URL{url},
	}
	err := s.store.LogCharmEvent(event)
	c.Assert(err, ErrorMatches, "LogCharmEvent: got charm URL with revision: cs:oneiric/wordpress-0")

	// TODO: This may work in the future, but not now.
	event, err = s.store.CharmEvent(url, "key")
	c.Assert(err, ErrorMatches, "CharmEvent: got charm URL with revision: cs:oneiric/wordpress-0")
	c.Assert(event, IsNil)
}

func (s *S) TestLogCharmEvent(c *C) {
	url1 := charm.MustParseURL("cs:oneiric/wordpress")
	url2 := charm.MustParseURL("cs:oneiric/mysql")
	urls := []*charm.URL{url1, url2}

	event1 := &store.CharmEvent{
		Kind:        store.EventPublishDone,
		Revision:    42,
		RevisionKey: "revKey1",
		URLs:        urls,
		Warnings:    []string{"A warning."},
		Errors:      []string{"An error."},
		Time:        time.Unix(1, 0),
	}
	event2 := &store.CharmEvent{
		Kind:        store.EventPublishFailed,
		RevisionKey: "revKey2",
		URLs:        urls[:1],
	}

	err := s.store.LogCharmEvent(event1)
	c.Assert(err, IsNil)
	err = s.store.LogCharmEvent(event2)
	c.Assert(err, IsNil)

	events := s.Session.DB("juju").C("events")
	var s1, s2 map[string]interface{}

	err = events.Find(bson.M{"errors": bson.M{"$exists": true}}).One(&s1)
	c.Assert(err, IsNil)
	c.Assert(s1["kind"], Equals, int(store.EventPublishDone))
	c.Assert(s1["revisionkey"], Equals, "revKey1")
	c.Assert(s1["urls"], Equals, []interface{}{"cs:oneiric/wordpress", "cs:oneiric/mysql"})
	c.Assert(s1["warnings"], Equals, []interface{}{"A warning."})
	c.Assert(s1["errors"], Equals, []interface{}{"An error."})
	c.Assert(s1["time"], Equals, bson.Timestamp(1e9))

	err = events.Find(bson.M{"errors": bson.M{"$exists": false}}).One(&s2)
	c.Assert(err, IsNil)
	c.Assert(s2["kind"], Equals, int(store.EventPublishFailed))
	c.Assert(s2["revisionkey"], Equals, "revKey2")
	c.Assert(s2["urls"], Equals, []interface{}{"cs:oneiric/wordpress"})
	c.Assert(s2["warnings"], IsNil)
	c.Assert(s2["errors"], IsNil)
	c.Assert(s2["time"].(bson.Timestamp) > bson.Now()-10e9, Equals, true)

	// Mongo stores timestamps in milliseconds, so chop
	// off the extra bits for comparison.
	event2.Time = time.Unix(0, event2.Time.UnixNano()/1e6*1e6)

	event, err := s.store.CharmEvent(urls[0], "revKey2")
	c.Assert(err, IsNil)
	c.Assert(event, Equals, event2)

	event, err = s.store.CharmEvent(urls[1], "revKey1")
	c.Assert(err, IsNil)
	c.Assert(event, Equals, event1)

	event, err = s.store.CharmEvent(urls[1], "revKeyX")
	c.Assert(err == store.ErrNotFound, Equals, true)
	c.Assert(event, IsNil)
}

func (s *S) TestConflictingCharmEventUpdate(c *C) {
	url := charm.MustParseURL("cs:oneiric/wordpress")
	urls := []*charm.URL{url}

	// Initiate an update to force a conflict.
	wc, _, err := s.store.AddCharm(s.charm, urls, "key0")
	c.Assert(err, IsNil)

	event := &store.CharmEvent{
		Kind:        store.EventPublishFailed,
		RevisionKey: "revKey",
		URLs:        urls,
	}

	err = s.store.LogCharmEvent(event)
	cerr := wc.Close()
	c.Assert(cerr, IsNil)
	c.Assert(err, ErrorMatches, "charm update in progress")
}
