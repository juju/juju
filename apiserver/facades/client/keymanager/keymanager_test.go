// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/ssh"
	sshtesting "github.com/juju/utils/v3/ssh/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/keymanager"
	"github.com/juju/juju/apiserver/facades/client/keymanager/mocks"
	keymanagertesting "github.com/juju/juju/apiserver/facades/client/keymanager/testing"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type keyManagerSuite struct {
	testing.CleanupSuite

	model        *mocks.MockModel
	blockChecker *mocks.MockBlockChecker
	apiUser      names.UserTag
	api          *keymanager.KeyManagerAPI

	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&keyManagerSuite{})

func (s *keyManagerSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&keymanager.RunSSHImportId, keymanagertesting.FakeImport)
	s.apiUser = names.NewUserTag("admin")
}

func (s *keyManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.model = mocks.NewMockModel(ctrl)
	s.model.EXPECT().ModelTag().Return(coretesting.ModelTag).AnyTimes()
	s.blockChecker = mocks.NewMockBlockChecker(ctrl)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.apiUser,
	}

	s.api = keymanager.NewKeyManagerAPI(s.model, s.authorizer, s.blockChecker, coretesting.ControllerTag, loggo.GetLogger("juju.apiserver.keymanager"))

	return ctrl
}

func (s *keyManagerSuite) setAuthorizedKeys(c *gc.C, keys ...string) {
	joined := strings.Join(keys, "\n")
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"authorized-keys": joined,
	})
	s.model.EXPECT().ModelConfig(gomock.Any()).Return(config.New(config.UseDefaults, attrs)).AnyTimes()
}

func (s *keyManagerSuite) TestListKeys(c *gc.C) {
	defer s.setup(c).Finish()

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorizedKeys(c, key1, key2, "bad key")

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{Tag: names.NewUserTag("admin").String()},
			{Tag: "invalid"},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.api.ListKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{key1, key2, "Invalid key: bad key"}},
			{Result: []string{key1, key2, "Invalid key: bad key"}},
		},
	})
}

func (s *keyManagerSuite) TestListKeysHidesJujuInternal(c *gc.C) {
	defer s.setup(c).Finish()

	key1 := sshtesting.ValidKeyOne.Key + " juju-client-key"
	key2 := sshtesting.ValidKeyTwo.Key + " " + config.JujuSystemKey
	s.setAuthorizedKeys(c, key1, key2)

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{Tag: names.NewUserTag("admin").String()},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.api.ListKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: nil},
		},
	})
}

func (s *keyManagerSuite) TestListJujuSystemKey(c *gc.C) {
	defer s.setup(c).Finish()

	key1 := sshtesting.ValidKeyOne.Key
	s.setAuthorizedKeys(c, key1)

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{Tag: config.JujuSystemKey},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.api.ListKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "permission denied")
}

func (s *keyManagerSuite) assertAddKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorizedKeys(c, key1, key2, "bad key")

	newKey := sshtesting.ValidKeyThree.Key + " newuser@host"

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: strings.Join([]string{key1, key2, "bad key", newKey}, "\n"),
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").Name(),
		Keys: []string{key2, newKey, newKey, "invalid-key"},
	}
	results, err := s.api.AddKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
			{Error: nil},
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", newKey))},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
		},
	})
}

func (s *keyManagerSuite) TestAddKeys(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)
	s.assertAddKeys(c)
}

func (s *keyManagerSuite) TestAddKeysSuperUser(c *gc.C) {
	s.apiUser = names.NewUserTag("superuser-fred")
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)
	s.assertAddKeys(c)
}

func (s *keyManagerSuite) TestAddKeysModelAdmin(c *gc.C) {
	s.apiUser = names.NewUserTag("admin" + coretesting.ModelTag.String())
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)
	s.assertAddKeys(c)
}

func (s *keyManagerSuite) TestAddKeysNonAuthorised(c *gc.C) {
	s.apiUser = names.NewUserTag("fred")
	defer s.setup(c).Finish()

	_, err := s.api.AddKeys(context.Background(), params.ModifyUserSSHKeys{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
}

func (s *keyManagerSuite) TestBlockAddKeys(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(errors.OperationBlockedError("TestAddKeys"))

	_, err := s.api.AddKeys(context.Background(), params.ModifyUserSSHKeys{})

	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}

func (s *keyManagerSuite) TestAddJujuSystemKey(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)
	s.setAuthorizedKeys(c, sshtesting.ValidKeyOne.Key)

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: sshtesting.ValidKeyOne.Key,
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	newKey := sshtesting.ValidKeyThree.Key + " " + config.JujuSystemKey
	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").Name(),
		Keys: []string{newKey},
	}
	results, err := s.api.AddKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("may not add key with comment juju-system-key: " + newKey)},
		},
	})
}

func (s *keyManagerSuite) assertDeleteKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorizedKeys(c, key1, key2, "bad key")

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: strings.Join([]string{key1, "bad key"}, "\n"),
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").String(),
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, sshtesting.ValidKeyThree.Fingerprint, "invalid-key"},
	}
	results, err := s.api.DeleteKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key: " + sshtesting.ValidKeyThree.Fingerprint)},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
		},
	})
}

func (s *keyManagerSuite) TestDeleteKeys(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed(context.Background()).Return(nil)
	s.assertDeleteKeys(c)
}

func (s *keyManagerSuite) TestDeleteKeysSuperUser(c *gc.C) {
	s.apiUser = names.NewUserTag("superuser-fred")
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed(context.Background()).Return(nil)
	s.assertDeleteKeys(c)
}

func (s *keyManagerSuite) TestDeleteKeysModelAdmin(c *gc.C) {
	s.apiUser = names.NewUserTag("admin" + coretesting.ModelTag.String())
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed(context.Background()).Return(nil)
	s.assertDeleteKeys(c)
}

func (s *keyManagerSuite) TestDeleteKeysNonAuthorised(c *gc.C) {
	s.apiUser = names.NewUserTag("fred")
	defer s.setup(c).Finish()

	_, err := s.api.DeleteKeys(context.Background(), params.ModifyUserSSHKeys{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
}

func (s *keyManagerSuite) TestBlockDeleteKeys(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed(context.Background()).Return(errors.OperationBlockedError("TestDeleteKeys"))

	_, err := s.api.DeleteKeys(context.Background(), params.ModifyUserSSHKeys{})

	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}

func (s *keyManagerSuite) TestDeleteJujuSystemKey(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed(context.Background()).Return(nil)

	key1 := sshtesting.ValidKeyOne.Key + " juju-client-key"
	key2 := sshtesting.ValidKeyTwo.Key + " " + config.JujuSystemKey
	key3 := sshtesting.ValidKeyThree.Key + " a user key"
	s.setAuthorizedKeys(c, key1, key2, key3)

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: strings.Join([]string{key1, key2, key3}, "\n"),
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").Name(),
		Keys: []string{"juju-client-key", config.JujuSystemKey},
	}
	results, err := s.api.DeleteKeys(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("may not delete internal key: juju-client-key")},
			{Error: apiservertesting.ServerError("may not delete internal key: " + config.JujuSystemKey)},
		},
	})
}

// This should be impossible to do anyway since it's impossible to request
// to remove the client and system key
func (s *keyManagerSuite) TestCannotDeleteAllKeys(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed(context.Background()).Return(nil)

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorizedKeys(c, key1, key2)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").String(),
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, "user@host"},
	}
	_, err := s.api.DeleteKeys(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "cannot delete all keys")
}

func (s *keyManagerSuite) assertImportKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	key3 := sshtesting.ValidKeyThree.Key
	key4 := sshtesting.ValidKeyFour.Key
	keymv := strings.Split(sshtesting.ValidKeyMulti, "\n")
	keymp := strings.Split(sshtesting.PartValidKeyMulti, "\n")
	keymi := strings.Split(sshtesting.MultiInvalid, "\n")
	s.setAuthorizedKeys(c, key1, key2, "bad key")

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: strings.Join([]string{
			key1, key2, "bad key", key3, keymv[0], keymv[1], keymp[0], key4,
		}, "\n"),
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").String(),
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
	results, err := s.api.ImportKeys(context.Background(), args)

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
}

func (s *keyManagerSuite) TestImportKeys(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)
	s.assertImportKeys(c)
}

func (s *keyManagerSuite) TestImportKeysSuperUser(c *gc.C) {
	s.apiUser = names.NewUserTag("superuser-fred")
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)
	s.assertImportKeys(c)
}

func (s *keyManagerSuite) TestImportKeysModelAdmin(c *gc.C) {
	s.apiUser = names.NewUserTag("admin" + coretesting.ModelTag.String())
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)
	s.assertImportKeys(c)
}

func (s *keyManagerSuite) TestImportKeysNonAuthorised(c *gc.C) {
	s.apiUser = names.NewUserTag("fred")
	defer s.setup(c).Finish()

	_, err := s.api.ImportKeys(context.Background(), params.ModifyUserSSHKeys{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeUnauthorized)
}

func (s *keyManagerSuite) TestImportJujuSystemKey(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)

	key1 := sshtesting.ValidKeyOne.Key
	s.setAuthorizedKeys(c, key1)
	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: key1,
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").String(),
		Keys: []string{"lp:systemkey"},
	}
	results, err := s.api.ImportKeys(context.Background(), args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("may not add key with comment juju-system-key: " + keymanagertesting.SystemKey)},
		},
	})
}

func (s *keyManagerSuite) TestBlockImportKeys(c *gc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(errors.OperationBlockedError("TestImportKeys"))

	_, err := s.api.ImportKeys(context.Background(), params.ModifyUserSSHKeys{})

	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
}
