// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type storageProviderSuite struct{}

var _ = gc.Suite(&storageProviderSuite{})

func NewStorageProviderTest(c *gc.C) storage.Provider {
	env, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)

	c.Assert(err, gc.IsNil)
	c.Assert(env, gc.NotNil)

	provider, err := env.StorageProvider(
		oracle.DefaultStorageProviderType,
	)

	c.Assert(err, gc.IsNil)
	c.Assert(provider, gc.NotNil)

	return provider
}

func (s *storageProviderSuite) NewStorageProvider(c *gc.C) storage.Provider {
	return NewStorageProviderTest(c)
}

func (s *storageProviderSuite) TestVolumeSource(c *gc.C) {
	provider := s.NewStorageProvider(c)
	source, err := provider.VolumeSource(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.NotNil)
	cfg, err := storage.NewConfig("oracle-latency", oracle.DefaultTypes[0],
		map[string]interface{}{
			oracle.OracleVolumeType: oracle.OracleLatencyPool,
		})
	c.Assert(err, gc.IsNil)
	c.Assert(cfg, gc.NotNil)

	source, err = provider.VolumeSource(cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(source, gc.NotNil)
}

func (s *storageProviderSuite) TestFileSystemSource(c *gc.C) {
	provider := s.NewStorageProvider(c)

	_, err := provider.FilesystemSource(nil)
	c.Assert(err, gc.NotNil)
	c.Assert(errors.IsNotSupported(err), jc.IsTrue)
}

func (s *storageProviderSuite) TestSupports(c *gc.C) {
	provider := s.NewStorageProvider(c)

	ok := provider.Supports(storage.StorageKindBlock)
	c.Assert(ok, jc.IsTrue)

	ok = provider.Supports(storage.StorageKindFilesystem)
	c.Assert(ok, jc.IsFalse)
}

func (s *storageProviderSuite) TestScope(c *gc.C) {
	provider := s.NewStorageProvider(c)

	scope := provider.Scope()
	c.Assert(scope, jc.DeepEquals, storage.ScopeEnviron)
}

func (s *storageProviderSuite) TestDynamic(c *gc.C) {
	provider := s.NewStorageProvider(c)

	ok := provider.Dynamic()
	c.Assert(ok, jc.IsTrue)
}

func (s *storageProviderSuite) TestDefaultPools(c *gc.C) {
	provider := s.NewStorageProvider(c)
	cfg := provider.DefaultPools()
	c.Assert(cfg, gc.NotNil)
}

func (s *storageProviderSuite) TestValidateConfig(c *gc.C) {
	provider := s.NewStorageProvider(c)
	cfg, err := storage.NewConfig("oracle-latency", oracle.DefaultTypes[0],
		map[string]interface{}{
			oracle.OracleVolumeType: oracle.OracleLatencyPool,
		})
	c.Assert(err, gc.IsNil)
	err = provider.ValidateConfig(cfg)
	c.Assert(err, gc.IsNil)
}

func (s *storageProviderSuite) TestValidateConfigWithError(c *gc.C) {
	provider := s.NewStorageProvider(c)
	cfg, err := storage.NewConfig("some-test-name", oracle.DefaultTypes[0],
		map[string]interface{}{
			"some-werid-type": 321,
		})
	c.Assert(err, gc.IsNil)

	err = provider.ValidateConfig(cfg)
	c.Assert(err, gc.IsNil)

	cfg, err = storage.NewConfig("some-test-name", oracle.DefaultTypes[0],
		map[string]interface{}{
			oracle.OracleVolumeType: 321,
		})
	c.Assert(err, gc.IsNil)

	err = provider.ValidateConfig(cfg)
	c.Assert(err, gc.NotNil)

	cfg, err = storage.NewConfig("some-test-name", oracle.DefaultTypes[0],
		map[string]interface{}{
			oracle.OracleVolumeType: "some-string",
		})
	c.Assert(err, gc.IsNil)

	err = provider.ValidateConfig(cfg)
	c.Assert(err, gc.NotNil)
}
