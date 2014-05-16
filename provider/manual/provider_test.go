// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"fmt"
	"io"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/provider/manual"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type providerSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.PatchValue(manual.InitUbuntuUser, func(host, user, keys string, stdin io.Reader, stdout io.Writer) error {
		return nil
	})
}

func (s *providerSuite) TestPrepare(c *gc.C) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = true
	delete(minimal, "storage-auth-key")
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	env, err := manual.ProviderInstance.Prepare(coretesting.Context(c), testConfig)
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
	_, err = manual.ProviderInstance.Prepare(coretesting.Context(c), testConfig)
	c.Assert(err, gc.ErrorMatches, "use-sshstorage must not be specified")

	s.PatchValue(manual.NewSSHStorage, func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
		return nil, fmt.Errorf("newSSHStorage failed")
	})
	minimal["use-sshstorage"] = true
	testConfig, err = config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	_, err = manual.ProviderInstance.Prepare(coretesting.Context(c), testConfig)
	c.Assert(err, gc.ErrorMatches, "initialising SSH storage failed: newSSHStorage failed")
}

func (s *providerSuite) TestNullAlias(c *gc.C) {
	p, err := environs.Provider("manual")
	c.Assert(p, gc.NotNil)
	c.Assert(err, gc.IsNil)
	p, err = environs.Provider("null")
	c.Assert(p, gc.NotNil)
	c.Assert(err, gc.IsNil)
}
