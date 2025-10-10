// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	"testing"

	"github.com/juju/tc"

	internalstorage "github.com/juju/juju/internal/storage"
)

// storageRegistrySuite is a testing suite for asserting the behaviour of the
// storage.ProviderRegistry implementation on the oci environ.
type storageRegistrySuite struct {
	commonSuite
}

func TestStorageRegistrySuite(t *testing.T) {
	tc.Run(t, &storageRegistrySuite{})
}

func (s *storageRegistrySuite) TestRecommendedPoolForKind(c *tc.C) {
	bPool := s.env.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	c.Check(bPool.Name(), tc.Equals, "iscsi")
	c.Check(bPool.Provider().String(), tc.Equals, "oci")

	fsPool := s.env.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	c.Check(fsPool.Name(), tc.Equals, "iscsi")
	c.Check(fsPool.Provider().String(), tc.Equals, "oci")
}
