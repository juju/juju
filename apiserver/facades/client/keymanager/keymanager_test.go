// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"fmt"
	"strings"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/ssh"
	sshtesting "github.com/juju/utils/ssh/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/client/keymanager"
	keymanagertesting "github.com/juju/juju/apiserver/facades/client/keymanager/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
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

func (s *keyManagerSuite) TestNewKeyManagerAPIRefusesController(c *gc.C) {
	s.testNewKeyManagerAPIRefuses(c, names.NewMachineTag("0"), true)
}

func (s *keyManagerSuite) TestNewKeyManagerAPIRefusesUnit(c *gc.C) {
	s.testNewKeyManagerAPIRefuses(c, names.NewUnitTag("mysql/0"), false)
}

func (s *keyManagerSuite) TestNewKeyManagerAPIRefusesMachine(c *gc.C) {
	s.testNewKeyManagerAPIRefuses(c, names.NewMachineTag("99"), false)
}

func (s *keyManagerSuite) testNewKeyManagerAPIRefuses(c *gc.C, tag names.Tag, controller bool) {
	auth := apiservertesting.FakeAuthorizer{
		Tag:        tag,
		Controller: controller,
	}
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, auth)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *keyManagerSuite) setAuthorisedKeys(c *gc.C, keys string) {
	s.setAuthorisedKeysForModel(c, s.State, keys)
}

func (s *keyManagerSuite) setAuthorisedKeysForModel(c *gc.C, st *state.State, keys string) {
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = m.UpdateModelConfig(map[string]interface{}{"authorized-keys": keys}, nil)
	c.Assert(err, jc.ErrorIsNil)

	modelConfig, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelConfig.AuthorizedKeys(), gc.Equals, keys)
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

func (s *keyManagerSuite) TestListKeysHidesJujuInternal(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " juju-client-key"
	key2 := sshtesting.ValidKeyTwo.Key + " juju-system-key"
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2}, "\n"))

	args := params.ListSSHKeys{
		Entities: params.Entities{[]params.Entity{
			{Tag: s.AdminUserTag(c).Name()},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.keymanager.ListKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: nil},
		},
	})
}

func (s *keyManagerSuite) TestListJujuSystemKey(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("fred")
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)
	key1 := sshtesting.ValidKeyOne.Key
	s.setAuthorisedKeys(c, key1)

	args := params.ListSSHKeys{
		Entities: params.Entities{[]params.Entity{
			{Tag: "juju-system-key"},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.keymanager.ListKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "permission denied")
}

func (s *keyManagerSuite) assertModelKeys(c *gc.C, expected []string) {
	s.assertKeysForModel(c, s.State, expected)
}

func (s *keyManagerSuite) assertKeysForModel(c *gc.C, st *state.State, expected []string) {
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	modelConfig, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	keys := modelConfig.AuthorizedKeys()
	c.Assert(keys, gc.Equals, strings.Join(expected, "\n"))
}

func (s *keyManagerSuite) assertAddKeys(c *gc.C, st *state.State, apiUser names.UserTag, ok bool) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeysForModel(c, st, strings.Join(initialKeys, "\n"))

	anAuthoriser := s.authoriser
	anAuthoriser.Tag = apiUser
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(st, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)

	newKey := sshtesting.ValidKeyThree.Key + " newuser@host"
	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
		Keys: []string{key2, newKey, "invalid-key"},
	}
	results, err := s.keymanager.AddKeys(args)
	if !ok {
		c.Assert(err, gc.ErrorMatches, "permission denied")
		c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
		return
	}
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
		},
	})
	s.assertKeysForModel(c, st, append(initialKeys, newKey))
}

func (s *keyManagerSuite) TestAddKeys(c *gc.C) {
	s.assertAddKeys(c, s.State, s.AdminUserTag(c), true)
}

func (s *keyManagerSuite) TestAddKeysSuperUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "superuser-fred", NoModelUser: true})
	s.assertAddKeys(c, s.State, user.UserTag(), true)
}

func (s *keyManagerSuite) TestAddKeysModelAdmin(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()

	otherModel, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)

	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "admin" + otherModel.ModelTag().String()})
	s.assertAddKeys(c, otherState, user.UserTag(), true)
}

func (s *keyManagerSuite) TestAddKeysNonAuthorised(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	s.assertAddKeys(c, s.State, user.UserTag(), false)
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
	s.assertModelKeys(c, initialKeys)
}

func (s *keyManagerSuite) TestAddJujuSystemKey(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("fred")
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
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
	s.assertModelKeys(c, []string{key1})
}

func (s *keyManagerSuite) assertDeleteKeys(c *gc.C, st *state.State, apiUser names.UserTag, ok bool) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = apiUser
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(st, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeysForModel(c, st, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, sshtesting.ValidKeyThree.Fingerprint, "invalid-key"},
	}
	results, err := s.keymanager.DeleteKeys(args)
	if !ok {
		c.Assert(err, gc.ErrorMatches, "permission denied")
		c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key: " + sshtesting.ValidKeyThree.Fingerprint)},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
		},
	})
	s.assertKeysForModel(c, st, []string{key1, "bad key"})
}

func (s *keyManagerSuite) TestDeleteKeys(c *gc.C) {
	s.assertDeleteKeys(c, s.State, s.AdminUserTag(c), true)
}

func (s *keyManagerSuite) TestDeleteKeysSuperUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "superuser-fred", NoModelUser: true})
	s.assertDeleteKeys(c, s.State, user.UserTag(), true)
}

func (s *keyManagerSuite) TestDeleteKeysModelAdmin(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()

	otherModel, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)

	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "admin" + otherModel.ModelTag().String()})
	s.assertDeleteKeys(c, otherState, user.UserTag(), true)
}

func (s *keyManagerSuite) TestDeleteKeysNonAuthorised(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	s.assertDeleteKeys(c, s.State, user.UserTag(), false)
}

func (s *keyManagerSuite) TestDeleteKeysNotJujuInternal(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " juju-client-key"
	key2 := sshtesting.ValidKeyTwo.Key + " juju-system-key"
	key3 := sshtesting.ValidKeyThree.Key + " a user key"
	initialKeys := []string{key1, key2, key3}
	s.setAuthorisedKeys(c, strings.Join(initialKeys, "\n"))

	args := params.ModifyUserSSHKeys{
		User: s.AdminUserTag(c).Name(),
		Keys: []string{"juju-client-key", "juju-system-key"},
	}
	results, err := s.keymanager.DeleteKeys(args)
	c.Check(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("may not delete internal key: juju-client-key")},
			{Error: apiservertesting.ServerError("may not delete internal key: juju-system-key")},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertModelKeys(c, initialKeys)
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
	s.assertModelKeys(c, initialKeys)
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
	s.assertModelKeys(c, initialKeys)
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

	// No model changes.
	s.assertModelKeys(c, []string{initialKey})
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

func (s *keyManagerSuite) assertImportKeys(c *gc.C, st *state.State, apiUser names.UserTag, ok bool) {
	s.PatchValue(&keymanager.RunSSHImportId, keymanagertesting.FakeImport)

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	key3 := sshtesting.ValidKeyThree.Key
	key4 := sshtesting.ValidKeyFour.Key
	keymv := strings.Split(sshtesting.ValidKeyMulti, "\n")
	keymp := strings.Split(sshtesting.PartValidKeyMulti, "\n")
	keymi := strings.Split(sshtesting.MultiInvalid, "\n")
	initialKeys := []string{key1, key2, "bad key"}
	s.setAuthorisedKeysForModel(c, st, strings.Join(initialKeys, "\n"))

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

	anAuthoriser := s.authoriser
	anAuthoriser.Tag = apiUser
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(st, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.keymanager.ImportKeys(args)
	if !ok {
		c.Assert(err, gc.ErrorMatches, "permission denied")
		c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
		return
	}
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
	s.assertKeysForModel(c, st, append(initialKeys, key3, keymv[0], keymv[1], keymp[0], key4))
}

func (s *keyManagerSuite) TestImportKeys(c *gc.C) {
	s.assertImportKeys(c, s.State, s.AdminUserTag(c), true)
}

func (s *keyManagerSuite) TestImportKeysSuperUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "superuser-fred", NoModelUser: true})
	s.assertImportKeys(c, s.State, user.UserTag(), true)
}

func (s *keyManagerSuite) TestImportKeysModelAdmin(c *gc.C) {
	otherState := s.Factory.MakeModel(c, nil)
	defer otherState.Close()

	otherModel, err := otherState.Model()
	c.Assert(err, jc.ErrorIsNil)

	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "admin" + otherModel.ModelTag().String()})
	s.assertImportKeys(c, otherState, user.UserTag(), true)
}

func (s *keyManagerSuite) TestImportKeysNonAuthorised(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	s.assertImportKeys(c, s.State, user.UserTag(), false)
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
	s.assertModelKeys(c, initialKeys)
}
