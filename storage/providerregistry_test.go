// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

type providerRegistrySuite struct{}

var _ = gc.Suite(&providerRegistrySuite{})

type mockProvider struct {
}

func (p *mockProvider) VolumeSource(*config.Config, *storage.Config) (storage.VolumeSource, error) {
	return nil, errors.New("not implemented")
}

func (p *mockProvider) ValidateConfig(*storage.Config) error {
	return nil
}

func (s *providerRegistrySuite) TestRegisterProvider(c *gc.C) {
	p1 := &mockProvider{}
	ptype := storage.ProviderType("foo")
	storage.RegisterProvider(ptype, p1)
	p, err := storage.StorageProvider(ptype)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, p1)
}

func (s *providerRegistrySuite) TestNoSuchProvider(c *gc.C) {
	_, err := storage.StorageProvider(storage.ProviderType("foo"))
	c.Assert(err, gc.ErrorMatches, `no registered provider for "foo"`)
}

func (s *providerRegistrySuite) TestRegisterProviderDuplicate(c *gc.C) {
	defer func() {
		if v := recover(); v != nil {
			c.Assert(v, gc.ErrorMatches, `.*duplicate storage provider type "foo"`)
		}
	}()
	p1 := &mockProvider{}
	p2 := &mockProvider{}
	storage.RegisterProvider(storage.ProviderType("foo"), p1)
	storage.RegisterProvider(storage.ProviderType("foo"), p2)
	c.Errorf("panic expected")
}

func (s *providerRegistrySuite) TestSupportedEnvironProviders(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	ptypeBar := storage.ProviderType("bar")
	storage.RegisterEnvironStorageProviders("ec2", ptypeFoo, ptypeBar)
	c.Assert(storage.IsProviderSupported("ec2", ptypeFoo), jc.IsTrue)
	c.Assert(storage.IsProviderSupported("ec2", ptypeBar), jc.IsTrue)
	c.Assert(storage.IsProviderSupported("ec2", storage.ProviderType("foobar")), jc.IsFalse)
	c.Assert(storage.IsProviderSupported("openstack", ptypeBar), jc.IsFalse)
}

func (s *providerRegistrySuite) TestRegisterEnvironProvidersMultipleCalls(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	ptypeBar := storage.ProviderType("bar")
	storage.RegisterEnvironStorageProviders("ec2", ptypeFoo)
	storage.RegisterEnvironStorageProviders("ec2", ptypeBar)
	storage.RegisterEnvironStorageProviders("ec2", ptypeBar)
	c.Assert(storage.IsProviderSupported("ec2", ptypeFoo), jc.IsTrue)
	c.Assert(storage.IsProviderSupported("ec2", ptypeBar), jc.IsTrue)
	defaultProvider, ok := storage.DefaultProviderForEnviron("ec2")
	c.Assert(ok, jc.IsTrue)
	c.Assert(defaultProvider, gc.Equals, ptypeFoo)
}

func (s *providerRegistrySuite) DefaultProviderForEnviron(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	ptypeBar := storage.ProviderType("bar")
	storage.RegisterEnvironStorageProviders("ec2", ptypeFoo, ptypeBar)
	defaultProvider, ok := storage.DefaultProviderForEnviron("ec2")
	c.Assert(ok, jc.IsTrue)
	c.Assert(defaultProvider, gc.Equals, ptypeFoo)
}

func (s *providerRegistrySuite) NoDefaultProviderForEnviron(c *gc.C) {
	ptypeFoo := storage.ProviderType("foo")
	ptypeBar := storage.ProviderType("bar")
	storage.RegisterEnvironStorageProviders("ec2", ptypeFoo, ptypeBar)
	_, ok := storage.DefaultProviderForEnviron("openstack")
	c.Assert(ok, jc.IsFalse)
}
