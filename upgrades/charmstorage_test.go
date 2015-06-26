// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
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
	stor := s.Environ.(environs.EnvironStorage).Storage()
	err := stor.Put("somewhere", strings.NewReader("abc"), 3)
	c.Assert(err, jc.ErrorIsNil)

	dummyCharm := s.AddTestingCharm(c, "dummy")
	dummyCharmURL, err := stor.URL("somewhere")
	c.Assert(err, jc.ErrorIsNil)
	url, err := url.Parse(dummyCharmURL)
	c.Assert(err, jc.ErrorIsNil)
	s.bundleURLs[dummyCharm.URL().String()] = url

	s.testMigrateCharmStorage(c, dummyCharm.URL(), &mockAgentConfig{})
}

func (s *migrateCharmStorageSuite) TestMigrateCharmStorageLocalstorage(c *gc.C) {
	storageDir := c.MkDir()
	err := ioutil.WriteFile(filepath.Join(storageDir, "somewhere"), []byte("abc"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	dummyCharm := s.AddTestingCharm(c, "dummy")
	url := &url.URL{Scheme: "https", Host: "localhost:8040", Path: "/somewhere"}
	c.Assert(err, jc.ErrorIsNil)
	s.bundleURLs[dummyCharm.URL().String()] = url

	s.testMigrateCharmStorage(c, dummyCharm.URL(), &mockAgentConfig{
		values: map[string]string{
			agent.ProviderType: "local",
			agent.StorageDir:   storageDir,
		},
	})
}

func (s *migrateCharmStorageSuite) testMigrateCharmStorage(c *gc.C, curl *charm.URL, agentConfig agent.Config) {
	curlPlaceholder := charm.MustParseURL("cs:quantal/dummy-1")
	err := s.State.AddStoreCharmPlaceholder(curlPlaceholder)
	c.Assert(err, jc.ErrorIsNil)

	curlPending := charm.MustParseURL("cs:quantal/missing-123")
	_, err = s.State.PrepareStoreCharmUpload(curlPending)
	c.Assert(err, jc.ErrorIsNil)

	var storagePath string
	var called bool
	s.PatchValue(upgrades.StateAddCharmStoragePaths, func(st *state.State, storagePaths map[*charm.URL]string) error {
		c.Assert(storagePaths, gc.HasLen, 1)
		for k, v := range storagePaths {
			c.Assert(k.String(), gc.Equals, curl.String())
			storagePath = v
		}
		called = true
		return nil
	})
	err = upgrades.MigrateCharmStorage(s.State, agentConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)

	storage := storage.NewStorage(s.State.EnvironUUID(), s.State.MongoSession())
	r, length, err := storage.Get(storagePath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.NotNil)
	defer r.Close()
	c.Assert(length, gc.Equals, int64(3))
	data, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)
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
	err := upgrades.MigrateCharmStorage(s.State, &mockAgentConfig{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
