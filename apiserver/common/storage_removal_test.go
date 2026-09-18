// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/rpc/params"
)

// storageEntitiesSuite tests [common.StorageEntities].
type storageEntitiesSuite struct{}

// TestStorageEntitiesSuite runs all tests contained within
// [storageEntitiesSuite].
func TestStorageEntitiesSuite(t *testing.T) {
	tc.Run(t, &storageEntitiesSuite{})
}

// TestStorageEntitiesEmpty asserts that no instances render to a nil
// entity slice, preserving the pre-existing empty destroy results.
func (s *storageEntitiesSuite) TestStorageEntitiesEmpty(c *tc.C) {
	c.Check(common.StorageEntities(nil), tc.IsNil)
	c.Check(common.StorageEntities([]domainstorage.StorageInstanceClassification{}), tc.IsNil)
}

// TestStorageEntities asserts that storage instance classifications are
// rendered as entities tagged with their storage identifier.
func (s *storageEntitiesSuite) TestStorageEntities(c *tc.C) {
	detachable := domainstorage.StorageInstanceClassification{
		Detachable: true,
		ID:         "lxd-fs/0",
		UUID:       tc.Must(c, domainstorage.NewStorageInstanceUUID),
	}
	nonDetachable := domainstorage.StorageInstanceClassification{
		Detachable: false,
		ID:         "loop-vol/0",
		UUID:       tc.Must(c, domainstorage.NewStorageInstanceUUID),
	}

	c.Check(common.StorageEntities([]domainstorage.StorageInstanceClassification{
		detachable, nonDetachable,
	}), tc.DeepEquals, []params.Entity{
		{Tag: "storage-lxd-fs-0"},
		{Tag: "storage-loop-vol-0"},
	})
}
