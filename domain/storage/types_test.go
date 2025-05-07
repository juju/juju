// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/domain/storage"
	internalstorage "github.com/juju/juju/internal/storage"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

type defaultStoragePoolsSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&defaultStoragePoolsSuite{})

func (s *defaultStoragePoolsSuite) TestDefaultStoragePools(c *tc.C) {
	p1, err := internalstorage.NewConfig("pool1", "whatever", map[string]interface{}{"1": "2"})
	c.Assert(err, jc.ErrorIsNil)
	p2, err := internalstorage.NewConfig("pool2", "whatever", map[string]interface{}{"3": "4"})
	c.Assert(err, jc.ErrorIsNil)
	provider := &dummystorage.StorageProvider{
		DefaultPools_: []*internalstorage.Config{p1, p2},
	}

	registry := internalstorage.StaticProviderRegistry{
		Providers: map[internalstorage.ProviderType]internalstorage.Provider{
			"whatever": provider,
		},
	}

	pools, err := storage.DefaultStoragePools(registry)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, jc.SameContents, []*internalstorage.Config{p1, p2})
}
