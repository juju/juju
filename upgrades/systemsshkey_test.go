// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/utils/ssh"
)

type systemSSHKeySuiteBase struct {
	jujutesting.JujuConnSuite
	ctx upgrades.Context
}

func (s *systemSSHKeySuiteBase) keyFile() string {
	return filepath.Join(s.DataDir(), "system-identity")
}

func (s *systemSSHKeySuiteBase) assertKeyCreation(c *gc.C) string {
	c.Assert(s.keyFile(), jc.IsNonEmptyFile)

	// Check the private key from the system identify file.
	contents, err := ioutil.ReadFile(s.keyFile())
	c.Assert(err, jc.ErrorIsNil)
	privateKey := string(contents)
	c.Check(privateKey, jc.HasPrefix, "-----BEGIN RSA PRIVATE KEY-----\n")
	c.Check(privateKey, jc.HasSuffix, "-----END RSA PRIVATE KEY-----\n")
	return privateKey
}

func (s *systemSSHKeySuiteBase) assertHasPublicKeyInAuth(c *gc.C, privateKey string) {
	publicKey, err := ssh.PublicKey([]byte(privateKey), config.JujuSystemKey)
	c.Assert(err, jc.ErrorIsNil)
	// Check the public key from the auth keys config.
	cfg, err := s.JujuConnSuite.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	authKeys := ssh.SplitAuthorisedKeys(cfg.AuthorizedKeys())
	// The dummy env is created with 1 fake key. We check that another has been added.
	c.Assert(authKeys, gc.HasLen, 2)
	c.Check(authKeys[1]+"\n", gc.Equals, publicKey)
}

type systemSSHKeySuite struct {
	systemSSHKeySuiteBase
}

var _ = gc.Suite(&systemSSHKeySuite{})

func (s *systemSSHKeySuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{dataDir: s.DataDir()},
		apiState:    apiState,
	}

	c.Assert(s.keyFile(), jc.DoesNotExist)
	// Bootstrap adds juju-system-key; remove it.
	err := s.State.UpdateEnvironConfig(map[string]interface{}{
		"authorized-keys": testing.FakeAuthKeys,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *systemSSHKeySuite) TestSystemKeyCreated(c *gc.C) {
	err := upgrades.EnsureSystemSSHKey(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	pk := s.assertKeyCreation(c)
	s.assertHasPublicKeyInAuth(c, pk)
}

func (s *systemSSHKeySuite) TestIdempotent(c *gc.C) {
	err := upgrades.EnsureSystemSSHKey(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	privateKey, err := ioutil.ReadFile(s.keyFile())
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.EnsureSystemSSHKey(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure we haven't generated the key again a second time.
	privateKey2, err := ioutil.ReadFile(s.keyFile())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(privateKey, gc.DeepEquals, privateKey2)
}

type systemSSHKeyReduxSuite struct {
	systemSSHKeySuiteBase
}

var _ = gc.Suite(&systemSSHKeyReduxSuite{})

func (s *systemSSHKeyReduxSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// no api state.
	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{dataDir: s.DataDir()},
		state:       s.State,
	}
	c.Assert(s.keyFile(), jc.DoesNotExist)
	// Bootstrap adds juju-system-key; remove it.
	err := s.State.UpdateEnvironConfig(map[string]interface{}{
		"authorized-keys": testing.FakeAuthKeys,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *systemSSHKeyReduxSuite) TestReduxSystemKeyCreated(c *gc.C) {
	err := upgrades.EnsureSystemSSHKeyRedux(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.assertKeyCreation(c)

	// Config authorized keys should be unaltered.
	cfg, err := s.JujuConnSuite.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AuthorizedKeys(), gc.Equals, testing.FakeAuthKeys)
}

func (s *systemSSHKeyReduxSuite) TestReduxUpdatesAgentConfig(c *gc.C) {
	err := upgrades.EnsureSystemSSHKeyRedux(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	info, _ := s.ctx.AgentConfig().StateServingInfo()
	c.Assert(info.SystemIdentity, gc.Not(gc.Equals), "")
}

func (s *systemSSHKeyReduxSuite) TestReduxIdempotent(c *gc.C) {
	err := upgrades.EnsureSystemSSHKeyRedux(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	privateKey, err := ioutil.ReadFile(s.keyFile())
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.EnsureSystemSSHKeyRedux(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure we haven't generated the key again a second time.
	privateKey2, err := ioutil.ReadFile(s.keyFile())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(privateKey, gc.DeepEquals, privateKey2)
}

func (s *systemSSHKeyReduxSuite) TestReduxExistsInStateServingInfo(c *gc.C) {
	err := state.SetSystemIdentity(s.State, "ssh-private-key")
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.EnsureSystemSSHKeyRedux(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.State.StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.SystemIdentity, gc.Equals, "ssh-private-key")
}

func (s *systemSSHKeyReduxSuite) TestReduxExistsOnDisk(c *gc.C) {
	err := ioutil.WriteFile(s.keyFile(), []byte("ssh-private-key"), 0600)
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.EnsureSystemSSHKeyRedux(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	info, err := s.State.StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.SystemIdentity, gc.Equals, "ssh-private-key")
}

type updateAuthKeysSuite struct {
	systemSSHKeySuiteBase
	systemIdentity string
}

var _ = gc.Suite(&updateAuthKeysSuite{})

func (s *updateAuthKeysSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	mockAgent := &mockAgentConfig{dataDir: s.DataDir()}
	// The ensure system ssh redux has already run.
	err := upgrades.EnsureSystemSSHKeyRedux(&mockContext{
		agentConfig: mockAgent,
		state:       s.State,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.systemIdentity = s.assertKeyCreation(c)

	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.ctx = &mockContext{
		agentConfig: mockAgent,
		apiState:    apiState,
	}
	// Bootstrap adds juju-system-key; remove it.
	err = s.State.UpdateEnvironConfig(map[string]interface{}{
		"authorized-keys": testing.FakeAuthKeys,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateAuthKeysSuite) TestUpgradeStep(c *gc.C) {
	err := upgrades.UpdateAuthorizedKeysForSystemIdentity(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.assertHasPublicKeyInAuth(c, s.systemIdentity)
}

func (s *updateAuthKeysSuite) TestIdempotent(c *gc.C) {
	err := upgrades.UpdateAuthorizedKeysForSystemIdentity(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.UpdateAuthorizedKeysForSystemIdentity(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.assertHasPublicKeyInAuth(c, s.systemIdentity)
}

func (s *updateAuthKeysSuite) TestReplacesWrongKey(c *gc.C) {
	// Put a wrong key in there.
	_, publicKey, err := ssh.GenerateKey(config.JujuSystemKey)
	c.Assert(err, jc.ErrorIsNil)
	keys := testing.FakeAuthKeys + "\n" + publicKey
	err = s.State.UpdateEnvironConfig(map[string]interface{}{
		"authorized-keys": keys,
	}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.UpdateAuthorizedKeysForSystemIdentity(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.assertHasPublicKeyInAuth(c, s.systemIdentity)
}
