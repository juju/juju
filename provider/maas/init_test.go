// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/maas"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
)

type maasProviderSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&maasProviderSuite{})

func (*maasProviderSuite) TestMAASProviderRegistered(c *gc.C) {
	p, err := registry.StorageProvider(maas.MaasStorageProviderType)
	c.Assert(err, jc.ErrorIsNil)
	_, ok := p.(storage.Provider)
	c.Assert(ok, jc.IsTrue)
}

func (*maasProviderSuite) TestSupportedProviders(c *gc.C) {
	supported := []storage.ProviderType{maas.MaasStorageProviderType}
	for _, providerType := range supported {
		ok := registry.IsProviderSupported("maas", providerType)
		c.Assert(ok, jc.IsTrue)
	}
}
