// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"encoding/base64"
	"strings"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/ssh"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type AuthorisedKeysKeysSuite struct {
	testbase.LoggingSuite
}

const (
	// We'll use the current user for ssh tests.
	testSSHUser = ""
)

var _ = gc.Suite(&AuthorisedKeysKeysSuite{})

func (s *AuthorisedKeysKeysSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	fakeHome := coretesting.MakeEmptyFakeHomeWithoutJuju(c)
	s.AddCleanup(func(*gc.C) { fakeHome.Restore() })
}

func writeAuthKeysFile(c *gc.C, keys []string) {
	err := ssh.WriteAuthorisedKeys(testSSHUser, keys)
	c.Assert(err, gc.IsNil)
}

func (s *AuthorisedKeysKeysSuite) TestListKeys(c *gc.C) {
	keys := []string{
		sshtesting.ValidKeyOne.Key + " user@host",
		sshtesting.ValidKeyTwo.Key,
	}
	writeAuthKeysFile(c, keys)
	keys, err := ssh.ListKeys(testSSHUser, ssh.Fingerprints)
	c.Assert(err, gc.IsNil)
	c.Assert(
		keys, gc.DeepEquals,
		[]string{sshtesting.ValidKeyOne.Fingerprint + " (user@host)", sshtesting.ValidKeyTwo.Fingerprint})
}

func (s *AuthorisedKeysKeysSuite) TestListKeysFull(c *gc.C) {
	keys := []string{
		sshtesting.ValidKeyOne.Key + " user@host",
		sshtesting.ValidKeyTwo.Key + " anotheruser@host",
	}
	writeAuthKeysFile(c, keys)
	actual, err := ssh.ListKeys(testSSHUser, ssh.FullKeys)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, keys)
}

func (s *AuthorisedKeysKeysSuite) TestAddNewKey(c *gc.C) {
	key := sshtesting.ValidKeyOne.Key + " user@host"
	err := ssh.AddKeys(testSSHUser, key)
	c.Assert(err, gc.IsNil)
	actual, err := ssh.ListKeys(testSSHUser, ssh.FullKeys)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, []string{key})
}

func (s *AuthorisedKeysKeysSuite) TestAddMoreKeys(c *gc.C) {
	firstKey := sshtesting.ValidKeyOne.Key + " user@host"
	writeAuthKeysFile(c, []string{firstKey})
	moreKeys := []string{
		sshtesting.ValidKeyTwo.Key + " anotheruser@host",
		sshtesting.ValidKeyThree.Key + " yetanotheruser@host",
	}
	err := ssh.AddKeys(testSSHUser, moreKeys...)
	c.Assert(err, gc.IsNil)
	actual, err := ssh.ListKeys(testSSHUser, ssh.FullKeys)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, append([]string{firstKey}, moreKeys...))
}

func (s *AuthorisedKeysKeysSuite) TestAddDuplicateKey(c *gc.C) {
	key := sshtesting.ValidKeyOne.Key + " user@host"
	err := ssh.AddKeys(testSSHUser, key)
	c.Assert(err, gc.IsNil)
	moreKeys := []string{
		sshtesting.ValidKeyOne.Key + " user@host",
		sshtesting.ValidKeyTwo.Key + " yetanotheruser@host",
	}
	err = ssh.AddKeys(testSSHUser, moreKeys...)
	c.Assert(err, gc.ErrorMatches, "cannot add duplicate ssh key: "+sshtesting.ValidKeyOne.Fingerprint)
}

func (s *AuthorisedKeysKeysSuite) TestAddDuplicateComment(c *gc.C) {
	key := sshtesting.ValidKeyOne.Key + " user@host"
	err := ssh.AddKeys(testSSHUser, key)
	c.Assert(err, gc.IsNil)
	moreKeys := []string{
		sshtesting.ValidKeyTwo.Key + " user@host",
		sshtesting.ValidKeyThree.Key + " yetanotheruser@host",
	}
	err = ssh.AddKeys(testSSHUser, moreKeys...)
	c.Assert(err, gc.ErrorMatches, "cannot add ssh key with duplicate comment: user@host")
}

func (s *AuthorisedKeysKeysSuite) TestAddKeyWithoutComment(c *gc.C) {
	keys := []string{
		sshtesting.ValidKeyOne.Key + " user@host",
		sshtesting.ValidKeyTwo.Key,
	}
	err := ssh.AddKeys(testSSHUser, keys...)
	c.Assert(err, gc.ErrorMatches, "cannot add ssh key without comment")
}

func (s *AuthorisedKeysKeysSuite) TestAddKeepsUnrecognised(c *gc.C) {
	writeAuthKeysFile(c, []string{sshtesting.ValidKeyOne.Key, "invalid-key"})
	anotherKey := sshtesting.ValidKeyTwo.Key + " anotheruser@host"
	err := ssh.AddKeys(testSSHUser, anotherKey)
	c.Assert(err, gc.IsNil)
	actual, err := ssh.ReadAuthorisedKeys(testSSHUser)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, []string{sshtesting.ValidKeyOne.Key, "invalid-key", anotherKey})
}

func (s *AuthorisedKeysKeysSuite) TestDeleteKeys(c *gc.C) {
	firstKey := sshtesting.ValidKeyOne.Key + " user@host"
	anotherKey := sshtesting.ValidKeyTwo.Key
	thirdKey := sshtesting.ValidKeyThree.Key + " anotheruser@host"
	writeAuthKeysFile(c, []string{firstKey, anotherKey, thirdKey})
	err := ssh.DeleteKeys(testSSHUser, "user@host", sshtesting.ValidKeyTwo.Fingerprint)
	c.Assert(err, gc.IsNil)
	actual, err := ssh.ListKeys(testSSHUser, ssh.FullKeys)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, []string{thirdKey})
}

func (s *AuthorisedKeysKeysSuite) TestDeleteKeysKeepsUnrecognised(c *gc.C) {
	firstKey := sshtesting.ValidKeyOne.Key + " user@host"
	writeAuthKeysFile(c, []string{firstKey, sshtesting.ValidKeyTwo.Key, "invalid-key"})
	err := ssh.DeleteKeys(testSSHUser, "user@host")
	c.Assert(err, gc.IsNil)
	actual, err := ssh.ReadAuthorisedKeys(testSSHUser)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, []string{"invalid-key", sshtesting.ValidKeyTwo.Key})
}

func (s *AuthorisedKeysKeysSuite) TestDeleteNonExistentComment(c *gc.C) {
	firstKey := sshtesting.ValidKeyOne.Key + " user@host"
	writeAuthKeysFile(c, []string{firstKey})
	err := ssh.DeleteKeys(testSSHUser, "someone@host")
	c.Assert(err, gc.ErrorMatches, "cannot delete non existent key: someone@host")
}

func (s *AuthorisedKeysKeysSuite) TestDeleteNonExistentFingerprint(c *gc.C) {
	firstKey := sshtesting.ValidKeyOne.Key + " user@host"
	writeAuthKeysFile(c, []string{firstKey})
	err := ssh.DeleteKeys(testSSHUser, sshtesting.ValidKeyTwo.Fingerprint)
	c.Assert(err, gc.ErrorMatches, "cannot delete non existent key: "+sshtesting.ValidKeyTwo.Fingerprint)
}

func (s *AuthorisedKeysKeysSuite) TestDeleteLastKeyForbidden(c *gc.C) {
	keys := []string{
		sshtesting.ValidKeyOne.Key + " user@host",
		sshtesting.ValidKeyTwo.Key + " yetanotheruser@host",
	}
	writeAuthKeysFile(c, keys)
	err := ssh.DeleteKeys(testSSHUser, "user@host", sshtesting.ValidKeyTwo.Fingerprint)
	c.Assert(err, gc.ErrorMatches, "cannot delete all keys")
}

func (s *AuthorisedKeysKeysSuite) TestReplaceKeys(c *gc.C) {
	firstKey := sshtesting.ValidKeyOne.Key + " user@host"
	anotherKey := sshtesting.ValidKeyTwo.Key
	writeAuthKeysFile(c, []string{firstKey, anotherKey})

	replaceKey := sshtesting.ValidKeyThree.Key + " anotheruser@host"
	err := ssh.ReplaceKeys(testSSHUser, replaceKey)
	c.Assert(err, gc.IsNil)
	actual, err := ssh.ListKeys(testSSHUser, ssh.FullKeys)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, []string{replaceKey})
}

func (s *AuthorisedKeysKeysSuite) TestReplaceKeepsUnrecognised(c *gc.C) {
	writeAuthKeysFile(c, []string{sshtesting.ValidKeyOne.Key, "invalid-key"})
	anotherKey := sshtesting.ValidKeyTwo.Key + " anotheruser@host"
	err := ssh.ReplaceKeys(testSSHUser, anotherKey)
	c.Assert(err, gc.IsNil)
	actual, err := ssh.ReadAuthorisedKeys(testSSHUser)
	c.Assert(err, gc.IsNil)
	c.Assert(actual, gc.DeepEquals, []string{"invalid-key", anotherKey})
}

func (s *AuthorisedKeysKeysSuite) TestEnsureJujuComment(c *gc.C) {
	sshKey := sshtesting.ValidKeyOne.Key
	for _, test := range []struct {
		key      string
		expected string
	}{
		{"invalid-key", "invalid-key"},
		{sshKey, sshKey + " Juju:sshkey"},
		{sshKey + " user@host", sshKey + " Juju:user@host"},
		{sshKey + " Juju:user@host", sshKey + " Juju:user@host"},
		{sshKey + " " + sshKey[3:5], sshKey + " Juju:" + sshKey[3:5]},
	} {
		actual := ssh.EnsureJujuComment(test.key)
		c.Assert(actual, gc.Equals, test.expected)
	}
}

func (s *AuthorisedKeysKeysSuite) TestSplitAuthorisedKeys(c *gc.C) {
	sshKey := sshtesting.ValidKeyOne.Key
	for _, test := range []struct {
		keyData  string
		expected []string
	}{
		{"", nil},
		{sshKey, []string{sshKey}},
		{sshKey + "\n", []string{sshKey}},
		{sshKey + "\n\n", []string{sshKey}},
		{sshKey + "\n#comment\n", []string{sshKey}},
		{sshKey + "\n #comment\n", []string{sshKey}},
		{sshKey + "\ninvalid\n", []string{sshKey, "invalid"}},
	} {
		actual := ssh.SplitAuthorisedKeys(test.keyData)
		c.Assert(actual, gc.DeepEquals, test.expected)
	}
}

func b64decode(c *gc.C, s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	c.Assert(err, gc.IsNil)
	return b
}

func (s *AuthorisedKeysKeysSuite) TestParseAuthorisedKey(c *gc.C) {
	for i, test := range []struct {
		line    string
		key     []byte
		comment string
		err     string
	}{{
		line: sshtesting.ValidKeyOne.Key,
		key:  b64decode(c, strings.Fields(sshtesting.ValidKeyOne.Key)[1]),
	}, {
		line:    sshtesting.ValidKeyOne.Key + " a b c",
		key:     b64decode(c, strings.Fields(sshtesting.ValidKeyOne.Key)[1]),
		comment: "a b c",
	}, {
		line: "ssh-xsa blah",
		err:  "invalid authorized_key \"ssh-xsa blah\"",
	}, {
		// options should be skipped
		line: `no-pty,principals="\"",command="\!" ` + sshtesting.ValidKeyOne.Key,
		key:  b64decode(c, strings.Fields(sshtesting.ValidKeyOne.Key)[1]),
	}, {
		line: "ssh-rsa",
		err:  "invalid authorized_key \"ssh-rsa\"",
	}} {
		c.Logf("test %d: %s", i, test.line)
		ak, err := ssh.ParseAuthorisedKey(test.line)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, gc.IsNil)
			c.Assert(ak, gc.Not(gc.IsNil))
			c.Assert(ak.Key, gc.DeepEquals, test.key)
			c.Assert(ak.Comment, gc.Equals, test.comment)
		}
	}
}
