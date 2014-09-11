// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"net/url"
	"strings"

	jc "github.com/juju/testing/checkers"
	"gopkg.in/juju/charm.v3"
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
)

type migrateCharmStorageSuite struct {
	jujutesting.JujuConnSuite

	bundleURLs map[string]*url.URL
}

var _ = gc.Suite(&migrateCharmStorageSuite{})

func (s *migrateCharmStorageSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.bundleURLs = make(map[string]*url.URL)

	s.PatchValue(upgrades.CharmBundleURL, func(ch *state.Charm) *url.URL {
		return s.bundleURLs[ch.URL().String()]
	})
	s.PatchValue(upgrades.CharmStoragePath, func(ch *state.Charm) string {
		// pretend none of the charms have storage paths
		return ""
	})
}

func (s *migrateCharmStorageSuite) TestMigrateCharmStorage(c *gc.C) {
	err := s.Environ.Storage().Put("somewhere", strings.NewReader("abc"), 3)
	c.Assert(err, gc.IsNil)

	dummyCharm := s.AddTestingCharm(c, "dummy")
	dummyCharmURL, err := s.Environ.Storage().URL("somewhere")
	c.Assert(err, gc.IsNil)
	url, err := url.Parse(dummyCharmURL)
	c.Assert(err, gc.IsNil)
	s.bundleURLs[dummyCharm.URL().String()] = url

	curlPlaceholder := charm.MustParseURL("cs:quantal/dummy-1")
	err = s.State.AddStoreCharmPlaceholder(curlPlaceholder)
	c.Assert(err, gc.IsNil)

	curlPending := charm.MustParseURL("cs:quantal/missing-123")
	_, err = s.State.PrepareStoreCharmUpload(curlPending)
	c.Assert(err, gc.IsNil)

	var storagePath string
	var called bool
	s.PatchValue(upgrades.StateAddCharmStoragePaths, func(st *state.State, storagePaths map[*charm.URL]string) error {
		c.Assert(storagePaths, gc.HasLen, 1)
		for k, v := range storagePaths {
			c.Assert(k.String(), gc.Equals, dummyCharm.URL().String())
			storagePath = v
		}
		called = true
		return nil
	})
	err = upgrades.MigrateCharmStorage(s.State)
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.IsTrue)

	storage, err := s.State.Storage()
	c.Assert(err, gc.IsNil)
	defer storage.Close()
	r, length, err := storage.Get(storagePath)
	c.Assert(err, gc.IsNil)
	c.Assert(r, gc.NotNil)
	defer r.Close()
	c.Assert(length, gc.Equals, int64(3))
	data, err := ioutil.ReadAll(r)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "abc")
}

func (s *migrateCharmStorageSuite) TestMigrateCharmStorageIdempotency(c *gc.C) {
	// If MigrateCharmStorage is called a second time, it will
	// leave alone the charms that have already been migrated.
	// The final step of migration is a transactional update
	// of the charm document in state, which is what we base
	// the decision on.
	s.PatchValue(upgrades.CharmStoragePath, func(ch *state.Charm) string {
		return "alreadyset"
	})
	s.AddTestingCharm(c, "dummy")
	var called bool
	s.PatchValue(upgrades.StateAddCharmStoragePaths, func(st *state.State, storagePaths map[*charm.URL]string) error {
		c.Assert(storagePaths, gc.HasLen, 0)
		called = true
		return nil
	})
	err := upgrades.MigrateCharmStorage(s.State)
	c.Assert(err, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}
