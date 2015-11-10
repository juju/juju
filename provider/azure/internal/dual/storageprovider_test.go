// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dual_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/azure/internal/dual"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/dummy"
	coretesting "github.com/juju/juju/testing"
)

type storageProviderSuite struct {
	coretesting.BaseSuite

	p1, p2         dummy.StorageProvider
	dual           *dual.StorageProvider
	envcfg         *config.Config
	cfg            *storage.Config
	isPrimaryCalls int
	isPrimaryFunc  func(*config.Config) bool
}

var _ = gc.Suite(&storageProviderSuite{})

func (s *storageProviderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.p1 = dummy.StorageProvider{}
	s.p2 = dummy.StorageProvider{}
	s.dual = dual.NewStorageProvider(&s.p1, &s.p2, s.isPrimary)
	s.envcfg = coretesting.EnvironConfig(c)
	var err error
	s.cfg, err = storage.NewConfig("azure", "azure", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.isPrimaryCalls = 0
	s.isPrimaryFunc = func(*config.Config) bool {
		s.isPrimaryCalls++
		return true
	}
}

func (s *storageProviderSuite) isPrimary(cfg *config.Config) bool {
	return s.isPrimaryFunc(cfg)
}

func (s *storageProviderSuite) TestNewEnvironProvider(c *gc.C) {
	// Initially the "active" provider is the primary one, but it will
	// still be overridden when configuration is set as can be seen
	// in subsequent tests.
	c.Assert(s.dual.Active(), gc.Equals, &s.p1)
}

func (s *storageProviderSuite) TestIsPrimary(c *gc.C) {
	var calls int
	s.isPrimaryFunc = func(*config.Config) bool {
		calls++
		return false
	}
	for i := 0; i < 2; i++ {
		s.dual.VolumeSource(s.envcfg, nil)
		c.Assert(calls, gc.Equals, 1) // isPrimary sticks after the first call
	}
}

func (s *storageProviderSuite) TestVolumeSource(c *gc.C) {
	s.testVolumeSource(c, &s.p1)
}

func (s *storageProviderSuite) TestVolumeSourceIsNotPrimary(c *gc.C) {
	s.isPrimaryFunc = func(*config.Config) bool { return false }
	s.testVolumeSource(c, &s.p2)
}

func (s *storageProviderSuite) testVolumeSource(c *gc.C, active *dummy.StorageProvider) {
	_, err := s.dual.VolumeSource(s.envcfg, s.cfg)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	s.checkActiveCall(c, active, "VolumeSource", s.envcfg, s.cfg)
}

func (s *storageProviderSuite) TestFilesystemSource(c *gc.C) {
	s.testFilesystemSource(c, &s.p1)
}

func (s *storageProviderSuite) TestFilesystemSourceIsNotPrimary(c *gc.C) {
	s.isPrimaryFunc = func(*config.Config) bool { return false }
	s.testFilesystemSource(c, &s.p2)
}

func (s *storageProviderSuite) testFilesystemSource(c *gc.C, active *dummy.StorageProvider) {
	_, err := s.dual.FilesystemSource(s.envcfg, s.cfg)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	s.checkActiveCall(c, active, "FilesystemSource", s.envcfg, s.cfg)
}

func (s *storageProviderSuite) TestValidateConfig(c *gc.C) {
	err := s.dual.ValidateConfig(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.isPrimaryCalls, gc.Equals, 0)
	s.checkActiveCall(c, &s.p1, "ValidateConfig", s.cfg)
}

func (s *storageProviderSuite) TestValidateConfigSecondaryActive(c *gc.C) {
	s.TestVolumeSourceIsNotPrimary(c)
	s.p2.ResetCalls()
	err := s.dual.ValidateConfig(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.checkActiveCall(c, &s.p2, "ValidateConfig", s.cfg)
}

func (s *storageProviderSuite) TestSupports(c *gc.C) {
	supports := s.dual.Supports(storage.StorageKindBlock)
	c.Assert(supports, jc.IsTrue)
	c.Assert(s.isPrimaryCalls, gc.Equals, 0)
	s.checkActiveCall(c, &s.p1, "Supports", storage.StorageKindBlock)
}

func (s *storageProviderSuite) TestSupportsSecondaryActive(c *gc.C) {
	s.TestVolumeSourceIsNotPrimary(c)
	s.p2.ResetCalls()
	supports := s.dual.Supports(storage.StorageKindBlock)
	c.Assert(supports, jc.IsTrue)
	s.checkActiveCall(c, &s.p2, "Supports", storage.StorageKindBlock)
}

func (s *storageProviderSuite) TestScope(c *gc.C) {
	s.dual.Scope()
	c.Assert(s.isPrimaryCalls, gc.Equals, 0)
	s.checkActiveCall(c, &s.p1, "Scope")
}

func (s *storageProviderSuite) TestScopeSecondaryActive(c *gc.C) {
	s.TestVolumeSourceIsNotPrimary(c)
	s.p2.ResetCalls()
	s.dual.Scope()
	s.checkActiveCall(c, &s.p2, "Scope")
}

func (s *storageProviderSuite) TestDynamic(c *gc.C) {
	s.dual.Dynamic()
	c.Assert(s.isPrimaryCalls, gc.Equals, 0)
	s.checkActiveCall(c, &s.p1, "Dynamic")
}

func (s *storageProviderSuite) TestDynamicSecondaryActive(c *gc.C) {
	s.TestVolumeSourceIsNotPrimary(c)
	s.p2.ResetCalls()
	s.dual.Dynamic()
	s.checkActiveCall(c, &s.p2, "Dynamic")
}

/*
func (s *storageProviderSuite) TestBoilerplateConfig(c *gc.C) {
	var calls int
	s.isPrimaryFunc = func(*config.Config) bool {
		calls++
		return true
	}
	_ = s.dual.BoilerplateConfig()
	c.Assert(calls, gc.Equals, 0)
	s.checkActiveCall(c, &s.p1, "BoilerplateConfig")
}

func (s *storageProviderSuite) TestBoilerplateConfigSecondaryActive(c *gc.C) {
	s.TestOpenIsNotPrimary(c)
	s.p2.ResetCalls()
	_ = s.dual.BoilerplateConfig()
	s.checkActiveCall(c, &s.p2, "BoilerplateConfig")
}

func (s *storageProviderSuite) TestRestrictedConfigAttributes(c *gc.C) {
	var calls int
	s.isPrimaryFunc = func(*config.Config) bool {
		calls++
		return true
	}
	_ = s.dual.RestrictedConfigAttributes()
	c.Assert(calls, gc.Equals, 0)
	s.checkActiveCall(c, &s.p1, "RestrictedConfigAttributes")
}

func (s *storageProviderSuite) TestRestrictedConfigAttributesSecondaryActive(c *gc.C) {
	s.TestOpenIsNotPrimary(c)
	s.p2.ResetCalls()
	_ = s.dual.RestrictedConfigAttributes()
	s.checkActiveCall(c, &s.p2, "RestrictedConfigAttributes")
}
*/

func (s *storageProviderSuite) checkActiveCall(c *gc.C, active *dummy.StorageProvider, name string, args ...interface{}) {
	inactive := &s.p1
	if active == inactive {
		inactive = &s.p2
	}
	c.Assert(s.dual.Active(), gc.Equals, active)
	active.CheckCallNames(c, name)
	active.CheckCall(c, 0, name, args...)
	inactive.CheckCallNames(c) // none
}
