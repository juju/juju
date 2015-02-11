// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	ec2storage "github.com/juju/juju/provider/ec2/storage"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&providerSuite{})

func (*providerSuite) TestEBSProviderRegistered(c *gc.C) {
	p, err := storage.StorageProvider(ec2storage.EBSProviderType)
	c.Assert(err, jc.ErrorIsNil)
	_, ok := p.(storage.Provider)
	c.Assert(ok, jc.IsTrue)
}

func (*providerSuite) TestSupportedProviders(c *gc.C) {
	supported := []storage.ProviderType{ec2storage.EBSProviderType, provider.LoopProviderType}
	for _, providerType := range supported {
		ok := storage.IsProviderSupported("ec2", providerType)
		c.Assert(ok, jc.IsTrue)
	}
}
