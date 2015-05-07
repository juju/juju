// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	// Ensure environ providers are registered.
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

type providerRegistrySuite struct{}

var _ = gc.Suite(&providerRegistrySuite{})

type mockProvider struct {
	storage.Provider
}

func (s *providerRegistrySuite) TestRegisterProvider(c *gc.C) {
	p1 := &mockProvider{}
	ptype := storage.ProviderType("foo")
	registry.RegisterProvider(ptype, p1)
	p, err := registry.StorageProvider(ptype)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, p1)
}

func (s *providerRegistrySuite) TestUnregisterProvider(c *gc.C) {
	ptype := storage.ProviderType("foo")

	// No-op, since there's nothing registered yet.
	registry.RegisterProvider(ptype, nil)

	// Register and then unregister, ensure that the provider cannot
	// be accessed.
	registry.RegisterProvider(ptype, &mockProvider{})
	registry.RegisterProvider(ptype, nil)
	_, err := registry.StorageProvider(storage.ProviderType("foo"))
	c.Assert(err, gc.ErrorMatches, `storage provider "foo" not found`)
}

func (s *providerRegistrySuite) TestNoSuchProvider(c *gc.C) {
	_, err := registry.StorageProvider(storage.ProviderType("foo"))
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
	registry.RegisterProvider(storage.ProviderType("foo"), p1)
	registry.RegisterProvider(storage.ProviderType("foo"), p2)
	c.Errorf("panic expected")
}

func (s *providerRegistrySuite) TestSupportedEnvironProviders(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	ptypeBar := storage.ProviderType("bar")
	registry.RegisterEnvironStorageProviders("ec2", ptypeFoo, ptypeBar)
	c.Assert(registry.IsProviderSupported("ec2", ptypeFoo), jc.IsTrue)
	c.Assert(registry.IsProviderSupported("ec2", ptypeBar), jc.IsTrue)
	c.Assert(registry.IsProviderSupported("ec2", storage.ProviderType("foobar")), jc.IsFalse)
	c.Assert(registry.IsProviderSupported("openstack", ptypeBar), jc.IsFalse)
}

func (s *providerRegistrySuite) TestSupportedEnvironCommonProviders(c *gc.C) {
	for _, envProvider := range environs.RegisteredProviders() {
		for storageProvider := range provider.CommonProviders() {
			c.Logf("Checking storage provider %v is registered for env provider %v", storageProvider, envProvider)
			c.Assert(registry.IsProviderSupported(envProvider, storageProvider), jc.IsTrue)
		}
	}
}

func (s *providerRegistrySuite) TestRegisterEnvironProvidersMultipleCalls(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	ptypeBar := storage.ProviderType("bar")
	registry.RegisterEnvironStorageProviders("ec2", ptypeFoo)
	registry.RegisterEnvironStorageProviders("ec2", ptypeBar)
	registry.RegisterEnvironStorageProviders("ec2", ptypeBar)
	c.Assert(registry.IsProviderSupported("ec2", ptypeFoo), jc.IsTrue)
	c.Assert(registry.IsProviderSupported("ec2", ptypeBar), jc.IsTrue)
}

func (s *providerRegistrySuite) TestListEnvProviderUnknownEnv(c *gc.C) {
	all, exists := registry.EnvironStorageProviders("fluffy")
	c.Assert(exists, jc.IsFalse)
	c.Assert(all, gc.IsNil)
}

func (s *providerRegistrySuite) TestListEnvProviderKnownEnv(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	registry.RegisterEnvironStorageProviders("ec2", ptypeFoo)
	all, exists := registry.EnvironStorageProviders("ec2")
	c.Assert(exists, jc.IsTrue)
	c.Assert(len(all) > 0, jc.IsTrue)

	found := false
	for _, one := range all {
		if one == ptypeFoo {
			found = true
			break
		}
	}
	c.Assert(found, jc.IsTrue)
}
