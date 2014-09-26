// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
	gc "gopkg.in/check.v1"
)

func newStorageConfig(c *gc.C, attrs testing.Attrs) *storageConfig {
	attrs = testing.FakeConfig().Merge(attrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	ecfg, err := validateConfig(cfg, nil)
	c.Assert(err, gc.IsNil)
	return &storageConfig{ecfg: ecfg}
}

type StorageConfigSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&StorageConfigSuite{})

func (s *StorageConfigSuite) TestStorageConfig(c *gc.C) {
	cfg := newStorageConfig(c, validAttrs())
	cfg.storageDir = "dir"
	cfg.storageAddr = "addr"
	cfg.storagePort = 8080
	c.Check(cfg.StorageDir(), gc.Equals, "dir")
	c.Check(cfg.StorageAddr(), gc.Equals, "addr:8080")
	c.Check(cfg.StorageCACert(), gc.Equals, testing.CACert)
	c.Check(cfg.StorageCAKey(), gc.Equals, testing.CAKey)
	c.Check(cfg.StorageAuthKey(), gc.Equals, "ABCDEFGH")

	hostnames := cfg.StorageHostnames()
	c.Assert(hostnames, gc.HasLen, 1)
	c.Check(hostnames[0], gc.Equals, "addr")
}

func (s *StorageConfigSuite) TestStorageConfigEmpty(c *gc.C) {
	cfg := storageConfig{
		ecfg: &environConfig{
			Config: &config.Config{},
		},
	}
	c.Check(cfg.StorageDir(), gc.Equals, "")
	c.Check(cfg.StorageAddr(), gc.Equals, ":0")
	c.Check(cfg.StorageCACert(), gc.Equals, "")
	c.Check(cfg.StorageCAKey(), gc.Equals, "")
	c.Check(cfg.StorageAuthKey(), gc.Equals, "")

	hostnames := cfg.StorageHostnames()
	c.Assert(hostnames, gc.HasLen, 1)
	c.Check(hostnames[0], gc.Equals, "")
}
