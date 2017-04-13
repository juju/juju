// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"github.com/juju/go-oracle-cloud/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type storageSuite struct{}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) NewStorageProvider(c *gc.C) storage.ProviderRegistry {
	env, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		&api.Client{},
		&advancingClock,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(env, gc.NotNil)
	return env
}

func (s *storageSuite) TestStorageProviderTypes(c *gc.C) {
	environ := s.NewStorageProvider(c)

	types, err := environ.StorageProviderTypes()
	c.Assert(err, gc.IsNil)
	c.Assert(types, gc.DeepEquals, oracle.DefaultTypes)
}

func (s *storageSuite) TestStorageProvider(c *gc.C) {
	environ := s.NewStorageProvider(c)
	provider, err := environ.StorageProvider(
		oracle.DefaultStorageProviderType)
	c.Assert(err, gc.IsNil)
	c.Assert(provider, gc.NotNil)
}

func (s *storageSuite) TestStorageProviderWithError(c *gc.C) {
	environ := s.NewStorageProvider(c)
	someType := storage.ProviderType("someType")
	provider, err := environ.StorageProvider(someType)
	c.Assert(err, gc.NotNil)
	c.Assert(provider, gc.IsNil)

}
