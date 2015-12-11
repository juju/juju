// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"errors"
	"io"
	"strings"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
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
	envtesting.RemoveFakeTools(c, stor, "releases")
	envtesting.RemoveFakeToolsMetadata(c, stor)
	err := upgrades.MigrateToolsStorage(s.State, &mockAgentConfig{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fakeToolsStorage.stored, gc.HasLen, 0)
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorage(c *gc.C) {
	stor := s.Environ.(environs.EnvironStorage).Storage()
	envtesting.RemoveFakeTools(c, stor, "releases")
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "releases", "released", migrateToolsVersions...)
	s.testMigrateToolsStorage(c, &mockAgentConfig{}, tools)
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorageLocalstorage(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	tools := envtesting.AssertUploadFakeToolsVersions(c, stor, "releases", "released", migrateToolsVersions...)
	for _, providerType := range []string{"local", "manual"} {
		config := &mockAgentConfig{
			values: map[string]string{
				agent.ProviderType: providerType,
				agent.StorageDir:   storageDir,
			},
		}
		s.testMigrateToolsStorage(c, config, tools)
	}
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorageBadSHA256(c *gc.C) {
	fakeToolsStorage := s.installFakeToolsStorage()
	stor := s.Environ.(environs.EnvironStorage).Storage()
	envtesting.AssertUploadFakeToolsVersions(c, stor, "releases", "released", migrateToolsVersions...)
	// Overwrite one of the tools archives with junk, so the hash does not match.
	err := stor.Put(
		envtools.StorageName(migrateToolsVersions[0], "releases"),
		strings.NewReader("junk"),
		4,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = upgrades.MigrateToolsStorage(s.State, &mockAgentConfig{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(migrateToolsVersions[1], jc.Satisfies, fakeToolsStorage.contains)
	c.Assert(fakeToolsStorage.stored, gc.HasLen, 1)
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorageMissing(c *gc.C) {
	fakeToolsStorage := s.installFakeToolsStorage()
	stor := s.Environ.(environs.EnvironStorage).Storage()
	envtesting.AssertUploadFakeToolsVersions(c, stor, "releases", "released", migrateToolsVersions...)
	// Remove one of the tools archives (but not the metadata).
	err := stor.Remove(envtools.StorageName(migrateToolsVersions[0], "releases"))
	c.Assert(err, jc.ErrorIsNil)
	err = upgrades.MigrateToolsStorage(s.State, &mockAgentConfig{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(migrateToolsVersions[1], jc.Satisfies, fakeToolsStorage.contains)
	c.Assert(fakeToolsStorage.stored, gc.HasLen, 1)
}

func (s *migrateToolsStorageSuite) TestMigrateToolsStorageReadFails(c *gc.C) {
	fakeToolsStorage := s.installFakeToolsStorage()
	stor := s.Environ.(environs.EnvironStorage).Storage()
	envtesting.AssertUploadFakeToolsVersions(c, stor, "releases", "released", migrateToolsVersions...)

	storageErr := errors.New("no tools for you")
	dummy.Poison(stor, envtools.StorageName(migrateToolsVersions[0], "releases"), storageErr)

	err := upgrades.MigrateToolsStorage(s.State, &mockAgentConfig{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(migrateToolsVersions[1], jc.Satisfies, fakeToolsStorage.contains)
	c.Assert(fakeToolsStorage.stored, gc.HasLen, 1)
}

func (s *migrateToolsStorageSuite) testMigrateToolsStorage(c *gc.C, agentConfig agent.Config, tools []*coretools.Tools) {
	fakeToolsStorage := s.installFakeToolsStorage()
	err := upgrades.MigrateToolsStorage(s.State, agentConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fakeToolsStorage.stored, gc.DeepEquals, map[version.Binary]toolstorage.Metadata{
		tools[0].Version: toolstorage.Metadata{
			Version: tools[0].Version,
			Size:    tools[0].Size,
			SHA256:  tools[0].SHA256,
		},
		tools[1].Version: toolstorage.Metadata{
			Version: tools[1].Version,
			Size:    tools[1].Size,
			SHA256:  tools[1].SHA256,
		},
	})
}

func (s *migrateToolsStorageSuite) installFakeToolsStorage() *fakeToolsStorage {
	fakeToolsStorage := &fakeToolsStorage{
		stored: make(map[version.Binary]toolstorage.Metadata),
	}
	s.PatchValue(upgrades.StateToolsStorage, func(*state.State) (toolstorage.StorageCloser, error) {
		return fakeToolsStorage, nil
	})
	return fakeToolsStorage
}

type cleanToolsStorageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&cleanToolsStorageSuite{})

func (s *cleanToolsStorageSuite) TestCleanToolsStorage(c *gc.C) {
	fakeToolsStorage := &fakeToolsStorage{}
	s.PatchValue(upgrades.StateToolsStorage, func(*state.State) (toolstorage.StorageCloser, error) {
		return fakeToolsStorage, nil
	})
	fakeToolsStorage.SetErrors(errors.New("woop"))
	err := upgrades.CleanToolsStorage(nil)
	c.Assert(err, gc.ErrorMatches, "woop")
}

type fakeToolsStorage struct {
	gitjujutesting.Stub
	toolstorage.Storage
	stored map[version.Binary]toolstorage.Metadata
}

func (s *fakeToolsStorage) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s *fakeToolsStorage) AddTools(r io.Reader, meta toolstorage.Metadata) error {
	s.MethodCall(s, "AddTools", r, meta)
	s.stored[meta.Version] = meta
	return s.NextErr()
}

func (s *fakeToolsStorage) RemoveInvalid() error {
	s.MethodCall(s, "RemoveInvalid")
	return s.NextErr()
}

func (s *fakeToolsStorage) contains(v version.Binary) bool {
	_, ok := s.stored[v]
	return ok
}
