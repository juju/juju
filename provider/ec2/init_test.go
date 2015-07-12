// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&providerSuite{})

func (*providerSuite) TestEBSProviderRegistered(c *gc.C) {
	p, err := registry.StorageProvider(ec2.EBS_ProviderType)
	c.Assert(err, jc.ErrorIsNil)
	_, ok := p.(storage.Provider)
	c.Assert(ok, jc.IsTrue)
}

func (*providerSuite) TestSupportedProviders(c *gc.C) {
	supported := []storage.ProviderType{ec2.EBS_ProviderType}
	for _, providerType := range supported {
		ok := registry.IsProviderSupported("ec2", providerType)
		c.Assert(ok, jc.IsTrue)
	}
}

type initSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&initSuite{})

func makeEnviron(c *gc.C) environs.Environ {
	attrs := testing.FakeConfig().Merge(testing.Attrs{
		"type":           "ec2",
		"control-bucket": "x",
		"access-key":     "ko",
		"secret-key":     "ko",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	e, err := environs.New(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return e
}

func (s *initSuite) TestImageMetadataDatasourceAdded(c *gc.C) {
	env := makeEnviron(c)
	dss, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)

	expected := "cloud local storage"
	found := false
	for i, ds := range dss {
		c.Logf("datasource %d: %+v", i, ds)
		if ds.Description() == expected {
			found = true
			break
		}
	}
	c.Assert(found, jc.IsTrue)
}
