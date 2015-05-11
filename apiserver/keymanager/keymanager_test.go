// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"fmt"
	"strings"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/keymanager"
	keymanagertesting "github.com/juju/juju/apiserver/keymanager/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/utils/ssh"
	sshtesting "github.com/juju/juju/utils/ssh/testing"
)

type keyManagerSuite struct {
	jujutesting.JujuConnSuite

	keymanager *keymanager.KeyManagerAPI
	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer

	commontesting.BlockHelper
}

var _ = gc.Suite(&keyManagerSuite{})

func (s *keyManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

func (s *keyManagerSuite) TestNewKeyManagerAPIAcceptsClient(c *gc.C) {
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *keyManagerSuite) TestNewKeyManagerAPIAcceptsEnvironManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = true
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *keyManagerSuite) TestNewKeyManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	anAuthoriser.EnvironManager = false
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *keyManagerSuite) TestNewKeyManagerAPIRefusesNonEnvironManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewMachineTag("99")
	anAuthoriser.EnvironManager = false
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *keyManagerSuite) setAuthorisedKeys(c *gc.C, keys string) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"authorized-keys": keys}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envConfig.AuthorizedKeys(), gc.Equals, keys)
}

func (s *keyManagerSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2, "bad key"}, "\n"))

	args := params.ListSSHKeys{
		Entities: params.Entities{[]params.Entity{
			{Tag: s.AdminUserTag(c).Name()},
			{Tag: "invalid"},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.keymanager.ListKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{key1, key2, "Invalid key: bad key"}},
			{Result: []string{key1, key2, "Invalid key: bad key"}},
		},
	})
}

func (s *keyManagerSuite) assertEnvironKeys(c *gc.C, expected []string) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
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
		User: s.AdminUserTag(c).Name(),
		Keys: []string{key2, newKey, "invalid-key"},
	}
	results, err := s.keymanager.AddKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
		},
	})
	s.assertEnvironKeys(c, append(initialKeys, newKey))
}

func (s *keyManagerSuite) TestBlockAddKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	newKey := sshtesting.ValidKeyThree.Key + " newuser@host"
	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
		Keys: []string{key2, newKey, "invalid-key"},
	}

	s.BlockAllChanges(c, "TestBlockAddKeys")
	_, err := s.keymanager.AddKeys(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockAddKeys")
	s.assertEnvironKeys(c, initialKeys)
}

func (s *keyManagerSuite) TestAddJujuSystemKey(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = true
	anAuthoriser.Tag = names.NewMachineTag("0")
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	newKey := sshtesting.ValidKeyThree.Key + " juju-system-key"
	args := params.ModifyUserSSHKeys{
		User: "juju-system-key",
		Keys: []string{newKey},
	}
	results, err := s.keymanager.AddKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})
	s.assertEnvironKeys(c, append(initialKeys, newKey))
}

func (s *keyManagerSuite) TestAddJujuSystemKeyNotMachine(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = true
	anAuthoriser.Tag = names.NewUnitTag("wordpress/0")
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)
	key1 := sshtesting.ValidKeyOne.Key
	s.setAuthorisedKeys(c, key1)

	newKey := sshtesting.ValidKeyThree.Key + " juju-system-key"
	args := params.ModifyUserSSHKeys{
		User: "juju-system-key",
		Keys: []string{newKey},
	}
	_, err = s.keymanager.AddKeys(args)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	s.assertEnvironKeys(c, []string{key1})
}

func (s *keyManagerSuite) TestDeleteKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, sshtesting.ValidKeyThree.Fingerprint, "invalid-key"},
	}
	results, err := s.keymanager.DeleteKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key: " + sshtesting.ValidKeyThree.Fingerprint)},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
		},
	})
	s.assertEnvironKeys(c, []string{"bad key", key1})
}

func (s *keyManagerSuite) TestBlockDeleteKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, sshtesting.ValidKeyThree.Fingerprint, "invalid-key"},
	}

	s.BlockAllChanges(c, "TestBlockDeleteKeys")
	_, err := s.keymanager.DeleteKeys(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockDeleteKeys")
	s.assertEnvironKeys(c, initialKeys)
}

func (s *keyManagerSuite) TestCannotDeleteAllKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
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
	c.Skip("no user validation done yet")
	s.assertInvalidUserOperation(c, func(args params.ModifyUserSSHKeys) error {
		_, err := s.keymanager.AddKeys(args)
		return err
	})
}

func (s *keyManagerSuite) TestDeleteKeysInvalidUser(c *gc.C) {
	c.Skip("no user validation done yet")
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
	key4 := sshtesting.ValidKeyFour.Key
	keymv := strings.Split(sshtesting.ValidKeyMulti, "\n")
	keymp := strings.Split(sshtesting.PartValidKeyMulti, "\n")
	keymi := strings.Split(sshtesting.MultiInvalid, "\n")
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
		Keys: []string{
			"lp:existing",
			"lp:validuser",
			"invalid-key",
			"lp:multi",
			"lp:multiempty",
			"lp:multipartial",
			"lp:multiinvalid",
			"lp:multionedup",
		},
	}
	results, err := s.keymanager.ImportKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 8)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key id: invalid-key")},
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key id: lp:multiempty")},
			{Error: apiservertesting.ServerError(fmt.Sprintf(
				`invalid ssh key for lp:multipartial: `+
					`generating key fingerprint: `+
					`invalid authorized_key "%s"`, keymp[1]))},
			{Error: apiservertesting.ServerError(fmt.Sprintf(
				`invalid ssh key for lp:multiinvalid: `+
					`generating key fingerprint: `+
					`invalid authorized_key "%s"`+"\n"+
					`invalid ssh key for lp:multiinvalid: `+
					`generating key fingerprint: `+
					`invalid authorized_key "%s"`, keymi[0], keymi[1]))},
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
		},
	})
	s.assertEnvironKeys(c, append(initialKeys, key3, keymv[0], keymv[1], keymp[0], key4))
}

func (s *keyManagerSuite) TestBlockImportKeys(c *gc.C) {
	s.PatchValue(&keymanager.RunSSHImportId, keymanagertesting.FakeImport)

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
		Keys: []string{"lp:existing", "lp:validuser", "invalid-key"},
	}

	s.BlockAllChanges(c, "TestBlockImportKeys")
	_, err := s.keymanager.ImportKeys(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockImportKeys")
	s.assertEnvironKeys(c, initialKeys)
}
