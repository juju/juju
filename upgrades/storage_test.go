// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/upgrades"
)

type defaultStoragePoolsSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&defaultStoragePoolsSuite{})

func (s *defaultStoragePoolsSuite) TestDefaultStoragePools(c *gc.C) {
	err := upgrades.AddDefaultStoragePools(s.State)
	settings := state.NewStateSettings(s.State)
	err = poolmanager.AddDefaultStoragePools(settings)
	c.Assert(err, jc.ErrorIsNil)
	pm := poolmanager.New(settings)
	for _, pName := range []string{"ebs-ssd"} {
		p, err := pm.Get(pName)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(p.Provider(), gc.Equals, ec2.EBS_ProviderType)
	}
}
