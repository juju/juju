// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeys

import (
	"strings"

	"github.com/juju/tc"
	sshtesting "github.com/juju/utils/v4/ssh/testing"

	basetesting "github.com/juju/juju/api/base/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type SSHKeysSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&SSHKeysSuite{})

type keySuiteBase struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

type ListKeysSuite struct {
	keySuiteBase
}

var _ = tc.Suite(&ListKeysSuite{})

func (s *ListKeysSuite) TestListKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ListKeys")
		c.Assert(arg, tc.DeepEquals, params.ListSSHKeys{
			Mode:     params.SSHListModeFingerprint,
			Entities: params.Entities{Entities: []params.Entity{{Tag: "admin"}}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsResults{})
		*(result.(*params.StringsResults)) = params.StringsResults{
			Results: []params.StringsResult{{
				Result: []string{key1, key2},
			}},
		}
		return nil
	})

	keysCmd := &listKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	context, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd))
	c.Assert(err, tc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(output, tc.Matches, "Keys used in model: king/sword\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestListKeysWithModelUUID(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ListKeys")
		c.Assert(arg, tc.DeepEquals, params.ListSSHKeys{
			Mode:     params.SSHListModeFingerprint,
			Entities: params.Entities{Entities: []params.Entity{{Tag: "admin"}}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsResults{})
		*(result.(*params.StringsResults)) = params.StringsResults{
			Results: []params.StringsResult{{
				Result: []string{key1, key2},
			}},
		}
		return nil
	})

	keysCmd := &listKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	store := jujuclienttesting.MinimalStore()
	store.Models["arthur"].Models["queen/dagger"] = store.Models["arthur"].Models["king/sword"]
	keysCmd.SetClientStore(store)
	context, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), "-m", "queen/dagger")
	c.Assert(err, tc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(output, tc.Matches, "Keys used in model: queen/dagger\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestListFullKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ListKeys")
		c.Assert(arg, tc.DeepEquals, params.ListSSHKeys{
			Mode:     params.SSHListModeFull,
			Entities: params.Entities{Entities: []params.Entity{{Tag: "admin"}}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsResults{})
		*(result.(*params.StringsResults)) = params.StringsResults{
			Results: []params.StringsResult{{
				Result: []string{key1, key2},
			}},
		}
		return nil
	})

	keysCmd := &listKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	context, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), "--full")
	c.Assert(err, tc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(output, tc.Matches, "Keys used in model: king/sword\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestTooManyArgs(c *tc.C) {
	keysCmd := &listKeysCommand{}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), "foo")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["foo"\]`)
}

type AddKeySuite struct {
	keySuiteBase
}

var _ = tc.Suite(&AddKeySuite{})

func (s *AddKeySuite) TestAddKey(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	var added []string
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "AddKeys")
		c.Assert(arg, tc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		added = []string{key1, key2}
		return nil
	})

	keysCmd := &addKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(added, tc.DeepEquals, []string{key1, key2})
}

func (s *AddKeySuite) TestBlockAddKey(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "AddKeys")
		c.Assert(arg, tc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return apiservererrors.OperationBlockedError("keys")
	})

	keysCmd := &addKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, tc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), tc.IsTrue)
}

type RemoveKeySuite struct {
	keySuiteBase
}

var _ = tc.Suite(&RemoveKeySuite{})

func (s *RemoveKeySuite) TestRemoveKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	var removed []string
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "DeleteKeys")
		c.Assert(arg, tc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		removed = []string{key1, key2}
		return nil
	})

	keysCmd := &removeKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(removed, tc.DeepEquals, []string{key1, key2})
}

func (s *RemoveKeySuite) TestBlockRemoveKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "DeleteKeys")
		c.Assert(arg, tc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return apiservererrors.OperationBlockedError("keys")
	})

	keysCmd := &removeKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, tc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), tc.IsTrue)
}

type ImportKeySuite struct {
	keySuiteBase
}

var _ = tc.Suite(&ImportKeySuite{})

func (s *ImportKeySuite) TestImportKeys(c *tc.C) {
	key1 := "lp:user1"
	key2 := "gh:user2"
	var imported []string
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ImportKeys")
		c.Assert(arg, tc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		imported = []string{key1, key2}
		return nil
	})

	keysCmd := &importKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imported, tc.DeepEquals, []string{key1, key2})
}

func (s *ImportKeySuite) TestBlockImportKeys(c *tc.C) {
	key1 := "lp:user1"
	key2 := "gh:user2"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "KeyManager")
		c.Assert(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ImportKeys")
		c.Assert(arg, tc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return apiservererrors.OperationBlockedError("keys")
	})

	keysCmd := &importKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, tc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), tc.IsTrue)
}
