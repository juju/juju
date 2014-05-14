// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"strings"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/keymanager"
	"launchpad.net/juju-core/state/api/params"
	keymanagerserver "launchpad.net/juju-core/state/apiserver/keymanager"
	keymanagertesting "launchpad.net/juju-core/state/apiserver/keymanager/testing"
	"launchpad.net/juju-core/utils/ssh"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

type keymanagerSuite struct {
	jujutesting.JujuConnSuite

	keymanager *keymanager.Client
}

var _ = gc.Suite(&keymanagerSuite{})

func (s *keymanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.keymanager = keymanager.NewClient(s.APIState)
	c.Assert(s.keymanager, gc.NotNil)

}

func (s *keymanagerSuite) setAuthorisedKeys(c *gc.C, keys string) {
	err := s.BackingState.UpdateEnvironConfig(map[string]interface{}{"authorized-keys": keys}, nil, nil)
	c.Assert(err, gc.IsNil)
}

func (s *keymanagerSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2}, "\n"))

	keyResults, err := s.keymanager.ListKeys(ssh.Fingerprints, state.AdminUser)
	c.Assert(err, gc.IsNil)
	c.Assert(len(keyResults), gc.Equals, 1)
	result := keyResults[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals,
		[]string{sshtesting.ValidKeyOne.Fingerprint + " (user@host)", sshtesting.ValidKeyTwo.Fingerprint})
}

func (s *keymanagerSuite) TestListKeysErrors(c *gc.C) {
	keyResults, err := s.keymanager.ListKeys(ssh.Fingerprints, "invalid")
	c.Assert(err, gc.IsNil)
	c.Assert(len(keyResults), gc.Equals, 1)
	result := keyResults[0]
	c.Assert(result.Error, gc.ErrorMatches, `permission denied`)
}

func clientError(message string) *params.Error {
	return &params.Error{
		Message: message,
		Code:    "",
	}
}

func (s *keymanagerSuite) assertEnvironKeys(c *gc.C, expected []string) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	keys := envConfig.AuthorizedKeys()
	c.Assert(keys, gc.Equals, strings.Join(expected, "\n"))
}

func (s *keymanagerSuite) TestAddKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorisedKeys(c, key1)

	newKeys := []string{sshtesting.ValidKeyTwo.Key, sshtesting.ValidKeyThree.Key, "invalid"}
	errResults, err := s.keymanager.AddKeys(state.AdminUser, newKeys...)
	c.Assert(err, gc.IsNil)
	c.Assert(errResults, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: clientError("invalid ssh key: invalid")},
	})
	s.assertEnvironKeys(c, append([]string{key1}, newKeys[:2]...))
}

func (s *keymanagerSuite) TestAddSystemKey(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorisedKeys(c, key1)

	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	keyManager := keymanager.NewClient(apiState)
	newKey := sshtesting.ValidKeyTwo.Key
	errResults, err := keyManager.AddKeys("juju-system-key", newKey)
	c.Assert(err, gc.IsNil)
	c.Assert(errResults, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
	})
	s.assertEnvironKeys(c, []string{key1, newKey})
}

func (s *keymanagerSuite) TestAddSystemKeyWrongUser(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorisedKeys(c, key1)

	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	keyManager := keymanager.NewClient(apiState)
	newKey := sshtesting.ValidKeyTwo.Key
	_, err := keyManager.AddKeys("some-user", newKey)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	s.assertEnvironKeys(c, []string{key1})
}

func (s *keymanagerSuite) TestDeleteKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	key3 := sshtesting.ValidKeyThree.Key
	initialKeys := []string{key1, key2, key3, "invalid"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	errResults, err := s.keymanager.DeleteKeys(state.AdminUser, sshtesting.ValidKeyTwo.Fingerprint, "user@host", "missing")
	c.Assert(err, gc.IsNil)
	c.Assert(errResults, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: clientError("invalid ssh key: missing")},
	})
	s.assertEnvironKeys(c, []string{"invalid", key3})
}

func (s *keymanagerSuite) TestImportKeys(c *gc.C) {
	s.PatchValue(&keymanagerserver.RunSSHImportId, keymanagertesting.FakeImport)

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorisedKeys(c, key1)

	keyIds := []string{"lp:validuser", "invalid-key"}
	errResults, err := s.keymanager.ImportKeys(state.AdminUser, keyIds...)
	c.Assert(err, gc.IsNil)
	c.Assert(errResults, gc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: clientError("invalid ssh key id: invalid-key")},
	})
	s.assertEnvironKeys(c, []string{key1, sshtesting.ValidKeyThree.Key})
}

func (s *keymanagerSuite) assertInvalidUserOperation(c *gc.C, test func(user string, keys []string) error) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorisedKeys(c, key1)

	// Run the required test code and check the error.
	keys := []string{sshtesting.ValidKeyTwo.Key, sshtesting.ValidKeyThree.Key}
	err := test("invalid", keys)
	c.Assert(err, gc.ErrorMatches, `permission denied`)

	// No environ changes.
	s.assertEnvironKeys(c, []string{key1})
}

func (s *keymanagerSuite) TestAddKeysInvalidUser(c *gc.C) {
	s.assertInvalidUserOperation(c, func(user string, keys []string) error {
		_, err := s.keymanager.AddKeys(user, keys...)
		return err
	})
}

func (s *keymanagerSuite) TestDeleteKeysInvalidUser(c *gc.C) {
	s.assertInvalidUserOperation(c, func(user string, keys []string) error {
		_, err := s.keymanager.DeleteKeys(user, keys...)
		return err
	})
}

func (s *keymanagerSuite) TestImportKeysInvalidUser(c *gc.C) {
	s.assertInvalidUserOperation(c, func(user string, keys []string) error {
		_, err := s.keymanager.ImportKeys(user, keys...)
		return err
	})
}
