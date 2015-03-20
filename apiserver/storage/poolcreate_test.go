// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	jujustorage "github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
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

	err := s.api.CreatePool(params.StoragePool{
		Name:     pname,
		Provider: ptype,
		Attrs:    nil,
	})
	c.Assert(err, jc.ErrorIsNil)

	pools, err := s.poolManager.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pools, gc.HasLen, 1)
	c.Assert(pools[0], gc.DeepEquals, expected)
}

func (s *poolCreateSuite) TestCreatePoolError(c *gc.C) {
	msg := "as expected"
	s.baseStorageSuite.poolManager.createPool = func(name string, providerType jujustorage.ProviderType, attrs map[string]interface{}) (*jujustorage.Config, error) {
		return nil, errors.New(msg)
	}

	err := s.api.CreatePool(params.StoragePool{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}
