// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"net/url"
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// storageRegistrySuite is a testing suite for asserting the behaviour of the
// storage.ProviderRegistry implementation on the maas environ.
type storageRegistrySuite struct {
}

func TestStorageRegistrySuite(t *testing.T) {
	tc.Run(t, &storageRegistrySuite{})
}

// TestRecommendedPoolForKindWithCinder ensures that when cinder is available to
// the environ that the cinder pool is the recommended storage for filesystem
// and block storage.
func (s *storageRegistrySuite) TestRecommendedPoolForKindWithCinder(c *tc.C) {
	volumeURL, err := url.Parse("https://cinder.example.com")
	c.Assert(err, tc.IsNil)
	env := &Environ{
		volumeURL: volumeURL,
	}

	bPool := env.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	c.Check(bPool.Name(), tc.Equals, "cinder")
	c.Check(bPool.Provider().String(), tc.Equals, "cinder")

	fPool := env.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	c.Check(fPool.Name(), tc.Equals, "cinder")
	c.Check(fPool.Provider().String(), tc.Equals, "cinder")
}

// TestRecommendedPoolForKindWithoutCinder ensures that if cinder is not
// available to the environ that it is not one of the returned pools for either
// filesystem or block storage.
func (s *storageRegistrySuite) TestRecommendedPoolForKindWithoutCinder(c *tc.C) {
	env := &Environ{}

	bPool := env.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	if bPool != nil {
		c.Check(bPool.Name(), tc.Not(tc.Equals), "cinder")
		c.Check(bPool.Provider().String(), tc.Not(tc.Equals), "cinder")
	}

	fPool := env.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	if fPool != nil {
		c.Check(fPool.Name(), tc.Not(tc.Equals), "cinder")
		c.Check(fPool.Provider().String(), tc.Not(tc.Equals), "cinder")
	}
}

// TestStorageProviderTypesHasCinder tests that when the environ has a volume
// URL cinder is returned as one of the storage provider types from the
// registry.
func (s *storageRegistrySuite) TestStorageProviderTypesHasCinder(c *tc.C) {
	volumeURL, err := url.Parse("https://cinder.example.com")
	c.Assert(err, tc.IsNil)

	env := &Environ{
		volumeURL: volumeURL,
	}
	types, err := env.StorageProviderTypes()
	c.Check(err, tc.ErrorIsNil)
	c.Check(types, tc.SameContents, []internalstorage.ProviderType{
		"cinder",
		"loop",
		"tmpfs",
		"rootfs",
	})
}

// TestStorageProviderTypesCinderNotSupported tests that when the environ does
// not support Cinder storage it does not come out as one of the storage
// provider types available.
func (s *storageRegistrySuite) TestStorageProviderTypesCinderNotSupported(c *tc.C) {
	env := &Environ{}
	types, err := env.StorageProviderTypes()
	c.Check(err, tc.ErrorIsNil)
	c.Check(types, tc.SameContents, []internalstorage.ProviderType{
		"loop",
		"tmpfs",
		"rootfs",
	})
}

// TestCinderStorageProviderNotSupported ensures that if we ask of the environ
// for the cinder storage provider but it isn't supported by the environ we
// get back an error satisfying [coreerrors.NotSupported].
func (s *storageRegistrySuite) TestCinderStorageProviderNotSupported(c *tc.C) {
	env := &Environ{}
	_, err := env.StorageProvider(internalstorage.ProviderType("cinder"))
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}
