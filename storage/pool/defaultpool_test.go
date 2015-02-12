// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
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

func (s *defaultStoragePoolsSuite) TestDefaultStoragePools(c *gc.C) {
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, "storage")
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

	defaultPools := []pool.PoolInfo{
		{"pool1", storage.ProviderType("foo"), map[string]interface{}{"1": "2"}},
		{"pool2", storage.ProviderType("bar"), map[string]interface{}{"3": "4"}},
	}
	pool.RegisterDefaultStoragePools(defaultPools)

	settings := state.NewStateSettings(s.State)
	err := pool.AddDefaultStoragePools(settings, &mockAgentConfig{dataDir: s.DataDir()})
	c.Assert(err, jc.ErrorIsNil)
	pm := pool.NewPoolManager(settings)
	for _, info := range defaultPools {
		p, err := pm.Get(info.Name)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(p.Type(), gc.Equals, info.Type)
		c.Assert(p.Config(), gc.DeepEquals, info.Config)
	}
}
