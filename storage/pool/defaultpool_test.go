// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	ec2storage "github.com/juju/juju/provider/ec2/storage"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/pool"
)

type defaultStoragePoolsSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&defaultStoragePoolsSuite{})

type mockAgentConfig struct {
	dataDir string
}

func (mock *mockAgentConfig) DataDir() string {
	return mock.dataDir
}

func (s *defaultStoragePoolsSuite) TestDefaultEBSStoragePools(c *gc.C) {
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, "storage")
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	settings := state.NewStateSettings(s.State)
	err := pool.AddDefaultStoragePools(settings, &mockAgentConfig{dataDir: s.DataDir()})
	c.Assert(err, jc.ErrorIsNil)
	pm := pool.NewPoolManager(settings)
	for _, pName := range []string{"ebs", "ebs-ssd"} {
		p, err := pm.Get(pName)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(p.Type(), gc.Equals, ec2storage.EBSProviderType)
	}
}
