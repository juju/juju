// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/poolmanager"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

type defaultStoragePoolsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&defaultStoragePoolsSuite{})

func (s *defaultStoragePoolsSuite) TestDefaultStoragePools(c *gc.C) {
	p1, err := storage.NewConfig("pool1", storage.ProviderType("whatever"), map[string]interface{}{"1": "2"})
	c.Assert(err, jc.ErrorIsNil)
	p2, err := storage.NewConfig("pool2", storage.ProviderType("whatever"), map[string]interface{}{"3": "4"})
	c.Assert(err, jc.ErrorIsNil)
	provider := &dummystorage.StorageProvider{
		DefaultPools_: []*storage.Config{p1, p2},
	}

	settings := poolmanager.MemSettings{make(map[string]map[string]interface{})}
	pm := poolmanager.New(settings, storage.StaticProviderRegistry{
		map[storage.ProviderType]storage.Provider{"whatever": provider},
	})

	err = poolmanager.AddDefaultStoragePools(provider, pm)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(settings.Settings, jc.DeepEquals, map[string]map[string]interface{}{
		"pool#pool1": {"1": "2", "name": "pool1", "type": "whatever"},
		"pool#pool2": {"3": "4", "name": "pool2", "type": "whatever"},
	})
}
