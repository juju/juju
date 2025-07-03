// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/storageprovisioner"
)

type ConfigSuite struct {
	testhelpers.IsolationSuite

	// This is a bit unexpected: these tests should mutate the stored
	// config, and then call the checkNotValid method.
	config storageprovisioner.Config
}

func TestConfigSuite(t *testing.T) {
	tc.Run(t, &ConfigSuite{})
}

func (s *ConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = validEnvironConfig()
}

func (s *ConfigSuite) TestNilScope(c *tc.C) {
	s.config.Scope = nil
	s.checkNotValid(c, "nil Scope not valid")
}

func (s *ConfigSuite) TestInvalidScope(c *tc.C) {
	s.config.Scope = names.NewUnitTag("boo/0")
	s.checkNotValid(c, ".* Scope not valid")
}

func (s *ConfigSuite) TestEnvironScopeStorageDir(c *tc.C) {
	s.config.StorageDir = "surprise!"
	s.checkNotValid(c, "environ Scope with non-empty StorageDir not valid")
}

func (s *ConfigSuite) TestMachineScopeStorageDir(c *tc.C) {
	s.config = validMachineConfig()
	s.config.StorageDir = ""
	s.checkNotValid(c, "machine Scope with empty StorageDir not valid")
}

func (s *ConfigSuite) TestApplicationScopeStorageDir(c *tc.C) {
	s.config = validApplicationConfig()
	s.config.StorageDir = "surprise!"
	s.checkNotValid(c, "application Scope with StorageDir not valid")
}

func (s *ConfigSuite) TestNilVolumes(c *tc.C) {
	s.config.Volumes = nil
	s.checkNotValid(c, "nil Volumes not valid")
}

func (s *ConfigSuite) TestNilFilesystems(c *tc.C) {
	s.config.Filesystems = nil
	s.checkNotValid(c, "nil Filesystems not valid")
}

func (s *ConfigSuite) TestNilLife(c *tc.C) {
	s.config.Life = nil
	s.checkNotValid(c, "nil Life not valid")
}

func (s *ConfigSuite) TestNilRegistry(c *tc.C) {
	s.config.Registry = nil
	s.checkNotValid(c, "nil Registry not valid")
}

func (s *ConfigSuite) TestNilMachines(c *tc.C) {
	s.config.Scope = names.NewMachineTag("123")
	s.config.Machines = nil
	s.config.StorageDir = "surprise!"
	s.checkNotValid(c, "nil Machines not valid")
}

func (s *ConfigSuite) TestNilStatus(c *tc.C) {
	s.config.Status = nil
	s.checkNotValid(c, "nil Status not valid")
}

func (s *ConfigSuite) TestNilClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ConfigSuite) TestNilLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ConfigSuite) checkNotValid(c *tc.C, match string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
	c.Check(err, tc.ErrorMatches, match)
}

func validEnvironConfig() storageprovisioner.Config {
	cfg := almostValidConfig()
	cfg.Scope = coretesting.ModelTag
	return cfg
}

func validMachineConfig() storageprovisioner.Config {
	config := almostValidConfig()
	config.Scope = names.NewMachineTag("123/lxd/7")
	config.StorageDir = "storage-dir"
	return config
}

func validApplicationConfig() storageprovisioner.Config {
	config := almostValidConfig()
	config.Scope = names.NewApplicationTag("mariadb")
	return config
}

func almostValidConfig() storageprovisioner.Config {
	// gofmt doesn't seem to want to let me one-line any of these
	// except the last one, so I'm standardising on multi-line.
	return storageprovisioner.Config{
		Volumes: struct {
			storageprovisioner.VolumeAccessor
		}{},
		Filesystems: struct {
			storageprovisioner.FilesystemAccessor
		}{},
		Life: struct {
			storageprovisioner.LifecycleManager
		}{},
		Registry: struct {
			storage.ProviderRegistry
		}{},
		Machines: struct {
			storageprovisioner.MachineAccessor
		}{},
		Status: struct {
			storageprovisioner.StatusSetter
		}{},
		Clock: struct {
			clock.Clock
		}{},
		Logger: struct {
			logger.Logger
		}{},
	}
}
