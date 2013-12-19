// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"strings"

	gc "launchpad.net/gocheck"

	"fmt"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/keymanager"
	keymanagertesting "launchpad.net/juju-core/state/apiserver/keymanager/testing"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	statetesting "launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/utils/ssh"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

type keyManagerSuite struct {
	jujutesting.JujuConnSuite

	keymanager *keymanager.KeyManagerAPI
	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&keyManagerSuite{})

func (s *keyManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag:      "user-admin",
		LoggedIn: true,
		Client:   true,
	}
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
}

func (s *keyManagerSuite) TestNewKeyManagerAPIAcceptsClient(c *gc.C) {
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *keyManagerSuite) TestNewKeyManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Client = false
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *keyManagerSuite) setAuthorisedKeys(c *gc.C, keys string) {
	err := statetesting.UpdateConfig(s.State, map[string]interface{}{"authorized-keys": keys})
	c.Assert(err, gc.IsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(envConfig.AuthorizedKeys(), gc.Equals, keys)
}

func (s *keyManagerSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2, "bad key"}, "\n"))

	args := params.ListSSHKeys{
		Entities: params.Entities{[]params.Entity{
			{Tag: "admin"},
			{Tag: "invalid"},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.keymanager.ListKeys(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{key1, key2, "Invalid key: bad key"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *keyManagerSuite) assertEnvironKeys(c *gc.C, expected []string) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	keys := envConfig.AuthorizedKeys()
	c.Assert(keys, gc.Equals, strings.Join(expected, "\n"))
}

func (s *keyManagerSuite) TestAddKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	newKey := sshtesting.ValidKeyThree.Key + " newuser@host"
	args := params.ModifyUserSSHKeys{
		User: "admin",
		Keys: []string{key2, newKey, "invalid-key"},
	}
	results, err := s.keymanager.AddKeys(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
		},
	})
	s.assertEnvironKeys(c, append(initialKeys, newKey))
}

func (s *keyManagerSuite) TestDeleteKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: "admin",
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, sshtesting.ValidKeyThree.Fingerprint, "invalid-key"},
	}
	results, err := s.keymanager.DeleteKeys(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key: " + sshtesting.ValidKeyThree.Fingerprint)},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
		},
	})
	s.assertEnvironKeys(c, []string{"bad key", key1})
}

func (s *keyManagerSuite) TestCannotDeleteAllKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: "admin",
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, "user@host"},
	}
	_, err := s.keymanager.DeleteKeys(args)
	c.Assert(err, gc.ErrorMatches, "cannot delete all keys")
	s.assertEnvironKeys(c, initialKeys)
}

func (s *keyManagerSuite) assertInvalidUserOperation(c *gc.C, runTestLogic func(args params.ModifyUserSSHKeys) error) {
	initialKey := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorisedKeys(c, initialKey)

	// Set up the params.
	newKey := sshtesting.ValidKeyThree.Key + " newuser@host"
	args := params.ModifyUserSSHKeys{
		User: "invalid",
		Keys: []string{newKey},
	}
	// Run the required test code and check the error.
	err := runTestLogic(args)
	c.Assert(err, gc.DeepEquals, apiservertesting.ErrUnauthorized)

	// No environ changes.
	s.assertEnvironKeys(c, []string{initialKey})
}

func (s *keyManagerSuite) TestAddKeysInvalidUser(c *gc.C) {
	s.assertInvalidUserOperation(c, func(args params.ModifyUserSSHKeys) error {
		_, err := s.keymanager.AddKeys(args)
		return err
	})
}

func (s *keyManagerSuite) TestDeleteKeysInvalidUser(c *gc.C) {
	s.assertInvalidUserOperation(c, func(args params.ModifyUserSSHKeys) error {
		_, err := s.keymanager.DeleteKeys(args)
		return err
	})
}

func (s *keyManagerSuite) TestImportKeys(c *gc.C) {
	s.PatchValue(&keymanager.RunSSHImportId, keymanagertesting.FakeImport)

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	key3 := sshtesting.ValidKeyThree.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: "admin",
		Keys: []string{"lp:existing", "lp:validuser", "invalid-key"},
	}
	results, err := s.keymanager.ImportKeys(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key id: invalid-key")},
		},
	})
	s.assertEnvironKeys(c, append(initialKeys, key3))
}

func (s *keyManagerSuite) TestCallSSHImportId(c *gc.C) {
	c.Skip("the landing bot does not run ssh-import-id successfully")
	output, err := keymanager.RunSSHImportId("lp:wallyworld")
	c.Assert(err, gc.IsNil)
	lines := strings.Split(output, "\n")
	var key string
	for _, line := range lines {
		if !strings.HasPrefix(line, "ssh-") {
			continue
		}
		_, _, err := ssh.KeyFingerprint(line)
		if err == nil {
			key = line
		}
	}
	c.Assert(key, gc.Not(gc.Equals), "")
}
