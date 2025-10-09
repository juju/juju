// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"testing"

	"github.com/juju/tc"

	internalstorage "github.com/juju/juju/internal/storage"
)

// storageRegistrySuite is a testing suite for asserting the behaviour of the
// storage.ProviderRegistry implementation on the maas environ.
type storageRegistrySuite struct {
	maasSuite
}

func TestStorageRegistrySuite(t *testing.T) {
	tc.Run(t, &storageRegistrySuite{})
}

func (s *storageRegistrySuite) TestRecommendedPoolForKind(c *tc.C) {
	env := s.maasSuite.makeEnviron(c, newFakeController())
	bPool := env.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	c.Check(bPool.Name(), tc.Equals, "maas")
	c.Check(bPool.Provider().String(), tc.Equals, "maas")

	fPool := env.RecommendedPoolForKind(internalstorage.StorageKindFilesystem)
	c.Check(fPool.Name(), tc.Equals, "maas")
	c.Check(fPool.Provider().String(), tc.Equals, "maas")
}
