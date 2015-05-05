// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

type defaultStoragePoolsSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&defaultStoragePoolsSuite{})

func (s *defaultStoragePoolsSuite) TestDefaultStoragePools(c *gc.C) {
	p1, err := storage.NewConfig("pool1", storage.ProviderType("loop"), map[string]interface{}{"1": "2"})
	p2, err := storage.NewConfig("pool2", storage.ProviderType("tmpfs"), map[string]interface{}{"3": "4"})
	c.Assert(err, jc.ErrorIsNil)
	defaultPools := []*storage.Config{p1, p2}
	poolmanager.RegisterDefaultStoragePools(defaultPools)

	settings := state.NewStateSettings(s.State)
	err = poolmanager.AddDefaultStoragePools(settings)
	c.Assert(err, jc.ErrorIsNil)
	pm := poolmanager.New(settings)
	for _, pool := range defaultPools {
		p, err := pm.Get(pool.Name())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(p.Provider(), gc.Equals, pool.Provider())
		c.Assert(p.Attrs(), gc.DeepEquals, pool.Attrs())
	}
}
