// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/storageprovisioner"
)

type ConfigSuite struct {
	testing.IsolationSuite

	// This is a bit unexpected: these tests should mutate the stored
	// config, and then call the checkNotValid method.
	config storageprovisioner.Config
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = validEnvironConfig()
}

func (s *ConfigSuite) TestNilScope(c *gc.C) {
	s.config.Scope = nil
	s.checkNotValid(c, "nil Scope not valid")
}

func (s *ConfigSuite) TestInvalidScope(c *gc.C) {
	s.config.Scope = names.NewApplicationTag("boo")
	s.checkNotValid(c, ".* Scope not valid")
}

func (s *ConfigSuite) TestEnvironScopeStorageDir(c *gc.C) {
	s.config.StorageDir = "surprise!"
	s.checkNotValid(c, "environ Scope with non-empty StorageDir not valid")
}

func (s *ConfigSuite) TestMachineScopeStorageDir(c *gc.C) {
	s.config = validMachineConfig()
	s.config.StorageDir = ""
	s.checkNotValid(c, "machine Scope with empty StorageDir not valid")
}

func (s *ConfigSuite) TestNilVolumes(c *gc.C) {
	s.config.Volumes = nil
	s.checkNotValid(c, "nil Volumes not valid")
}

func (s *ConfigSuite) TestNilFilesystems(c *gc.C) {
	s.config.Filesystems = nil
	s.checkNotValid(c, "nil Filesystems not valid")
}

func (s *ConfigSuite) TestNilLife(c *gc.C) {
	s.config.Life = nil
	s.checkNotValid(c, "nil Life not valid")
}

func (s *ConfigSuite) TestNilRegistry(c *gc.C) {
	s.config.Registry = nil
	s.checkNotValid(c, "nil Registry not valid")
}

func (s *ConfigSuite) TestNilMachines(c *gc.C) {
	s.config.Machines = nil
	s.checkNotValid(c, "nil Machines not valid")
}

func (s *ConfigSuite) TestNilStatus(c *gc.C) {
	s.config.Status = nil
	s.checkNotValid(c, "nil Status not valid")
}

func (s *ConfigSuite) TestNilClock(c *gc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ConfigSuite) checkNotValid(c *gc.C, match string) {
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, match)
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
	}
}
