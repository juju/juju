// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"

	// Ensure environ providers are registered.
	_ "github.com/juju/juju/provider/all"
)

type providerRegistrySuite struct{}

var _ = gc.Suite(&providerRegistrySuite{})

type mockProvider struct {
	storage.Provider
}

func (s *providerRegistrySuite) TestRegisterProvider(c *gc.C) {
	p1 := &mockProvider{}
	ptype := storage.ProviderType("foo")
	provider.RegisterProvider(ptype, p1)
	p, err := provider.StorageProvider(ptype)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, p1)
}

func (s *providerRegistrySuite) TestNoSuchProvider(c *gc.C) {
	_, err := provider.StorageProvider(storage.ProviderType("foo"))
	c.Assert(err, gc.ErrorMatches, `storage provider "foo" not found`)
}

func (s *providerRegistrySuite) TestRegisterProviderDuplicate(c *gc.C) {
	defer func() {
		if v := recover(); v != nil {
			c.Assert(v, gc.ErrorMatches, `.*duplicate storage provider type "foo"`)
		}
	}()
	p1 := &mockProvider{}
	p2 := &mockProvider{}
	provider.RegisterProvider(storage.ProviderType("foo"), p1)
	provider.RegisterProvider(storage.ProviderType("foo"), p2)
	c.Errorf("panic expected")
}

func (s *providerRegistrySuite) TestSupportedEnvironProviders(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	ptypeBar := storage.ProviderType("bar")
	provider.RegisterEnvironStorageProviders("ec2", ptypeFoo, ptypeBar)
	c.Assert(provider.IsProviderSupported("ec2", ptypeFoo), jc.IsTrue)
	c.Assert(provider.IsProviderSupported("ec2", ptypeBar), jc.IsTrue)
	c.Assert(provider.IsProviderSupported("ec2", storage.ProviderType("foobar")), jc.IsFalse)
	c.Assert(provider.IsProviderSupported("openstack", ptypeBar), jc.IsFalse)
}

func (s *providerRegistrySuite) TestSupportedEnvironCommonProviders(c *gc.C) {
	for _, envProvider := range environs.RegisteredProviders() {
		for _, storageProvider := range provider.CommonProviders {
			c.Logf("Checking storage provider %v is registered for env provider %v", storageProvider, envProvider)
			c.Assert(provider.IsProviderSupported(envProvider, storageProvider), jc.IsTrue)
		}
	}
}

func (s *providerRegistrySuite) TestRegisterEnvironProvidersMultipleCalls(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	ptypeBar := storage.ProviderType("bar")
	provider.RegisterEnvironStorageProviders("ec2", ptypeFoo)
	provider.RegisterEnvironStorageProviders("ec2", ptypeBar)
	provider.RegisterEnvironStorageProviders("ec2", ptypeBar)
	c.Assert(provider.IsProviderSupported("ec2", ptypeFoo), jc.IsTrue)
	c.Assert(provider.IsProviderSupported("ec2", ptypeBar), jc.IsTrue)
}

func (s *providerRegistrySuite) TestDefaultPool(c *gc.C) {
	provider.RegisterDefaultPool("ec2", storage.StorageKindBlock, "ebs")
	provider.RegisterDefaultPool("ec2", storage.StorageKindFilesystem, "nfs")
	provider.RegisterDefaultPool("local", storage.StorageKindFilesystem, "nfs")
	pool, ok := provider.DefaultPool("ec2", storage.StorageKindBlock)
	c.Assert(ok, jc.IsTrue)
	c.Assert(pool, gc.Equals, "ebs")
	pool, ok = provider.DefaultPool("ec2", storage.StorageKindFilesystem)
	c.Assert(ok, jc.IsTrue)
	c.Assert(pool, gc.Equals, "nfs")
	pool, ok = provider.DefaultPool("local", storage.StorageKindBlock)
	c.Assert(ok, jc.IsFalse)
	pool, ok = provider.DefaultPool("maas", storage.StorageKindBlock)
	c.Assert(ok, jc.IsFalse)
}
