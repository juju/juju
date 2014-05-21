// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	charmtesting "launchpad.net/juju-core/charm/testing"
	env_config "launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
)

type StoreSuite struct {
	testing.FakeHomeSuite
	server *charmtesting.MockStore
	store  *charm.CharmStore
}

var _ = gc.Suite(&StoreSuite{})

func (s *StoreSuite) SetUpSuite(c *gc.C) {
	s.FakeHomeSuite.SetUpSuite(c)
	s.server = charmtesting.NewMockStore(c, map[string]int{
		"cs:series/good":   23,
		"cs:series/unwise": 23,
		"cs:series/better": 24,
		"cs:series/best":   25,
	})
}

func (s *StoreSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.PatchValue(&charm.CacheDir, c.MkDir())
	s.store = charm.NewStore(s.server.Address())
	s.server.Downloads = nil
	s.server.Authorizations = nil
	s.server.Metadata = nil
	s.server.DownloadsNoStats = nil
	s.server.InfoRequestCount = 0
	s.server.InfoRequestCountNoStats = 0
}

func (s *StoreSuite) TearDownSuite(c *gc.C) {
	s.server.Close()
	s.FakeHomeSuite.TearDownSuite(c)
}

func (s *StoreSuite) TestMissing(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/missing")
	expect := `charm not found: cs:series/missing`
	_, err := charm.Latest(s.store, charmURL)
	c.Assert(err, gc.ErrorMatches, expect)
	_, err = s.store.Get(charmURL)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *StoreSuite) TestError(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/borken")
	expect := `charm info errors for "cs:series/borken": badness`
	_, err := charm.Latest(s.store, charmURL)
	c.Assert(err, gc.ErrorMatches, expect)
	_, err = s.store.Get(charmURL)
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *StoreSuite) TestWarning(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/unwise")
	expect := `.* WARNING juju.charm charm store reports for "cs:series/unwise": foolishness` + "\n"
	r, err := charm.Latest(s.store, charmURL)
	c.Assert(r, gc.Equals, 23)
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, expect)
	ch, err := s.store.Get(charmURL)
	c.Assert(ch, gc.NotNil)
	c.Assert(err, gc.IsNil)
	c.Assert(c.GetTestLog(), gc.Matches, expect+expect)
}

func (s *StoreSuite) TestLatest(c *gc.C) {
	urls := []*charm.URL{
		charm.MustParseURL("cs:series/good"),
		charm.MustParseURL("cs:series/good-2"),
		charm.MustParseURL("cs:series/good-99"),
	}
	revInfo, err := s.store.Latest(urls...)
	c.Assert(err, gc.IsNil)
	c.Assert(revInfo, gc.DeepEquals, []charm.CharmRevision{
		{23, "2c9f01a53a73c221d5360207e7bb2f887ff83c32b04e58aca76c4d99fd071ec7", nil},
		{23, "2c9f01a53a73c221d5360207e7bb2f887ff83c32b04e58aca76c4d99fd071ec7", nil},
		{23, "2c9f01a53a73c221d5360207e7bb2f887ff83c32b04e58aca76c4d99fd071ec7", nil},
	})
}

func (s *StoreSuite) assertCached(c *gc.C, charmURL *charm.URL) {
	s.server.Downloads = nil
	ch, err := s.store.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.Downloads, gc.IsNil)
}

func (s *StoreSuite) TestGetCacheImplicitRevision(c *gc.C) {
	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	revCharmURL := charm.MustParseURL(base + "-23")
	ch, err := s.store.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.Downloads, gc.DeepEquals, []*charm.URL{revCharmURL})
	s.assertCached(c, charmURL)
	s.assertCached(c, revCharmURL)
}

func (s *StoreSuite) TestGetCacheExplicitRevision(c *gc.C) {
	base := "cs:series/good-12"
	charmURL := charm.MustParseURL(base)
	ch, err := s.store.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.Downloads, gc.DeepEquals, []*charm.URL{charmURL})
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
	c.Assert(s.server.Downloads, gc.DeepEquals, []*charm.URL{revCharmURL})
	s.assertCached(c, charmURL)
	s.assertCached(c, revCharmURL)
}

func (s *StoreSuite) TestGetTestModeFlag(c *gc.C) {
	base := "cs:series/good-12"
	charmURL := charm.MustParseURL(base)
	ch, err := s.store.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.Downloads, gc.DeepEquals, []*charm.URL{charmURL})
	c.Assert(s.server.DownloadsNoStats, gc.IsNil)
	c.Assert(s.server.InfoRequestCount, gc.Equals, 1)
	c.Assert(s.server.InfoRequestCountNoStats, gc.Equals, 0)

	storeInTestMode := s.store.WithTestMode(true)
	other := "cs:series/good-23"
	otherURL := charm.MustParseURL(other)
	ch, err = storeInTestMode.Get(otherURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch, gc.NotNil)
	c.Assert(s.server.Downloads, gc.DeepEquals, []*charm.URL{charmURL})
	c.Assert(s.server.DownloadsNoStats, gc.DeepEquals, []*charm.URL{otherURL})
	c.Assert(s.server.InfoRequestCount, gc.Equals, 1)
	c.Assert(s.server.InfoRequestCountNoStats, gc.Equals, 1)
}

// The following tests cover the low-level CharmStore-specific API.

func (s *StoreSuite) TestInfo(c *gc.C) {
	charmURLs := []charm.Location{
		charm.MustParseURL("cs:series/good"),
		charm.MustParseURL("cs:series/better"),
		charm.MustParseURL("cs:series/best"),
	}
	infos, err := s.store.Info(charmURLs...)
	c.Assert(err, gc.IsNil)
	c.Assert(infos, gc.HasLen, 3)
	expected := []int{23, 24, 25}
	for i, info := range infos {
		c.Assert(info.Errors, gc.IsNil)
		c.Assert(info.Revision, gc.Equals, expected[i])
	}
}

func (s *StoreSuite) TestInfoNotFound(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/missing")
	info, err := s.store.Info(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0].Errors, gc.HasLen, 1)
	c.Assert(info[0].Errors[0], gc.Matches, `charm not found: cs:series/missing`)
}

func (s *StoreSuite) TestInfoError(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/borken")
	info, err := s.store.Info(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0].Errors, gc.DeepEquals, []string{"badness"})
}

func (s *StoreSuite) TestInfoWarning(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/unwise")
	info, err := s.store.Info(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.HasLen, 1)
	c.Assert(info[0].Warnings, gc.DeepEquals, []string{"foolishness"})
}

func (s *StoreSuite) TestInfoTestModeFlag(c *gc.C) {
	charmURL := charm.MustParseURL("cs:series/good")
	_, err := s.store.Info(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(s.server.InfoRequestCount, gc.Equals, 1)
	c.Assert(s.server.InfoRequestCountNoStats, gc.Equals, 0)

	storeInTestMode, ok := s.store.WithTestMode(true).(*charm.CharmStore)
	c.Assert(ok, gc.Equals, true)
	_, err = storeInTestMode.Info(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(s.server.InfoRequestCount, gc.Equals, 1)
	c.Assert(s.server.InfoRequestCountNoStats, gc.Equals, 1)
}

func (s *StoreSuite) TestInfoDNSError(c *gc.C) {
	store := charm.NewStore("http://127.1.2.3")
	charmURL := charm.MustParseURL("cs:series/good")
	resp, err := store.Info(charmURL)
	c.Assert(resp, gc.IsNil)
	expect := `Cannot access the charm store. .*`
	c.Assert(err, gc.ErrorMatches, expect)
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
	store := env_config.SpecializeCharmRepo(s.store, config)

	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	_, err := store.Get(charmURL)

	c.Assert(err, gc.IsNil)

	c.Assert(s.server.Authorizations, gc.HasLen, 1)
	c.Assert(s.server.Authorizations[0], gc.Equals, "charmstore token=value")
}

func (s *StoreSuite) TestNilAuthorization(c *gc.C) {
	config := testing.EnvironConfig(c)
	store := env_config.SpecializeCharmRepo(s.store, config)

	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	_, err := store.Get(charmURL)

	c.Assert(err, gc.IsNil)
	c.Assert(s.server.Authorizations, gc.HasLen, 0)
}

func (s *StoreSuite) TestMetadata(c *gc.C) {
	store := s.store.WithJujuAttrs("juju-metadata")

	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	_, err := store.Get(charmURL)

	c.Assert(err, gc.IsNil)
	c.Assert(s.server.Metadata, gc.HasLen, 1)
	c.Assert(s.server.Metadata[0], gc.Equals, "juju-metadata")
}

func (s *StoreSuite) TestNilMetadata(c *gc.C) {
	base := "cs:series/good"
	charmURL := charm.MustParseURL(base)
	_, err := s.store.Get(charmURL)

	c.Assert(err, gc.IsNil)
	c.Assert(s.server.Metadata, gc.HasLen, 0)
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
	testing.FakeHomeSuite
	repo       *charm.LocalRepository
	seriesPath string
}

var _ = gc.Suite(&LocalRepoSuite{})

func (s *LocalRepoSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	root := c.MkDir()
	s.repo = &charm.LocalRepository{Path: root}
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
		_, err := charm.Latest(s.repo, charmURL)
		s.checkNotFoundErr(c, err, charmURL)
		_, err = s.repo.Get(charmURL)
		s.checkNotFoundErr(c, err, charmURL)
	}
}

func (s *LocalRepoSuite) TestMissingRepo(c *gc.C) {
	c.Assert(os.RemoveAll(s.repo.Path), gc.IsNil)
	_, err := charm.Latest(s.repo, charm.MustParseURL("local:quantal/zebra"))
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*"`)
	_, err = s.repo.Get(charm.MustParseURL("local:quantal/zebra"))
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*"`)
	c.Assert(ioutil.WriteFile(s.repo.Path, nil, 0666), gc.IsNil)
	_, err = charm.Latest(s.repo, charm.MustParseURL("local:quantal/zebra"))
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*"`)
	_, err = s.repo.Get(charm.MustParseURL("local:quantal/zebra"))
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*"`)
}

func (s *LocalRepoSuite) TestMultipleVersions(c *gc.C) {
	charmURL := charm.MustParseURL("local:quantal/upgrade")
	s.addDir("upgrade1")
	rev, err := charm.Latest(s.repo, charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 1)
	ch, err := s.repo.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)

	s.addDir("upgrade2")
	rev, err = charm.Latest(s.repo, charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 2)
	ch, err = s.repo.Get(charmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 2)

	revCharmURL := charmURL.WithRevision(1)
	rev, err = charm.Latest(s.repo, revCharmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 2)
	ch, err = s.repo.Get(revCharmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)

	badRevCharmURL := charmURL.WithRevision(33)
	rev, err = charm.Latest(s.repo, badRevCharmURL)
	c.Assert(err, gc.IsNil)
	c.Assert(rev, gc.Equals, 2)
	_, err = s.repo.Get(badRevCharmURL)
	s.checkNotFoundErr(c, err, badRevCharmURL)
}

func (s *LocalRepoSuite) TestBundle(c *gc.C) {
	charmURL := charm.MustParseURL("local:quantal/dummy")
	s.addBundle("dummy")

	rev, err := charm.Latest(s.repo, charmURL)
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
.* WARNING juju.charm failed to load charm at ".*/quantal/blah": .*
.* WARNING juju.charm failed to load charm at ".*/quantal/blah.charm": .*
.* WARNING juju.charm failed to load charm at ".*/quantal/upgrade2": .*
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
	_, err = charm.Latest(s.repo, charmURL)
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
