// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"bytes"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/storage"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/upgrades"
)

type migrateCustomImageMetadataStorageSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&migrateCustomImageMetadataStorageSuite{})

func (s *migrateCustomImageMetadataStorageSuite) TestMigrateCustomImageMetadata(c *gc.C) {
	stor := s.Environ.(environs.EnvironStorage).Storage()
	for path, content := range customImageMetadata {
		err := stor.Put(path, bytes.NewReader(content), int64(len(content)))
		c.Assert(err, jc.ErrorIsNil)
	}
	s.testMigrateCustomImageMetadata(c, &mockAgentConfig{})
}

func (s *migrateCustomImageMetadataStorageSuite) TestMigrateCustomImageMetadataLocalstorage(c *gc.C) {
	storageDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	for path, content := range customImageMetadata {
		err := stor.Put(path, bytes.NewReader(content), int64(len(content)))
		c.Assert(err, jc.ErrorIsNil)
	}
	s.testMigrateCustomImageMetadata(c, &mockAgentConfig{
		values: map[string]string{
			agent.ProviderType: "local",
			agent.StorageDir:   storageDir,
		},
	})
}

func (s *migrateCustomImageMetadataStorageSuite) testMigrateCustomImageMetadata(c *gc.C, agentConfig agent.Config) {
	var stor statetesting.MapStorage
	s.PatchValue(upgrades.NewStateStorage, func(string, *mgo.Session) storage.Storage {
		return &stor
	})
	err := upgrades.MigrateCustomImageMetadata(s.State, agentConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stor.Map, gc.DeepEquals, customImageMetadata)
}
