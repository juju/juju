// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io"
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
)

type migrateToolsStorageSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&migrateToolsStorageSuite{})

func (s *migrateToolsStorageSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

var migrateToolsVersions = []version.Binary{
	version.MustParseBinary("1.2.3-precise-amd64"),
	version.MustParseBinary("2.3.4-trusty-ppc64el"),
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorageNoTools(c *gc.C) {
	fakeToolsStorage := &fakeToolsStorage{
		stored: make(map[version.Binary]toolstorage.Metadata),
	}
	s.PatchValue(upgrades.StateToolsStorage, func(*state.State) (toolstorage.StorageCloser, error) {
		return fakeToolsStorage, nil
	})

	stor := s.Environ.(environs.EnvironStorage).Storage()
	envtesting.RemoveFakeTools(c, stor)
	envtesting.RemoveFakeToolsMetadata(c, stor)
	err := upgrades.MigrateToolsStorage(s.State, &mockAgentConfig{})
	c.Assert(err, gc.IsNil)
	c.Assert(fakeToolsStorage.stored, gc.HasLen, 0)
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorage(c *gc.C) {
	stor := s.Environ.(environs.EnvironStorage).Storage()
	envtesting.RemoveFakeTools(c, stor, "released")
	envtesting.AssertUploadFakeToolsVersions(c, stor, "released", migrateToolsVersions...)
	s.testMigrateToolsStorage(c, &mockAgentConfig{})
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorageLocalstorage(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, gc.IsNil)
	envtesting.AssertUploadFakeToolsVersions(c, stor, "released", migrateToolsVersions...)
	for _, providerType := range []string{"local", "manual"} {
		s.testMigrateToolsStorage(c, &mockAgentConfig{
			values: map[string]string{
				agent.ProviderType: providerType,
				agent.StorageDir:   storageDir,
			},
		})
	}
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorageBadSHA256(c *gc.C) {
	stor := s.Environ.(environs.EnvironStorage).Storage()
	envtesting.AssertUploadFakeToolsVersions(c, stor, "released", migrateToolsVersions...)
	// Overwrite one of the tools archives with junk, so the hash does not match.
	err := stor.Put(
		envtools.StorageName(migrateToolsVersions[0], "released"),
		strings.NewReader("junk"),
		4,
	)
	c.Assert(err, gc.IsNil)
	err = upgrades.MigrateToolsStorage(s.State, &mockAgentConfig{})
	c.Assert(err, gc.ErrorMatches, "failed to fetch 1.2.3-precise-amd64 tools: hash mismatch")
}

func (s *migrateToolsStorageSuite) testMigrateToolsStorage(c *gc.C, agentConfig agent.Config) {
	fakeToolsStorage := &fakeToolsStorage{
		stored: make(map[version.Binary]toolstorage.Metadata),
	}
	s.PatchValue(upgrades.StateToolsStorage, func(*state.State) (toolstorage.StorageCloser, error) {
		return fakeToolsStorage, nil
	})
	err := upgrades.MigrateToolsStorage(s.State, agentConfig)
	c.Assert(err, gc.IsNil)
	c.Assert(fakeToolsStorage.stored, gc.DeepEquals, map[version.Binary]toolstorage.Metadata{
		migrateToolsVersions[0]: toolstorage.Metadata{
			Version: migrateToolsVersions[0],
			Size:    129,
			SHA256:  "f26c7a6832cc5fd3a01eaa46c79a7fa7f4714ff3263f7372cedb9470a7b40bae",
		},
		migrateToolsVersions[1]: toolstorage.Metadata{
			Version: migrateToolsVersions[1],
			Size:    129,
			SHA256:  "eba00d942f9f69e2c862c23095fdb2a0ff578c7c4e77cc28829fcc98cb152693",
		},
	})
}

type fakeToolsStorage struct {
	toolstorage.Storage
	stored map[version.Binary]toolstorage.Metadata
}

func (s *fakeToolsStorage) Close() error {
	return nil
}

func (s *fakeToolsStorage) AddTools(r io.Reader, meta toolstorage.Metadata) error {
	s.stored[meta.Version] = meta
	return nil
}
