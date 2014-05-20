// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	stdtesting "testing"
	"time"

	"labix.org/v2/mgo/bson"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/store"
	"launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackageSsl(t, false)
}

var _ = gc.Suite(&StoreSuite{})
var _ = gc.Suite(&TrivialSuite{})

type StoreSuite struct {
	testing.MgoSuite
	testing.HTTPSuite
	testing.BaseSuite
	store *store.Store
}

var noTestMongoJs *bool = flag.Bool("notest-mongojs", false, "Disable MongoDB tests that require javascript")

type TrivialSuite struct{}

func (s *StoreSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
	if os.Getenv("JUJU_NOTEST_MONGOJS") == "1" || testing.MgoServer.WithoutV8 {
		c.Log("Tests requiring MongoDB Javascript will be skipped")
		*noTestMongoJs = true
	}
}

func (s *StoreSuite) TearDownSuite(c *gc.C) {
	s.HTTPSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *StoreSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
	var err error
	s.store, err = store.Open(testing.MgoServer.Addr())
	c.Assert(err, gc.IsNil)
}

func (s *StoreSuite) TearDownTest(c *gc.C) {
	if s.store != nil {
		s.store.Close()
	}
	s.HTTPSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

// FakeCharmDir is a charm that implements the interface that the
// store publisher cares about.
type FakeCharmDir struct {
	revision interface{} // so we can tell if it's not set.
	error    string
}

func (d *FakeCharmDir) Meta() *charm.Meta {
	return &charm.Meta{
		Name:        "fakecharm",
		Summary:     "Fake charm for testing purposes.",
		Description: "This is a fake charm for testing purposes.\n",
		Provides:    make(map[string]charm.Relation),
		Requires:    make(map[string]charm.Relation),
		Peers:       make(map[string]charm.Relation),
	}
}

func (d *FakeCharmDir) Config() *charm.Config {
	return &charm.Config{make(map[string]charm.Option)}
}

func (d *FakeCharmDir) SetRevision(revision int) {
	d.revision = revision
}

func (d *FakeCharmDir) BundleTo(w io.Writer) error {
	if d.error == "beforeWrite" {
		return fmt.Errorf(d.error)
	}
	_, err := w.Write([]byte(fmt.Sprintf("charm-revision-%v", d.revision)))
	if d.error == "afterWrite" {
		return fmt.Errorf(d.error)
	}
	return err
}

func (s *StoreSuite) TestCharmPublisherWithRevisionedURL(c *gc.C) {
	urls := []*charm.URL{charm.MustParseURL("cs:oneiric/wordpress-0")}
	pub, err := s.store.CharmPublisher(urls, "some-digest")
	c.Assert(err, gc.ErrorMatches, "CharmPublisher: got charm URL with revision: cs:oneiric/wordpress-0")
	c.Assert(pub, gc.IsNil)
}

func (s *StoreSuite) TestCharmPublisher(c *gc.C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	pub, err := s.store.CharmPublisher(urls, "some-digest")
	c.Assert(err, gc.IsNil)
	c.Assert(pub.Revision(), gc.Equals, 0)

	err = pub.Publish(testing.Charms.ClonedDir(c.MkDir(), "dummy"))
	c.Assert(err, gc.IsNil)

	for _, url := range urls {
		info, rc, err := s.store.OpenCharm(url)
		c.Assert(err, gc.IsNil)
		c.Assert(info.Revision(), gc.Equals, 0)
		c.Assert(info.Digest(), gc.Equals, "some-digest")
		data, err := ioutil.ReadAll(rc)
		c.Check(err, gc.IsNil)
		err = rc.Close()
		c.Assert(err, gc.IsNil)
		bundle, err := charm.ReadBundleBytes(data)
		c.Assert(err, gc.IsNil)

		// The same information must be available by reading the
		// full charm data...
		c.Assert(bundle.Meta().Name, gc.Equals, "dummy")
		c.Assert(bundle.Config().Options["title"].Default, gc.Equals, "My Title")

		// ... and the queriable details.
		c.Assert(info.Meta().Name, gc.Equals, "dummy")
		c.Assert(info.Config().Options["title"].Default, gc.Equals, "My Title")

		info2, err := s.store.CharmInfo(url)
		c.Assert(err, gc.IsNil)
		c.Assert(info2, gc.DeepEquals, info)
	}
}

func (s *StoreSuite) TestDeleteCharm(c *gc.C) {
	url := charm.MustParseURL("cs:oneiric/wordpress")
	for i := 0; i < 4; i++ {
		pub, err := s.store.CharmPublisher([]*charm.URL{url},
			fmt.Sprintf("some-digest-%d", i))
		c.Assert(err, gc.IsNil)
		c.Assert(pub.Revision(), gc.Equals, i)

		err = pub.Publish(testing.Charms.ClonedDir(c.MkDir(), "dummy"))
		c.Assert(err, gc.IsNil)
	}

	// Verify charms were published
	info, rc, err := s.store.OpenCharm(url)
	c.Assert(err, gc.IsNil)
	err = rc.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(info.Revision(), gc.Equals, 3)

	// Delete an arbitrary middle revision
	url1 := url.WithRevision(1)
	infos, err := s.store.DeleteCharm(url1)
	c.Assert(err, gc.IsNil)
	c.Assert(len(infos), gc.Equals, 1)

	// Verify still published
	info, rc, err = s.store.OpenCharm(url)
	c.Assert(err, gc.IsNil)
	err = rc.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(info.Revision(), gc.Equals, 3)

	// Delete all revisions
	expectedRevs := map[int]bool{0: true, 2: true, 3: true}
	infos, err = s.store.DeleteCharm(url)
	c.Assert(err, gc.IsNil)
	c.Assert(len(infos), gc.Equals, 3)
	for _, deleted := range infos {
		// We deleted the charm we expected to
		c.Assert(info.Meta().Name, gc.Equals, deleted.Meta().Name)
		_, has := expectedRevs[deleted.Revision()]
		c.Assert(has, gc.Equals, true)
		delete(expectedRevs, deleted.Revision())
	}
	c.Assert(len(expectedRevs), gc.Equals, 0)

	// The charm is all gone
	_, _, err = s.store.OpenCharm(url)
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *StoreSuite) TestCharmPublishError(c *gc.C) {
	url := charm.MustParseURL("cs:oneiric/wordpress")
	urls := []*charm.URL{url}

	// Publish one successfully to bump the revision so we can
	// make sure it is being correctly set below.
	pub, err := s.store.CharmPublisher(urls, "one-digest")
	c.Assert(err, gc.IsNil)
	c.Assert(pub.Revision(), gc.Equals, 0)
	err = pub.Publish(&FakeCharmDir{})
	c.Assert(err, gc.IsNil)

	pub, err = s.store.CharmPublisher(urls, "another-digest")
	c.Assert(err, gc.IsNil)
	c.Assert(pub.Revision(), gc.Equals, 1)
	err = pub.Publish(&FakeCharmDir{error: "beforeWrite"})
	c.Assert(err, gc.ErrorMatches, "beforeWrite")

	pub, err = s.store.CharmPublisher(urls, "another-digest")
	c.Assert(err, gc.IsNil)
	c.Assert(pub.Revision(), gc.Equals, 1)
	err = pub.Publish(&FakeCharmDir{error: "afterWrite"})
	c.Assert(err, gc.ErrorMatches, "afterWrite")

	// Still at the original charm revision that succeeded first.
	info, err := s.store.CharmInfo(url)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Revision(), gc.Equals, 0)
	c.Assert(info.Digest(), gc.Equals, "one-digest")
}

func (s *StoreSuite) TestCharmInfoNotFound(c *gc.C) {
	info, err := s.store.CharmInfo(charm.MustParseURL("cs:oneiric/wordpress"))
	c.Assert(err, gc.Equals, store.ErrNotFound)
	c.Assert(info, gc.IsNil)
}

func (s *StoreSuite) TestRevisioning(c *gc.C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	tests := []struct {
		urls []*charm.URL
		data string
	}{
		{urls[0:], "charm-revision-0"},
		{urls[1:], "charm-revision-1"},
		{urls[0:], "charm-revision-2"},
	}

	for i, t := range tests {
		pub, err := s.store.CharmPublisher(t.urls, fmt.Sprintf("digest-%d", i))
		c.Assert(err, gc.IsNil)
		c.Assert(pub.Revision(), gc.Equals, i)

		err = pub.Publish(&FakeCharmDir{})
		c.Assert(err, gc.IsNil)
	}

	for i, t := range tests {
		for _, url := range t.urls {
			url = url.WithRevision(i)
			info, rc, err := s.store.OpenCharm(url)
			c.Assert(err, gc.IsNil)
			data, err := ioutil.ReadAll(rc)
			cerr := rc.Close()
			c.Assert(info.Revision(), gc.Equals, i)
			c.Assert(url.Revision, gc.Equals, i) // Untouched.
			c.Assert(cerr, gc.IsNil)
			c.Assert(string(data), gc.Equals, string(t.data))
			c.Assert(err, gc.IsNil)
		}
	}

	info, rc, err := s.store.OpenCharm(urlA.WithRevision(1))
	c.Assert(err, gc.Equals, store.ErrNotFound)
	c.Assert(info, gc.IsNil)
	c.Assert(rc, gc.IsNil)
}

func (s *StoreSuite) TestLockUpdates(c *gc.C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	// Lock update of just B to force a partial conflict.
	lock1, err := s.store.LockUpdates(urls[1:])
	c.Assert(err, gc.IsNil)

	// Partially conflicts with locked update above.
	lock2, err := s.store.LockUpdates(urls)
	c.Check(err, gc.Equals, store.ErrUpdateConflict)
	c.Check(lock2, gc.IsNil)

	lock1.Unlock()

	// Trying again should work since lock1 was released.
	lock3, err := s.store.LockUpdates(urls)
	c.Assert(err, gc.IsNil)
	lock3.Unlock()
}

func (s *StoreSuite) TestLockUpdatesExpires(c *gc.C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	// Initiate an update of B only to force a partial conflict.
	lock1, err := s.store.LockUpdates(urls[1:])
	c.Assert(err, gc.IsNil)

	// Hack time to force an expiration.
	locks := s.Session.DB("juju").C("locks")
	selector := bson.M{"_id": urlB.String()}
	update := bson.M{"time": bson.Now().Add(-store.UpdateTimeout - 10e9)}
	err = locks.Update(selector, update)
	c.Check(err, gc.IsNil)

	// Works due to expiration of previous lock.
	lock2, err := s.store.LockUpdates(urls)
	c.Assert(err, gc.IsNil)
	defer lock2.Unlock()

	// The expired lock was forcefully killed. Unlocking it must
	// not interfere with lock2 which is still alive.
	lock1.Unlock()

	// The above statement was a NOOP and lock2 is still in effect,
	// so attempting another lock must necessarily fail.
	lock3, err := s.store.LockUpdates(urls)
	c.Check(err, gc.Equals, store.ErrUpdateConflict)
	c.Check(lock3, gc.IsNil)
}

var seriesSolverCharms = []struct {
	series, name string
}{
	{"oneiric", "wordpress"},
	{"precise", "wordpress"},
	{"quantal", "wordpress"},
	{"trusty", "wordpress"},
	{"volumetric", "wordpress"},

	{"precise", "mysql"},
	{"trusty", "mysqladmin"},

	{"def", "zebra"},
	{"zef", "zebra"},
}

func (s *StoreSuite) TestSeriesSolver(c *gc.C) {
	for _, t := range seriesSolverCharms {
		url := charm.MustParseURL(fmt.Sprintf("cs:%s/%s", t.series, t.name))
		urls := []*charm.URL{url}

		pub, err := s.store.CharmPublisher(urls, fmt.Sprintf("some-%s-%s-digest", t.series, t.name))
		c.Assert(err, gc.IsNil)
		c.Assert(pub.Revision(), gc.Equals, 0)

		err = pub.Publish(&FakeCharmDir{})
		c.Assert(err, gc.IsNil)
	}

	// LTS, then non-LTS, reverse alphabetical order
	ref, _, err := charm.ParseReference("cs:wordpress")
	c.Assert(err, gc.IsNil)
	series, err := s.store.Series(ref)
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.HasLen, 5)
	c.Check(series[0], gc.Equals, "trusty")
	c.Check(series[1], gc.Equals, "precise")
	c.Check(series[2], gc.Equals, "volumetric")
	c.Check(series[3], gc.Equals, "quantal")
	c.Check(series[4], gc.Equals, "oneiric")

	// Ensure that the full charm name matches, not just prefix
	ref, _, err = charm.ParseReference("cs:mysql")
	c.Assert(err, gc.IsNil)
	series, err = s.store.Series(ref)
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.HasLen, 1)
	c.Check(series[0], gc.Equals, "precise")

	// No LTS, reverse alphabetical order
	ref, _, err = charm.ParseReference("cs:zebra")
	c.Assert(err, gc.IsNil)
	series, err = s.store.Series(ref)
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.HasLen, 2)
	c.Check(series[0], gc.Equals, "zef")
	c.Check(series[1], gc.Equals, "def")
}

var mysqlSeriesCharms = []struct {
	fakeDigest string
	urls       []string
}{
	{"533224069221503992aaa726", []string{"cs:~charmers/oneiric/mysql", "cs:oneiric/mysql"}},
	{"533224c79221503992aaa7ea", []string{"cs:~charmers/precise/mysql", "cs:precise/mysql"}},
	{"533223a69221503992aaa6be", []string{"cs:~bjornt/trusty/mysql"}},
	{"533225b49221503992aaa8e5", []string{"cs:~clint-fewbar/precise/mysql"}},
	{"5332261b9221503992aaa96b", []string{"cs:~gandelman-a/precise/mysql"}},
	{"533226289221503992aaa97d", []string{"cs:~gandelman-a/quantal/mysql"}},
	{"5332264d9221503992aaa9b0", []string{"cs:~hazmat/precise/mysql"}},
	{"5332272d9221503992aaaa4d", []string{"cs:~jmit/oneiric/mysql"}},
	{"53328a439221503992aaad28", []string{"cs:~landscape/trusty/mysql"}},
	{"533228ae9221503992aaab96", []string{"cs:~negronjl/precise/mysql-file-permissions"}},
	{"533228f39221503992aaabde", []string{"cs:~openstack-ubuntu-testing/oneiric/mysql"}},
	{"533229029221503992aaabed", []string{"cs:~openstack-ubuntu-testing/precise/mysql"}},
	{"5332291e9221503992aaac09", []string{"cs:~openstack-ubuntu-testing/quantal/mysql"}},
	{"53327f4f9221503992aaad1e", []string{"cs:~tribaal/trusty/mysql"}},
}

func (s *StoreSuite) TestMysqlSeriesSolver(c *gc.C) {
	for _, t := range mysqlSeriesCharms {
		var urls []*charm.URL
		for _, url := range t.urls {
			urls = append(urls, charm.MustParseURL(url))
		}

		pub, err := s.store.CharmPublisher(urls, t.fakeDigest)
		c.Assert(err, gc.IsNil)
		c.Assert(pub.Revision(), gc.Equals, 0)

		err = pub.Publish(&FakeCharmDir{})
		c.Assert(err, gc.IsNil)
	}

	ref, _, err := charm.ParseReference("cs:mysql")
	c.Assert(err, gc.IsNil)
	series, err := s.store.Series(ref)
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.HasLen, 2)
	c.Check(series[0], gc.Equals, "precise")
	c.Check(series[1], gc.Equals, "oneiric")
}

func (s *StoreSuite) TestConflictingUpdate(c *gc.C) {
	// This test checks that if for whatever reason the locking
	// safety-net fails, adding two charms in parallel still
	// results in a sane outcome.
	url := charm.MustParseURL("cs:oneiric/wordpress")
	urls := []*charm.URL{url}

	pub1, err := s.store.CharmPublisher(urls, "some-digest")
	c.Assert(err, gc.IsNil)
	c.Assert(pub1.Revision(), gc.Equals, 0)

	pub2, err := s.store.CharmPublisher(urls, "some-digest")
	c.Assert(err, gc.IsNil)
	c.Assert(pub2.Revision(), gc.Equals, 0)

	// The first publishing attempt should work.
	err = pub2.Publish(&FakeCharmDir{})
	c.Assert(err, gc.IsNil)

	// Attempting to finish the second attempt should break,
	// since it lost the race and the given revision is already
	// in place.
	err = pub1.Publish(&FakeCharmDir{})
	c.Assert(err, gc.Equals, store.ErrUpdateConflict)
}

func (s *StoreSuite) TestRedundantUpdate(c *gc.C) {
	urlA := charm.MustParseURL("cs:oneiric/wordpress-a")
	urlB := charm.MustParseURL("cs:oneiric/wordpress-b")
	urls := []*charm.URL{urlA, urlB}

	pub, err := s.store.CharmPublisher(urls, "digest-0")
	c.Assert(err, gc.IsNil)
	c.Assert(pub.Revision(), gc.Equals, 0)
	err = pub.Publish(&FakeCharmDir{})
	c.Assert(err, gc.IsNil)

	// All charms are already on digest-0.
	pub, err = s.store.CharmPublisher(urls, "digest-0")
	c.Assert(err, gc.ErrorMatches, "charm is up-to-date")
	c.Assert(err, gc.Equals, store.ErrRedundantUpdate)
	c.Assert(pub, gc.IsNil)

	// Now add a second revision just for wordpress-b.
	pub, err = s.store.CharmPublisher(urls[1:], "digest-1")
	c.Assert(err, gc.IsNil)
	c.Assert(pub.Revision(), gc.Equals, 1)
	err = pub.Publish(&FakeCharmDir{})
	c.Assert(err, gc.IsNil)

	// Same digest bumps revision because one of them was old.
	pub, err = s.store.CharmPublisher(urls, "digest-1")
	c.Assert(err, gc.IsNil)
	c.Assert(pub.Revision(), gc.Equals, 2)
	err = pub.Publish(&FakeCharmDir{})
	c.Assert(err, gc.IsNil)
}

const fakeRevZeroSha = "319095521ac8a62fa1e8423351973512ecca8928c9f62025e37de57c9ef07a53"

func (s *StoreSuite) TestCharmBundleData(c *gc.C) {
	url := charm.MustParseURL("cs:oneiric/wordpress")
	urls := []*charm.URL{url}

	pub, err := s.store.CharmPublisher(urls, "key")
	c.Assert(err, gc.IsNil)
	c.Assert(pub.Revision(), gc.Equals, 0)

	err = pub.Publish(&FakeCharmDir{})
	c.Assert(err, gc.IsNil)

	info, rc, err := s.store.OpenCharm(url)
	c.Assert(err, gc.IsNil)
	c.Check(info.BundleSha256(), gc.Equals, fakeRevZeroSha)
	c.Check(info.BundleSize(), gc.Equals, int64(len("charm-revision-0")))
	err = rc.Close()
	c.Check(err, gc.IsNil)
}

func (s *StoreSuite) TestLogCharmEventWithRevisionedURL(c *gc.C) {
	url := charm.MustParseURL("cs:oneiric/wordpress-0")
	event := &store.CharmEvent{
		Kind:   store.EventPublishError,
		Digest: "some-digest",
		URLs:   []*charm.URL{url},
	}
	err := s.store.LogCharmEvent(event)
	c.Assert(err, gc.ErrorMatches, "LogCharmEvent: got charm URL with revision: cs:oneiric/wordpress-0")

	// This may work in the future, but not now.
	event, err = s.store.CharmEvent(url, "some-digest")
	c.Assert(err, gc.ErrorMatches, "CharmEvent: got charm URL with revision: cs:oneiric/wordpress-0")
	c.Assert(event, gc.IsNil)
}

func (s *StoreSuite) TestLogCharmEvent(c *gc.C) {
	url1 := charm.MustParseURL("cs:oneiric/wordpress")
	url2 := charm.MustParseURL("cs:oneiric/mysql")
	urls := []*charm.URL{url1, url2}

	event1 := &store.CharmEvent{
		Kind:     store.EventPublished,
		Revision: 42,
		Digest:   "revKey1",
		URLs:     urls,
		Warnings: []string{"A warning."},
		Time:     time.Unix(1, 0),
	}
	event2 := &store.CharmEvent{
		Kind:     store.EventPublished,
		Revision: 42,
		Digest:   "revKey2",
		URLs:     urls,
		Time:     time.Unix(1, 0),
	}
	event3 := &store.CharmEvent{
		Kind:   store.EventPublishError,
		Digest: "revKey2",
		Errors: []string{"An error."},
		URLs:   urls[:1],
	}

	for _, event := range []*store.CharmEvent{event1, event2, event3} {
		err := s.store.LogCharmEvent(event)
		c.Assert(err, gc.IsNil)
	}

	events := s.Session.DB("juju").C("events")
	var s1, s2 map[string]interface{}

	err := events.Find(bson.M{"digest": "revKey1"}).One(&s1)
	c.Assert(err, gc.IsNil)
	c.Assert(s1["kind"], gc.Equals, int(store.EventPublished))
	c.Assert(s1["urls"], gc.DeepEquals, []interface{}{"cs:oneiric/wordpress", "cs:oneiric/mysql"})
	c.Assert(s1["warnings"], gc.DeepEquals, []interface{}{"A warning."})
	c.Assert(s1["errors"], gc.IsNil)
	c.Assert(s1["time"], gc.DeepEquals, time.Unix(1, 0))

	err = events.Find(bson.M{"digest": "revKey2", "kind": store.EventPublishError}).One(&s2)
	c.Assert(err, gc.IsNil)
	c.Assert(s2["urls"], gc.DeepEquals, []interface{}{"cs:oneiric/wordpress"})
	c.Assert(s2["warnings"], gc.IsNil)
	c.Assert(s2["errors"], gc.DeepEquals, []interface{}{"An error."})
	c.Assert(s2["time"].(time.Time).After(bson.Now().Add(-10e9)), gc.Equals, true)

	// Mongo stores timestamps in milliseconds, so chop
	// off the extra bits for comparison.
	event3.Time = time.Unix(0, event3.Time.UnixNano()/1e6*1e6)

	event, err := s.store.CharmEvent(urls[0], "revKey2")
	c.Assert(err, gc.IsNil)
	c.Assert(event, gc.DeepEquals, event3)

	event, err = s.store.CharmEvent(urls[1], "revKey1")
	c.Assert(err, gc.IsNil)
	c.Assert(event, gc.DeepEquals, event1)

	event, err = s.store.CharmEvent(urls[1], "revKeyX")
	c.Assert(err, gc.Equals, store.ErrNotFound)
	c.Assert(event, gc.IsNil)
}

func (s *StoreSuite) TestSumCounters(c *gc.C) {
	if *noTestMongoJs {
		c.Skip("MongoDB javascript not available")
	}

	req := store.CounterRequest{Key: []string{"a"}}
	cs, err := s.store.Counters(&req)
	c.Assert(err, gc.IsNil)
	c.Assert(cs, gc.DeepEquals, []store.Counter{{Key: req.Key, Count: 0}})

	for i := 0; i < 10; i++ {
		err := s.store.IncCounter([]string{"a", "b", "c"})
		c.Assert(err, gc.IsNil)
	}
	for i := 0; i < 7; i++ {
		s.store.IncCounter([]string{"a", "b"})
		c.Assert(err, gc.IsNil)
	}
	for i := 0; i < 3; i++ {
		s.store.IncCounter([]string{"a", "z", "b"})
		c.Assert(err, gc.IsNil)
	}

	tests := []struct {
		key    []string
		prefix bool
		result int64
	}{
		{[]string{"a", "b", "c"}, false, 10},
		{[]string{"a", "b"}, false, 7},
		{[]string{"a", "z", "b"}, false, 3},
		{[]string{"a", "b", "c"}, true, 0},
		{[]string{"a", "b", "c", "d"}, false, 0},
		{[]string{"a", "b"}, true, 10},
		{[]string{"a"}, true, 20},
		{[]string{"b"}, true, 0},
	}

	for _, t := range tests {
		c.Logf("Test: %#v\n", t)
		req = store.CounterRequest{Key: t.key, Prefix: t.prefix}
		cs, err := s.store.Counters(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(cs, gc.DeepEquals, []store.Counter{{Key: t.key, Prefix: t.prefix, Count: t.result}})
	}

	// High-level interface works. Now check that the data is
	// stored correctly.
	counters := s.Session.DB("juju").C("stat.counters")
	docs1, err := counters.Count()
	c.Assert(err, gc.IsNil)
	if docs1 != 3 && docs1 != 4 {
		fmt.Errorf("Expected 3 or 4 docs in counters collection, got %d", docs1)
	}

	// Hack times so that the next operation adds another document.
	err = counters.Update(nil, bson.D{{"$set", bson.D{{"t", 1}}}})
	c.Check(err, gc.IsNil)

	err = s.store.IncCounter([]string{"a", "b", "c"})
	c.Assert(err, gc.IsNil)

	docs2, err := counters.Count()
	c.Assert(err, gc.IsNil)
	c.Assert(docs2, gc.Equals, docs1+1)

	req = store.CounterRequest{Key: []string{"a", "b", "c"}}
	cs, err = s.store.Counters(&req)
	c.Assert(err, gc.IsNil)
	c.Assert(cs, gc.DeepEquals, []store.Counter{{Key: req.Key, Count: 11}})

	req = store.CounterRequest{Key: []string{"a"}, Prefix: true}
	cs, err = s.store.Counters(&req)
	c.Assert(err, gc.IsNil)
	c.Assert(cs, gc.DeepEquals, []store.Counter{{Key: req.Key, Prefix: true, Count: 21}})
}

func (s *StoreSuite) TestCountersReadOnlySum(c *gc.C) {
	if *noTestMongoJs {
		c.Skip("MongoDB javascript not available")
	}

	// Summing up an unknown key shouldn't add the key to the database.
	req := store.CounterRequest{Key: []string{"a", "b", "c"}}
	_, err := s.store.Counters(&req)
	c.Assert(err, gc.IsNil)

	tokens := s.Session.DB("juju").C("stat.tokens")
	n, err := tokens.Count()
	c.Assert(err, gc.IsNil)
	c.Assert(n, gc.Equals, 0)
}

func (s *StoreSuite) TestCountersTokenCaching(c *gc.C) {
	if *noTestMongoJs {
		c.Skip("MongoDB javascript not available")
	}

	assertSum := func(i int, want int64) {
		req := store.CounterRequest{Key: []string{strconv.Itoa(i)}}
		cs, err := s.store.Counters(&req)
		c.Assert(err, gc.IsNil)
		c.Assert(cs[0].Count, gc.Equals, want)
	}
	assertSum(100000, 0)

	const genSize = 1024

	// All of these will be cached, as we have two generations
	// of genSize entries each.
	for i := 0; i < genSize*2; i++ {
		err := s.store.IncCounter([]string{strconv.Itoa(i)})
		c.Assert(err, gc.IsNil)
	}

	// Now go behind the scenes and corrupt all the tokens.
	tokens := s.Session.DB("juju").C("stat.tokens")
	iter := tokens.Find(nil).Iter()
	var t struct {
		Id    int    "_id"
		Token string "t"
	}
	for iter.Next(&t) {
		err := tokens.UpdateId(t.Id, bson.M{"$set": bson.M{"t": "corrupted" + t.Token}})
		c.Assert(err, gc.IsNil)
	}
	c.Assert(iter.Err(), gc.IsNil)

	// We can consult the counters for the cached entries still.
	// First, check that the newest generation is good.
	for i := genSize; i < genSize*2; i++ {
		assertSum(i, 1)
	}

	// Now, we can still access a single entry of the older generation,
	// but this will cause the generations to flip and thus the rest
	// of the old generation will go away as the top half of the
	// entries is turned into the old generation.
	assertSum(0, 1)

	// Now we've lost access to the rest of the old generation.
	for i := 1; i < genSize; i++ {
		assertSum(i, 0)
	}

	// But we still have all of the top half available since it was
	// moved into the old generation.
	for i := genSize; i < genSize*2; i++ {
		assertSum(i, 1)
	}
}

func (s *StoreSuite) TestCounterTokenUniqueness(c *gc.C) {
	if *noTestMongoJs {
		c.Skip("MongoDB javascript not available")
	}

	var wg0, wg1 sync.WaitGroup
	wg0.Add(10)
	wg1.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			wg0.Done()
			wg0.Wait()
			defer wg1.Done()
			err := s.store.IncCounter([]string{"a"})
			c.Check(err, gc.IsNil)
		}()
	}
	wg1.Wait()

	req := store.CounterRequest{Key: []string{"a"}}
	cs, err := s.store.Counters(&req)
	c.Assert(err, gc.IsNil)
	c.Assert(cs[0].Count, gc.Equals, int64(10))
}

func (s *StoreSuite) TestListCounters(c *gc.C) {
	if *noTestMongoJs {
		c.Skip("MongoDB javascript not available")
	}

	incs := [][]string{
		{"c", "b", "a"}, // Assign internal id c < id b < id a, to make sorting slightly trickier.
		{"a"},
		{"a", "c"},
		{"a", "b"},
		{"a", "b", "c"},
		{"a", "b", "c"},
		{"a", "b", "e"},
		{"a", "b", "d"},
		{"a", "f", "g"},
		{"a", "f", "h"},
		{"a", "i"},
		{"a", "i", "j"},
		{"k", "l"},
	}
	for _, key := range incs {
		err := s.store.IncCounter(key)
		c.Assert(err, gc.IsNil)
	}

	tests := []struct {
		prefix []string
		result []store.Counter
	}{
		{
			[]string{"a"},
			[]store.Counter{
				{Key: []string{"a", "b"}, Prefix: true, Count: 4},
				{Key: []string{"a", "f"}, Prefix: true, Count: 2},
				{Key: []string{"a", "b"}, Prefix: false, Count: 1},
				{Key: []string{"a", "c"}, Prefix: false, Count: 1},
				{Key: []string{"a", "i"}, Prefix: false, Count: 1},
				{Key: []string{"a", "i"}, Prefix: true, Count: 1},
			},
		}, {
			[]string{"a", "b"},
			[]store.Counter{
				{Key: []string{"a", "b", "c"}, Prefix: false, Count: 2},
				{Key: []string{"a", "b", "d"}, Prefix: false, Count: 1},
				{Key: []string{"a", "b", "e"}, Prefix: false, Count: 1},
			},
		}, {
			[]string{"z"},
			[]store.Counter(nil),
		},
	}

	// Use a different store to exercise cache filling.
	st, err := store.Open(testing.MgoServer.Addr())
	c.Assert(err, gc.IsNil)
	defer st.Close()

	for i := range tests {
		req := &store.CounterRequest{Key: tests[i].prefix, Prefix: true, List: true}
		result, err := st.Counters(req)
		c.Assert(err, gc.IsNil)
		c.Assert(result, gc.DeepEquals, tests[i].result)
	}
}

func (s *StoreSuite) TestListCountersBy(c *gc.C) {
	if *noTestMongoJs {
		c.Skip("MongoDB javascript not available")
	}

	incs := []struct {
		key []string
		day int
	}{
		{[]string{"a"}, 1},
		{[]string{"a"}, 1},
		{[]string{"b"}, 1},
		{[]string{"a", "b"}, 1},
		{[]string{"a", "c"}, 1},
		{[]string{"a"}, 3},
		{[]string{"a", "b"}, 3},
		{[]string{"b"}, 9},
		{[]string{"b"}, 9},
		{[]string{"a", "c", "d"}, 9},
		{[]string{"a", "c", "e"}, 9},
		{[]string{"a", "c", "f"}, 9},
	}

	day := func(i int) time.Time {
		return time.Date(2012, time.May, i, 0, 0, 0, 0, time.UTC)
	}

	counters := s.Session.DB("juju").C("stat.counters")
	for i, inc := range incs {
		err := s.store.IncCounter(inc.key)
		c.Assert(err, gc.IsNil)

		// Hack time so counters are assigned to 2012-05-<day>
		filter := bson.M{"t": bson.M{"$gt": store.TimeToStamp(time.Date(2013, time.January, 1, 0, 0, 0, 0, time.UTC))}}
		stamp := store.TimeToStamp(day(inc.day))
		stamp += int32(i) * 60 // Make every entry unique.
		err = counters.Update(filter, bson.D{{"$set", bson.D{{"t", stamp}}}})
		c.Check(err, gc.IsNil)
	}

	tests := []struct {
		request store.CounterRequest
		result  []store.Counter
	}{
		{
			store.CounterRequest{
				Key:    []string{"a"},
				Prefix: false,
				List:   false,
				By:     store.ByDay,
			},
			[]store.Counter{
				{Key: []string{"a"}, Prefix: false, Count: 2, Time: day(1)},
				{Key: []string{"a"}, Prefix: false, Count: 1, Time: day(3)},
			},
		}, {
			store.CounterRequest{
				Key:    []string{"a"},
				Prefix: true,
				List:   false,
				By:     store.ByDay,
			},
			[]store.Counter{
				{Key: []string{"a"}, Prefix: true, Count: 2, Time: day(1)},
				{Key: []string{"a"}, Prefix: true, Count: 1, Time: day(3)},
				{Key: []string{"a"}, Prefix: true, Count: 3, Time: day(9)},
			},
		}, {
			store.CounterRequest{
				Key:    []string{"a"},
				Prefix: true,
				List:   false,
				By:     store.ByDay,
				Start:  day(2),
			},
			[]store.Counter{
				{Key: []string{"a"}, Prefix: true, Count: 1, Time: day(3)},
				{Key: []string{"a"}, Prefix: true, Count: 3, Time: day(9)},
			},
		}, {
			store.CounterRequest{
				Key:    []string{"a"},
				Prefix: true,
				List:   false,
				By:     store.ByDay,
				Stop:   day(4),
			},
			[]store.Counter{
				{Key: []string{"a"}, Prefix: true, Count: 2, Time: day(1)},
				{Key: []string{"a"}, Prefix: true, Count: 1, Time: day(3)},
			},
		}, {
			store.CounterRequest{
				Key:    []string{"a"},
				Prefix: true,
				List:   false,
				By:     store.ByDay,
				Start:  day(3),
				Stop:   day(8),
			},
			[]store.Counter{
				{Key: []string{"a"}, Prefix: true, Count: 1, Time: day(3)},
			},
		}, {
			store.CounterRequest{
				Key:    []string{"a"},
				Prefix: true,
				List:   true,
				By:     store.ByDay,
			},
			[]store.Counter{
				{Key: []string{"a", "b"}, Prefix: false, Count: 1, Time: day(1)},
				{Key: []string{"a", "c"}, Prefix: false, Count: 1, Time: day(1)},
				{Key: []string{"a", "b"}, Prefix: false, Count: 1, Time: day(3)},
				{Key: []string{"a", "c"}, Prefix: true, Count: 3, Time: day(9)},
			},
		}, {
			store.CounterRequest{
				Key:    []string{"a"},
				Prefix: true,
				List:   false,
				By:     store.ByWeek,
			},
			[]store.Counter{
				{Key: []string{"a"}, Prefix: true, Count: 3, Time: day(6)},
				{Key: []string{"a"}, Prefix: true, Count: 3, Time: day(13)},
			},
		}, {
			store.CounterRequest{
				Key:    []string{"a"},
				Prefix: true,
				List:   true,
				By:     store.ByWeek,
			},
			[]store.Counter{
				{Key: []string{"a", "b"}, Prefix: false, Count: 2, Time: day(6)},
				{Key: []string{"a", "c"}, Prefix: false, Count: 1, Time: day(6)},
				{Key: []string{"a", "c"}, Prefix: true, Count: 3, Time: day(13)},
			},
		},
	}

	for _, test := range tests {
		result, err := s.store.Counters(&test.request)
		c.Assert(err, gc.IsNil)
		c.Assert(result, gc.DeepEquals, test.result)
	}
}

func (s *TrivialSuite) TestEventString(c *gc.C) {
	c.Assert(store.EventPublished, gc.Matches, "published")
	c.Assert(store.EventPublishError, gc.Matches, "publish-error")
	for kind := store.CharmEventKind(1); kind < store.EventKindCount; kind++ {
		// This guarantees the switch in String is properly
		// updated with new event kinds.
		c.Assert(kind.String(), gc.Matches, "[a-z-]+")
	}
}
