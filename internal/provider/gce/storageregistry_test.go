package gce_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/gce"
	internalstorage "github.com/juju/juju/internal/storage"
)

// storageRegistrySuite is a testing suite for asserting the behaviour of the
// storage.ProviderRegistry implementation on the gce environ.
type storageRegistrySuite struct {
	gce.BaseSuite
}

func TestStorageRegistrySuite(t *testing.T) {
	tc.Run(t, &storageRegistrySuite{})
}

func (s *storageRegistrySuite) TestRecommendedPoolForKind(c *tc.C) {
	env := s.SetupEnv(c, nil)

	pBlock := env.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	c.Check(pBlock.Name(), tc.Equals, "gce")
	c.Check(pBlock.Provider().String(), tc.Equals, "gce")

	pFilesystem := env.RecommendedPoolForKind(internalstorage.StorageKindFilesystem)
	c.Check(pFilesystem.Name(), tc.Equals, "gce")
	c.Check(pFilesystem.Provider().String(), tc.Equals, "gce")
}
