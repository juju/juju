// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeys

import (
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/ssh"
	sshtesting "github.com/juju/utils/v4/ssh/testing"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type SSHKeysSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&SSHKeysSuite{})

type keySuiteBase struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

type ListKeysSuite struct {
	keySuiteBase
}

var _ = gc.Suite(&ListKeysSuite{})

func (s *ListKeysSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ListKeys")
		c.Assert(arg, jc.DeepEquals, params.ListSSHKeys{
			Mode:     ssh.Fingerprints,
			Entities: params.Entities{Entities: []params.Entity{{Tag: "admin"}}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsResults{})
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
	c.Assert(err, jc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Matches, "Keys used in model: king/sword\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestListKeysWithModelUUID(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ListKeys")
		c.Assert(arg, jc.DeepEquals, params.ListSSHKeys{
			Mode:     ssh.Fingerprints,
			Entities: params.Entities{Entities: []params.Entity{{Tag: "admin"}}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsResults{})
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
	c.Assert(err, jc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Matches, "Keys used in model: queen/dagger\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestListFullKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ListKeys")
		c.Assert(arg, jc.DeepEquals, params.ListSSHKeys{
			Mode:     ssh.FullKeys,
			Entities: params.Entities{Entities: []params.Entity{{Tag: "admin"}}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsResults{})
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
	c.Assert(err, jc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Matches, "Keys used in model: king/sword\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestTooManyArgs(c *gc.C) {
	keysCmd := &listKeysCommand{}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), "foo")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["foo"\]`)
}

type AddKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&AddKeySuite{})

func (s *AddKeySuite) TestAddKey(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	var added []string
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "AddKeys")
		c.Assert(arg, jc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		added = []string{key1, key2}
		return nil
	})

	keysCmd := &addKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(added, jc.DeepEquals, []string{key1, key2})
}

func (s *AddKeySuite) TestBlockAddKey(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "AddKeys")
		c.Assert(arg, jc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return apiservererrors.OperationBlockedError("keys")
	})

	keysCmd := &addKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, gc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), jc.IsTrue)
}

type RemoveKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&RemoveKeySuite{})

func (s *RemoveKeySuite) TestRemoveKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	var removed []string
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "DeleteKeys")
		c.Assert(arg, jc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		removed = []string{key1, key2}
		return nil
	})

	keysCmd := &removeKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.DeepEquals, []string{key1, key2})
}

func (s *RemoveKeySuite) TestBlockRemoveKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " (user@host)"
	key2 := sshtesting.ValidKeyTwo.Key + " (another@host)"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "DeleteKeys")
		c.Assert(arg, jc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return apiservererrors.OperationBlockedError("keys")
	})

	keysCmd := &removeKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, gc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), jc.IsTrue)
}

type ImportKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&ImportKeySuite{})

func (s *ImportKeySuite) TestImportKeys(c *gc.C) {
	key1 := "lp:user1"
	key2 := "gh:user2"
	var imported []string
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ImportKeys")
		c.Assert(arg, jc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		imported = []string{key1, key2}
		return nil
	})

	keysCmd := &importKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imported, jc.DeepEquals, []string{key1, key2})
}

func (s *ImportKeySuite) TestBlockImportKeys(c *gc.C) {
	key1 := "lp:user1"
	key2 := "gh:user2"
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "KeyManager")
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ImportKeys")
		c.Assert(arg, jc.DeepEquals, params.ModifyUserSSHKeys{
			User: "admin",
			Keys: []string{key1, key2},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return apiservererrors.OperationBlockedError("keys")
	})

	keysCmd := &importKeysCommand{SSHKeysBase: SSHKeysBase{apiRoot: apiCaller}}
	keysCmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(keysCmd), key1, key2)
	c.Assert(err, gc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), jc.IsTrue)
}
