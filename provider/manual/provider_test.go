// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/provider/manual"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type providerSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) TestPrepare(c *gc.C) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = true
	delete(minimal, "storage-auth-key")
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	env, err := manual.ProviderInstance.Prepare(testConfig)
	c.Assert(err, gc.IsNil)
	cfg := env.Config()
	key, _ := cfg.UnknownAttrs()["storage-auth-key"].(string)
	c.Assert(key, jc.Satisfies, utils.IsValidUUIDString)
}

func (s *providerSuite) TestPrepareUseSSHStorage(c *gc.C) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = false
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	_, err = manual.ProviderInstance.Prepare(testConfig)
	c.Assert(err, gc.ErrorMatches, "use-sshstorage must not be specified")

	s.PatchValue(manual.NewSSHStorage, func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
		return nil, fmt.Errorf("newSSHStorage failed")
	})
	minimal["use-sshstorage"] = true
	testConfig, err = config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	_, err = manual.ProviderInstance.Prepare(testConfig)
	c.Assert(err, gc.ErrorMatches, "initialising SSH storage failed: newSSHStorage failed")
}
