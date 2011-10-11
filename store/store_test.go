package store_test

import (
	"io/ioutil"
	"launchpad.net/gobson/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/store"
	"launchpad.net/juju/go/charm"
	"launchpad.net/mgo"
	"os"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&S{})

type S struct {
	MgoSuite
	store *store.Store
}

func (s *S) SetUpTest(c *C) {
	s.MgoSuite.SetUpTest(c)
	var err os.Error
	s.store, err = store.New(s.Addr)
	c.Assert(err, IsNil)
}

func (s *S) TearDownTest(c *C) {
	s.store.Close()
	s.MgoSuite.TearDownTest(c)
}

func repoDir(name string) (path string) {
	return filepath.Join("..", "charm", "testrepo", "series", name)
}

func (s *S) TestAddCharm(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a-1")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b-2")
	urls := []*charm.URL{urlA, urlB}

	wc, err := s.store.AddCharm(urls)
	c.Assert(err, IsNil)
	dir, err := charm.ReadDir(repoDir("dummy"))
	c.Assert(err, IsNil)
	err = dir.BundleTo(wc)
	c.Assert(err, IsNil)
	err = wc.Close()
	c.Assert(err, IsNil)

	for _, url := range urls {
		rc, info, err := s.store.OpenCharm(url.WithRevision(-1))
		c.Assert(err, IsNil)
		data, err := ioutil.ReadAll(rc)
		err = rc.Close()
		c.Assert(info.Revision, Equals, 0)
		c.Assert(err, IsNil)
		bundle, err := charm.ReadBundleBytes(data)
		c.Assert(err, IsNil)
		c.Assert(bundle.Meta().Name, Equals, "dummy")
	}
}

func (s *S) TestConflictingUpdates(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	// Initiate an update of B only to force a partial conflict.
	wc, err := s.store.AddCharm(urls[1:])
	c.Assert(err, IsNil)

	// Partially conflicts with in-progress update above.
	wc2, err := s.store.AddCharm(urls)

	cerr := wc.Close()
	c.Assert(cerr, IsNil)

	c.Assert(err, Matches, "update already in progress")
	c.Assert(wc2, IsNil)

	// Trying again should work since wc was closed.
	wc, err = s.store.AddCharm(urls)
	c.Assert(err, IsNil)
	_, err = wc.Write([]byte("rev0"))
	cerr = wc.Close()
	c.Assert(cerr, IsNil)
	c.Assert(err, IsNil)

	// Must be revision 0 since initial updates didn't write.
	rc, info, err := s.store.OpenCharm(urls[1])
	c.Assert(err, IsNil)
	c.Assert(info.Revision, Equals, 0)
	err = rc.Close()
	c.Assert(err, IsNil)
}

func (s *S) TestExpiringConflict(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	// Initiate an update of B only to force a partial conflict.
	wc, err := s.store.AddCharm(urls[1:])
	c.Assert(err, IsNil)

	_, err = wc.Write([]byte("rev0"))
	c.Check(err, IsNil)

	// Hack time to force an expiration.
	locks := s.Session.DB("juju").C("locks")
	selector := bson.M{"_id": urlB.String()}
	update := bson.M{"time": bson.Now()-store.UpdateTimeout-10e9}
	err = locks.Update(selector, update)
	c.Check(err, IsNil)

	// Works due to expiration of previous lock.
	wc2, err := s.store.AddCharm(urls)
	c.Check(err, IsNil)

	_, err = wc2.Write([]byte("rev0"))
	c.Check(err, IsNil)

	err = wc2.Close()
	c.Check(err, IsNil)

	// Failure. Lost the race.
	err = wc.Close()
	c.Check(err, Equals, store.UpdateConflict)

}

func (s *S) TestRevisioning(c *C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	var tests = []struct {
		urls []*charm.URL
		data string
	}{
		{urls[0:], "rev0"},
		{urls[1:], "rev1"},
		{urls[0:], "rev2"},
	}

	for _, t := range tests {
		wc, err := s.store.AddCharm(t.urls)
		c.Assert(err, IsNil)
		_, err = wc.Write([]byte(t.data))
		cerr := wc.Close()
		c.Assert(err, IsNil)
		c.Assert(cerr, IsNil)
	}

	for i, t := range tests {
		for _, url := range t.urls {
			url = url.WithRevision(i)
			rc, info, err := s.store.OpenCharm(url)
			c.Assert(err, IsNil)
			data, err := ioutil.ReadAll(rc)
			cerr := rc.Close()
			c.Assert(info.Revision, Equals, i)
			c.Assert(url.Revision, Equals, i) // Untouched.
			c.Assert(cerr, IsNil)
			c.Assert(string(data), Equals, string(t.data))
			c.Assert(err, IsNil)
		}
	}

	rc, info, err := s.store.OpenCharm(urlA.WithRevision(1))
	c.Assert(err, Equals, mgo.NotFound)
	c.Assert(info, IsNil)
	c.Assert(rc, IsNil)
}
