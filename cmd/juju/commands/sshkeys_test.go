// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	jc "github.com/juju/testing/checkers"
	sshtesting "github.com/juju/utils/ssh/testing"
	gc "gopkg.in/check.v1"

	keymanagerserver "github.com/juju/juju/apiserver/keymanager"
	keymanagertesting "github.com/juju/juju/apiserver/keymanager/testing"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

type SSHKeysSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&SSHKeysSuite{})

func (s *SSHKeysSuite) assertHelpOutput(c *gc.C, cmd, args string) {
	if args != "" {
		args = " " + args
	}
	expected := fmt.Sprintf("usage: juju %s [options]%s", cmd, args)
	out := badrun(c, 0, cmd, "--help")
	lines := strings.Split(out, "\n")
	c.Assert(lines[0], gc.Equals, expected)
}

func (s *SSHKeysSuite) TestHelpList(c *gc.C) {
	s.assertHelpOutput(c, "list-ssh-keys", "")
}

func (s *SSHKeysSuite) TestHelpAdd(c *gc.C) {
	s.assertHelpOutput(c, "add-ssh-key", "<ssh key> ...")
}

func (s *SSHKeysSuite) TestHelpRemove(c *gc.C) {
	s.assertHelpOutput(c, "remove-ssh-key", "<ssh key id> ...")
}

func (s *SSHKeysSuite) TestHelpImport(c *gc.C) {
	s.assertHelpOutput(c, "import-ssh-key", "<ssh key id> ...")
}

type keySuiteBase struct {
	jujutesting.JujuConnSuite
	common.CmdBlockHelper
}

func (s *keySuiteBase) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.PatchEnvironment(osenv.JujuModelEnvKey, "dummymodel")
}

func (s *keySuiteBase) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CmdBlockHelper = common.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func (s *keySuiteBase) setAuthorizedKeys(c *gc.C, keys ...string) {
	keyString := strings.Join(keys, "\n")
	err := s.State.UpdateModelConfig(map[string]interface{}{"authorized-keys": keyString}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envConfig.AuthorizedKeys(), gc.Equals, keyString)
}

func (s *keySuiteBase) assertEnvironKeys(c *gc.C, expected ...string) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	keys := envConfig.AuthorizedKeys()
	c.Assert(keys, gc.Equals, strings.Join(expected, "\n"))
}

type ListKeysSuite struct {
	keySuiteBase
}

var _ = gc.Suite(&ListKeysSuite{})

func (s *ListKeysSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := coretesting.RunCommand(c, NewListKeysCommand())
	c.Assert(err, jc.ErrorIsNil)
	output := strings.TrimSpace(coretesting.Stdout(context))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Matches, "Keys used in model: dummymodel\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestListFullKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := coretesting.RunCommand(c, NewListKeysCommand(), "--full")
	c.Assert(err, jc.ErrorIsNil)
	output := strings.TrimSpace(coretesting.Stdout(context))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Matches, "Keys used in model: dummymodel\n.*user@host\n.*another@host")
}

func (s *ListKeysSuite) TestTooManyArgs(c *gc.C) {
	_, err := coretesting.RunCommand(c, NewListKeysCommand(), "foo")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["foo"\]`)
}

type AddKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&AddKeySuite{})

func (s *AddKeySuite) TestAddKey(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	context, err := coretesting.RunCommand(c, NewAddKeysCommand(), key2, "invalid-key")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Matches, `cannot add key "invalid-key".*\n`)
	s.assertEnvironKeys(c, key1, key2)
}

func (s *AddKeySuite) TestBlockAddKey(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	// Block operation
	s.BlockAllChanges(c, "TestBlockAddKey")
	_, err := coretesting.RunCommand(c, NewAddKeysCommand(), key2, "invalid-key")
	s.AssertBlocked(c, err, ".*TestBlockAddKey.*")
}

type RemoveKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&RemoveKeySuite{})

func (s *RemoveKeySuite) TestRemoveKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := coretesting.RunCommand(c, NewRemoveKeysCommand(),
		sshtesting.ValidKeyTwo.Fingerprint, "invalid-key")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Matches, `cannot remove key id "invalid-key".*\n`)
	s.assertEnvironKeys(c, key1)
}

func (s *RemoveKeySuite) TestBlockRemoveKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	// Block operation
	s.BlockAllChanges(c, "TestBlockRemoveKeys")
	_, err := coretesting.RunCommand(c, NewRemoveKeysCommand(),
		sshtesting.ValidKeyTwo.Fingerprint, "invalid-key")
	s.AssertBlocked(c, err, ".*TestBlockRemoveKeys.*")
}

type ImportKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&ImportKeySuite{})

func (s *ImportKeySuite) SetUpTest(c *gc.C) {
	s.keySuiteBase.SetUpTest(c)
	s.PatchValue(&keymanagerserver.RunSSHImportId, keymanagertesting.FakeImport)
}

func (s *ImportKeySuite) TestImportKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	context, err := coretesting.RunCommand(c, NewImportKeysCommand(), "lp:validuser", "invalid-key")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(coretesting.Stderr(context), gc.Matches, `cannot import key id "invalid-key".*\n`)
	s.assertEnvironKeys(c, key1, sshtesting.ValidKeyThree.Key)
}

func (s *ImportKeySuite) TestBlockImportKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	// Block operation
	s.BlockAllChanges(c, "TestBlockImportKeys")
	_, err := coretesting.RunCommand(c, NewImportKeysCommand(), "lp:validuser", "invalid-key")
	s.AssertBlocked(c, err, ".*TestBlockImportKeys.*")
}
