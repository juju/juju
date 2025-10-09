// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"testing"

	"github.com/go-goose/goose/v5/client"
	"github.com/go-goose/goose/v5/identity"
	"github.com/juju/tc"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/storage"
	internalstorage "github.com/juju/juju/internal/storage"
)

// storageRegistrySuite is a testing suite for asserting the behaviour of the
// storage.ProviderRegistry implementation on the maas environ.
type storageRegistrySuite struct {
}

type testAuthClient struct {
	client.AuthenticatingClient
	regionEndpoints map[string]identity.ServiceURLs
}

func (r *testAuthClient) IsAuthenticated() bool {
	return true
}

func (r *testAuthClient) TenantId() string {
	return "tenant-id"
}

func (r *testAuthClient) EndpointsForRegion(region string) identity.ServiceURLs {
	return r.regionEndpoints[region]
}

func TestStorageRegistrySuite(t *testing.T) {
	tc.Run(t, &storageRegistrySuite{})
}

// TestRecommendedPoolForKindWithCinder ensures that when cinder is available to
// the environ that the cinder pool is the recommended storage for filesystem
// and block storage.
func (s *storageRegistrySuite) TestRecommendedPoolForKindWithCinder(c *tc.C) {
	env := &Environ{
		cloudUnlocked: environscloudspec.CloudSpec{
			Region: "foo",
		},
		clientUnlocked: &testAuthClient{
			regionEndpoints: map[string]identity.ServiceURLs{
				"foo": {"volumev2": "https://bar.invalid"},
			},
		},
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
	env := &Environ{clientUnlocked: &testAuthClient{}}

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

func (s *storageRegistrySuite) TestStorageProviderTypes(c *tc.C) {
	env := &Environ{
		cloudUnlocked: environscloudspec.CloudSpec{
			Region: "foo",
		},
		clientUnlocked: &testAuthClient{
			regionEndpoints: map[string]identity.ServiceURLs{
				"foo": {"volumev2": "https://bar.invalid"},
			},
		}}
	types, err := env.StorageProviderTypes()
	c.Check(err, tc.ErrorIsNil)
	c.Check(types, tc.SameContents, []storage.ProviderType{
		"cinder",
		"loop",
		"tmpfs",
		"rootfs",
	})
}

// TestStorageProviderTypesNotSupported tests that when the environ does not
// support Cinder storage it does not come out as one of the storage provider
// types available.
func (s *storageRegistrySuite) TestStorageProviderTypesNotSupported(c *tc.C) {
	env := &Environ{clientUnlocked: &testAuthClient{}}
	types, err := env.StorageProviderTypes()
	c.Check(err, tc.ErrorIsNil)
	c.Check(types, tc.SameContents, []storage.ProviderType{
		"loop",
		"tmpfs",
		"rootfs",
	})
}
