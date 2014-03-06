// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/upgrades"
	"launchpad.net/juju-core/utils/ssh"
)

type systemSSHKeySuite struct {
	jujutesting.JujuConnSuite
	ctx upgrades.Context
}

var _ = gc.Suite(&systemSSHKeySuite{})

func (s *systemSSHKeySuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{dataDir: s.DataDir()},
		apiState:    apiState,
	}
	_, err := os.Stat(s.keyFile())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	// There's initially one authorised key for the test user.
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	authKeys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	c.Assert(authKeys, gc.HasLen, 1)
}

func (s *systemSSHKeySuite) keyFile() string {
	return filepath.Join(s.DataDir(), "system-identity")
}

func (s *systemSSHKeySuite) assertKeyCreation(c *gc.C) {
	c.Assert(s.keyFile(), jc.IsNonEmptyFile)

	// Check the private key from the system identify file.
	privateKey, err := ioutil.ReadFile(s.keyFile())
	c.Assert(err, gc.IsNil)
	c.Check(string(privateKey), jc.HasPrefix, "-----BEGIN RSA PRIVATE KEY-----\n")
	c.Check(string(privateKey), jc.HasSuffix, "-----END RSA PRIVATE KEY-----\n")

	// Check the public key from the auth keys config.
	cfg, err := s.JujuConnSuite.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	authKeys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	// The dummy env is created with 1 fake key. We check that another has been added.
	c.Assert(authKeys, gc.HasLen, 2)
	c.Check(authKeys[1], jc.HasPrefix, "ssh-rsa ")
	c.Check(authKeys[1], jc.HasSuffix, " juju-system-key")
}

func (s *systemSSHKeySuite) TestSystemKeyCreated(c *gc.C) {
	err := upgrades.EnsureSystemSSHKey(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertKeyCreation(c)
}

func (s *systemSSHKeySuite) TestIdempotent(c *gc.C) {
	err := upgrades.EnsureSystemSSHKey(s.ctx)
	c.Assert(err, gc.IsNil)

	privateKey, err := ioutil.ReadFile(s.keyFile())
	c.Assert(err, gc.IsNil)

	err = upgrades.EnsureSystemSSHKey(s.ctx)
	c.Assert(err, gc.IsNil)

	// Ensure we haven't generated the key again a second time.
	privateKey2, err := ioutil.ReadFile(s.keyFile())
	c.Assert(err, gc.IsNil)
	c.Assert(privateKey, gc.DeepEquals, privateKey2)
}
