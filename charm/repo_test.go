// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type MockStore struct {
	mux            *http.ServeMux
	lis            net.Listener
	bundleBytes    []byte
	bundleSha256   string
	downloads      []*charm.URL
	authorizations []string
}

func NewMockStore(c *gc.C) *MockStore {
	s := &MockStore{}
	bytes, err := ioutil.ReadFile(testing.Charms.BundlePath(c.MkDir(), "dummy"))
	c.Assert(err, gc.IsNil)
	s.bundleBytes = bytes
	h := sha256.New()
	h.Write(bytes)
	s.bundleSha256 = hex.EncodeToString(h.Sum(nil))
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/charm-info", func(w http.ResponseWriter, r *http.Request) {
		s.ServeInfo(w, r)
	})
	s.mux.HandleFunc("/charm-event", func(w http.ResponseWriter, r *http.Request) {
		s.ServeEvent(w, r)
	})
	s.mux.HandleFunc("/charm/", func(w http.ResponseWriter, r *http.Request) {
		s.ServeCharm(w, r)
	})
	lis, err := net.Listen("tcp", "127.0.0.1:4444")
	c.Assert(err, gc.IsNil)
	s.lis = lis
	go http.Serve(s.lis, s)
	return s
}

func (s *MockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *MockStore) ServeInfo(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	response := map[string]*charm.InfoResponse{}
	for _, url := range r.Form["charms"] {
		cr := &charm.InfoResponse{}
		response[url] = cr
		charmURL := charm.MustParseURL(url)
		switch charmURL.Name {
		case "borken":
			cr.Errors = append(cr.Errors, "badness")
		case "unwise":
			cr.Warnings = append(cr.Warnings, "foolishness")
			fallthrough
		case "good":
			if charmURL.Revision == -1 {
				cr.Revision = 23
			} else {
				cr.Revision = charmURL.Revision
			}
			cr.Sha256 = s.bundleSha256
		default:
			cr.Errors = append(cr.Errors, "entry not found")
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	if err != nil {
		panic(err)
	}
}

func (s *MockStore) ServeEvent(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	response := map[string]*charm.EventResponse{}
	for _, url := range r.Form["charms"] {
		digest := ""
		if i := strings.Index(url, "@"); i >= 0 {
			digest = url[i+1:]
			url = url[:i]
		}
		er := &charm.EventResponse{}
		response[url] = er
		if digest != "" && digest != "the-digest" {
			er.Kind = "not-found"
			er.Errors = []string{"entry not found"}
			continue
		}
		charmURL := charm.MustParseURL(url)
		switch charmURL.Name {
		case "borken":
			er.Kind = "publish-error"
			er.Errors = append(er.Errors, "badness")
		case "unwise":
			er.Warnings = append(er.Warnings, "foolishness")
			fallthrough
		case "good":
			er.Kind = "published"
			er.Revision = 23
			er.Digest = "the-digest"
		default:
			er.Kind = "not-found"
			er.Errors = []string{"entry not found"}
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	if err != nil {
		panic(err)
	}
}

func (s *MockStore) ServeCharm(w http.ResponseWriter, r *http.Request) {
	charmURL := charm.MustParseURL("cs:" + r.URL.Path[len("/charm/"):])
	s.downloads = append(s.downloads, charmURL)

	if auth := r.Header.Get("Authorization"); auth != "" {
		s.authorizations = append(s.authorizations, auth)
	}

	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(s.bundleBytes)))
	_, err := w.Write(s.bundleBytes)
	if err != nil {
		panic(err)
	}
}

type StoreSuite struct {
	testbase.LoggingSuite
	server      *MockStore
	store       *charm.CharmStore
	oldCacheDir string
}

var _ = gc.Suite(&StoreSuite{})

func (s *StoreSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.server = NewMockStore(c)
	s.oldCacheDir = charm.CacheDir
}

func (s *StoreSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	charm.CacheDir = c.MkDir()
	s.store = charm.NewStore("http://127.0.0.1:4444")
	s.server.downloads = nil
	s.server.authorizations = nil
}

// Uses the TearDownTest from testbase.LoggingSuite

func (s *StoreSuite) TearDownSuite(c *gc.C) {
	charm.CacheDir = s.oldCacheDir
	s.server.lis.Close()
	s.LoggingSuite.TearDownSuite(c)
}

func (s *StoreSuite) TestMissing(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/missing")
	expect := `charm not found: cs:series/missing`
	_, err := s.store.Latest(charmURL)
	c.Assert(err, gc.ErrorMatches, expect)
	_, err = s.store.Get(charmURL)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *StoreSuite) TestError(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/borken")
	expect := `charm info errors for "cs:series/borken": badness`
	_, err := s.store.Latest(charmURL)
	c.Assert(err, gc.ErrorMatches, expect)
	_, err = s.store.Get(charmURL)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *StoreSuite) TestWarning(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/unwise")
	expect := `.* WARNING juju charm: charm store reports for "cs:series/unwise": foolishness` + "\n"
	r, err := s.store.Latest(charmURL)
	c.Assert(r, gc.Equals, 23)
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, expect)
	ch, err := s.store.Get(charmURL)
	c.Assert(ch, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, expect+expect)
}

func (s *StoreSuite) TestLatest(c *gc.C) {
	for _, str := range []string{
		"cs:series/good",
		"cs:series/good-2",
		"cs:series/good-99",
	} {
		r, err := s.store.Latest(charm.MustParseURL(str))
		c.Assert(r, gc.Equals, 23)
		c.Assert(err, gc.IsNil)
	}
}

func (s *StoreSuite) assertCached(c *gc.C, charmURL *charm.URL) {
	s.server.downloads = nil
	ch, err := s.store.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.downloads, gc.IsNil)
}

func (s *StoreSuite) TestGetCacheImplicitRevision(c *gc.C) {
	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	revCharmURL := charm.MustParseURL(base + "-23")
	ch, err := s.store.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.downloads, gc.DeepEquals, []*charm.URL{revCharmURL})
	s.assertCached(c, charmURL)
	s.assertCached(c, revCharmURL)
}

func (s *StoreSuite) TestGetCacheExplicitRevision(c *gc.C) {
	base := "cs:series/good-12"
	charmURL := charm.MustParseURL(base)
	ch, err := s.store.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.downloads, gc.DeepEquals, []*charm.URL{charmURL})
	s.assertCached(c, charmURL)
}

func (s *StoreSuite) TestGetBadCache(c *gc.C) {
	c.Assert(os.Mkdir(filepath.Join(charm.CacheDir, "cache"), 0777), gc.IsNil)
	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	revCharmURL := charm.MustParseURL(base + "-23")
	name := charm.Quote(revCharmURL.String()) + ".charm"
	err := ioutil.WriteFile(filepath.Join(charm.CacheDir, "cache", name), nil, 0666)
	c.Assert(err, gc.IsNil)
	ch, err := s.store.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.downloads, gc.DeepEquals, []*charm.URL{revCharmURL})
	s.assertCached(c, charmURL)
	s.assertCached(c, revCharmURL)
}

// The following tests cover the low-level CharmStore-specific API.

func (s *StoreSuite) TestInfo(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/good")
	info, err := s.store.Info(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Errors, gc.IsNil)
	c.Assert(info.Revision, gc.Equals, 23)
}

func (s *StoreSuite) TestInfoNotFound(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/missing")
	info, err := s.store.Info(charmURL)
	c.Assert(err, gc.ErrorMatches, `charm not found: cs:series/missing`)
	c.Assert(info, gc.IsNil)
}

func (s *StoreSuite) TestInfoError(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/borken")
	info, err := s.store.Info(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Errors, gc.DeepEquals, []string{"badness"})
}

func (s *StoreSuite) TestInfoWarning(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/unwise")
	info, err := s.store.Info(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Warnings, gc.DeepEquals, []string{"foolishness"})
}

func (s *StoreSuite) TestEvent(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/good")
	event, err := s.store.Event(charmURL, "")
	c.Assert(err, gc.IsNil)
	c.Assert(event.Errors, gc.IsNil)
	c.Assert(event.Revision, gc.Equals, 23)
	c.Assert(event.Digest, gc.Equals, "the-digest")
}

func (s *StoreSuite) TestEventWithDigest(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/good")
	event, err := s.store.Event(charmURL, "the-digest")
	c.Assert(err, gc.IsNil)
	c.Assert(event.Errors, gc.IsNil)
	c.Assert(event.Revision, gc.Equals, 23)
	c.Assert(event.Digest, gc.Equals, "the-digest")
}

func (s *StoreSuite) TestEventNotFound(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/missing")
	event, err := s.store.Event(charmURL, "")
	c.Assert(err, gc.ErrorMatches, `charm event not found for "cs:series/missing"`)
	c.Assert(event, gc.IsNil)
}

func (s *StoreSuite) TestEventNotFoundDigest(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/good")
	event, err := s.store.Event(charmURL, "missing-digest")
	c.Assert(err, gc.ErrorMatches, `charm event not found for "cs:series/good" with digest "missing-digest"`)
	c.Assert(event, gc.IsNil)
}

func (s *StoreSuite) TestEventError(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/borken")
	event, err := s.store.Event(charmURL, "")
	c.Assert(err, gc.IsNil)
	c.Assert(event.Errors, gc.DeepEquals, []string{"badness"})
}

func (s *StoreSuite) TestAuthorization(c *gc.C) {
	config := testing.CustomEnvironConfig(c,
		testing.Attrs{"charm-store-auth": "token=value"})
	store := charm.AuthorizeCharmRepo(s.store, config)

	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	_, err := store.Get(charmURL)

	c.Assert(err, gc.IsNil)
	c.Assert(s.server.authorizations, gc.NotNil)
}

func (s *StoreSuite) TestNilAuthorization(c *gc.C) {
	config := testing.EnvironConfig(c)
	store := charm.AuthorizeCharmRepo(s.store, config)

	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	_, err := store.Get(charmURL)

	c.Assert(err, gc.IsNil)
	c.Assert(s.server.authorizations, gc.IsNil)
}

func (s *StoreSuite) TestEventWarning(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/unwise")
	event, err := s.store.Event(charmURL, "")
	c.Assert(err, gc.IsNil)
	c.Assert(event.Warnings, gc.DeepEquals, []string{"foolishness"})
}

func (s *StoreSuite) TestBranchLocation(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/name")
	location := s.store.BranchLocation(charmURL)
	c.Assert(location, gc.Equals, "lp:charms/series/name")

	charmURL = charm.MustParseURL("cs:~user/series/name")
	location = s.store.BranchLocation(charmURL)
	c.Assert(location, gc.Equals, "lp:~user/charms/series/name/trunk")
}

func (s *StoreSuite) TestCharmURL(c *gc.C) {
	tests := []struct{ url, loc string }{
		{"cs:precise/wordpress", "lp:charms/precise/wordpress"},
		{"cs:precise/wordpress", "http://launchpad.net/+branch/charms/precise/wordpress"},
		{"cs:precise/wordpress", "https://launchpad.net/+branch/charms/precise/wordpress"},
		{"cs:precise/wordpress", "http://code.launchpad.net/+branch/charms/precise/wordpress"},
		{"cs:precise/wordpress", "https://code.launchpad.net/+branch/charms/precise/wordpress"},
		{"cs:precise/wordpress", "bzr+ssh://bazaar.launchpad.net/+branch/charms/precise/wordpress"},
		{"cs:~charmers/precise/wordpress", "lp:~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "http://launchpad.net/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "https://launchpad.net/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "http://code.launchpad.net/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "https://code.launchpad.net/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "http://launchpad.net/+branch/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "https://launchpad.net/+branch/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "http://code.launchpad.net/+branch/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "https://code.launchpad.net/+branch/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "bzr+ssh://bazaar.launchpad.net/~charmers/charms/precise/wordpress/trunk"},
		{"cs:~charmers/precise/wordpress", "bzr+ssh://bazaar.launchpad.net/~charmers/charms/precise/wordpress/trunk/"},
		{"cs:~charmers/precise/wordpress", "~charmers/charms/precise/wordpress/trunk"},
		{"", "lp:~charmers/charms/precise/wordpress/whatever"},
		{"", "lp:~charmers/whatever/precise/wordpress/trunk"},
		{"", "lp:whatever/precise/wordpress"},
	}
	for _, t := range tests {
		charmURL, err := s.store.CharmURL(t.loc)
		if t.url == "" {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf("unknown branch location: %q", t.loc))
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(charmURL.String(), gc.Equals, t.url)
		}
	}
}

type LocalRepoSuite struct {
	testbase.LoggingSuite
	repo       *charm.LocalRepository
	seriesPath string
}

var _ = gc.Suite(&LocalRepoSuite{})

func (s *LocalRepoSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	root := c.MkDir()
	s.repo = &charm.LocalRepository{root}
	s.seriesPath = filepath.Join(root, "quantal")
	c.Assert(os.Mkdir(s.seriesPath, 0777), gc.IsNil)
}

func (s *LocalRepoSuite) addBundle(name string) string {
	return testing.Charms.BundlePath(s.seriesPath, name)
}

func (s *LocalRepoSuite) addDir(name string) string {
	return testing.Charms.ClonedDirPath(s.seriesPath, name)
}

func (s *LocalRepoSuite) checkNotFoundErr(c *gc.C, err error, charmURL *charm.URL) {
	expect := `charm not found in "` + s.repo.Path + `": ` + charmURL.String()
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *LocalRepoSuite) TestMissingCharm(c *gc.C) {
	for i, str := range []string{
		"local:quantal/zebra", "local:badseries/zebra",
	} {
		c.Logf("test %d: %s", i, str)
		charmURL := charm.MustParseURL(str)
		_, err := s.repo.Latest(charmURL)
		s.checkNotFoundErr(c, err, charmURL)
		_, err = s.repo.Get(charmURL)
		s.checkNotFoundErr(c, err, charmURL)
	}
}

func (s *LocalRepoSuite) TestMissingRepo(c *gc.C) {
	c.Assert(os.RemoveAll(s.repo.Path), gc.IsNil)
	_, err := s.repo.Latest(charm.MustParseURL("local:quantal/zebra"))
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*"`)
	_, err = s.repo.Get(charm.MustParseURL("local:quantal/zebra"))
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*"`)
	c.Assert(ioutil.WriteFile(s.repo.Path, nil, 0666), gc.IsNil)
	_, err = s.repo.Latest(charm.MustParseURL("local:quantal/zebra"))
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*"`)
	_, err = s.repo.Get(charm.MustParseURL("local:quantal/zebra"))
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*"`)
}

func (s *LocalRepoSuite) TestMultipleVersions(c *gc.C) {
	charmURL := charm.MustParseURL("local:quantal/upgrade")
	s.addDir("upgrade1")
	rev, err := s.repo.Latest(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 1)
	ch, err := s.repo.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)

	s.addDir("upgrade2")
	rev, err = s.repo.Latest(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 2)
	ch, err = s.repo.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 2)

	revCharmURL := charmURL.WithRevision(1)
	rev, err = s.repo.Latest(revCharmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 2)
	ch, err = s.repo.Get(revCharmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)

	badRevCharmURL := charmURL.WithRevision(33)
	rev, err = s.repo.Latest(badRevCharmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 2)
	_, err = s.repo.Get(badRevCharmURL)
	s.checkNotFoundErr(c, err, badRevCharmURL)
}

func (s *LocalRepoSuite) TestBundle(c *gc.C) {
	charmURL := charm.MustParseURL("local:quantal/dummy")
	s.addBundle("dummy")

	rev, err := s.repo.Latest(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 1)
	ch, err := s.repo.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
}

func (s *LocalRepoSuite) TestLogsErrors(c *gc.C) {
	err := ioutil.WriteFile(filepath.Join(s.seriesPath, "blah.charm"), nil, 0666)
	c.Assert(err, gc.IsNil)
	err = os.Mkdir(filepath.Join(s.seriesPath, "blah"), 0666)
	c.Assert(err, gc.IsNil)
	samplePath := s.addDir("upgrade2")
	gibberish := []byte("don't parse me by")
	err = ioutil.WriteFile(filepath.Join(samplePath, "metadata.yaml"), gibberish, 0666)
	c.Assert(err, gc.IsNil)

	charmURL := charm.MustParseURL("local:quantal/dummy")
	s.addDir("dummy")
	ch, err := s.repo.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
	c.Assert(c.GetTestLog(), gc.Matches, `
.* WARNING juju charm: failed to load charm at ".*/quantal/blah": .*
.* WARNING juju charm: failed to load charm at ".*/quantal/blah.charm": .*
.* WARNING juju charm: failed to load charm at ".*/quantal/upgrade2": .*
`[1:])
}

func renameSibling(c *gc.C, path, name string) {
	c.Assert(os.Rename(path, filepath.Join(filepath.Dir(path), name)), gc.IsNil)
}

func (s *LocalRepoSuite) TestIgnoresUnpromisingNames(c *gc.C) {
	err := ioutil.WriteFile(filepath.Join(s.seriesPath, "blah.notacharm"), nil, 0666)
	c.Assert(err, gc.IsNil)
	err = os.Mkdir(filepath.Join(s.seriesPath, ".blah"), 0666)
	c.Assert(err, gc.IsNil)
	renameSibling(c, s.addDir("dummy"), ".dummy")
	renameSibling(c, s.addBundle("dummy"), "dummy.notacharm")
	charmURL := charm.MustParseURL("local:quantal/dummy")

	_, err = s.repo.Get(charmURL)
	s.checkNotFoundErr(c, err, charmURL)
	_, err = s.repo.Latest(charmURL)
	s.checkNotFoundErr(c, err, charmURL)
	c.Assert(c.GetTestLog(), gc.Equals, "")
}

func (s *LocalRepoSuite) TestFindsSymlinks(c *gc.C) {
	realPath := testing.Charms.ClonedDirPath(c.MkDir(), "dummy")
	linkPath := filepath.Join(s.seriesPath, "dummy")
	err := os.Symlink(realPath, linkPath)
	c.Assert(err, gc.IsNil)
	ch, err := s.repo.Get(charm.MustParseURL("local:quantal/dummy"))
	c.Assert(err, gc.IsNil)
	checkDummy(c, ch, linkPath)
}
