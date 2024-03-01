// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	domainstorage "github.com/juju/juju/domain/storage"
	jujustorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/rpc/params"
)

type poolCreateSuite struct {
	baseStorageSuite
}

var _ = gc.Suite(&poolCreateSuite{})

func (s *poolCreateSuite) TestCreatePool(c *gc.C) {
	const (
		pname = "pname"
		ptype = string(provider.LoopProviderType)
	)
	expected, _ := jujustorage.NewConfig(pname, provider.LoopProviderType, nil)

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name:     pname,
			Provider: ptype,
			Attrs:    nil,
		}},
	}
	results, err := s.api.CreatePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	pools, err := s.storagePoolService.ListStoragePools(context.Background(), domainstorage.StoragePoolFilter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
	c.Assert(pools[0], gc.DeepEquals, expected)
}

func (s *poolCreateSuite) TestCreatePoolError(c *gc.C) {
	msg := "as expected"
	s.baseStorageSuite.storagePoolService.createPool = func(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) error {
		return errors.New(msg)
	}

	args := params.StoragePoolArgs{
		Pools: []params.StoragePool{{
			Name: "doesnt-matter",
		}},
	}
	results, err := s.api.CreatePool(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: "as expected",
	})
}
