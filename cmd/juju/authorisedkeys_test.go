// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	jujutesting "launchpad.net/juju-core/juju/testing"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

type AuthorisedKeysSuite struct {
	testbase.LoggingSuite
	jujuHome *coretesting.FakeHome
}

var _ = gc.Suite(&AuthorisedKeysSuite{})

var authKeysCommandNames = []string{
	"help",
	"list",
}

func (s *AuthorisedKeysSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.jujuHome = coretesting.MakeEmptyFakeHome(c)
}

func (s *AuthorisedKeysSuite) TearDownTest(c *gc.C) {
	s.jujuHome.Restore()
	s.LoggingSuite.TearDownTest(c)
}

func (s *AuthorisedKeysSuite) TestHelpCommands(c *gc.C) {
	// Check that we have correctly registered all the sub commands
	// by checking the help output.
	out := badrun(c, 0, "authorised-keys", "--help")
	lines := strings.Split(out, "\n")
	var names []string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 || !strings.HasPrefix(line, "    ") {
			continue
		}
		names = append(names, f[0])
	}
	// The names should be output in alphabetical order, so don't sort.
	c.Assert(names, gc.DeepEquals, authKeysCommandNames)
}

func (s *AuthorisedKeysSuite) assertHelpOutput(c *gc.C, cmd, args string) {
	if args != "" {
		args = " " + args
	}
	expected := fmt.Sprintf("usage: juju authorised-keys %s [options]%s", cmd, args)
	out := badrun(c, 0, "authorised-keys", cmd, "--help")
	lines := strings.Split(out, "\n")
	c.Assert(lines[0], gc.Equals, expected)
}

func (s *AuthorisedKeysSuite) TestHelpList(c *gc.C) {
	s.assertHelpOutput(c, "list", "")
}

type ListKeysSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&ListKeysSuite{})

func (s *ListKeysSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.PatchEnvironment(osenv.JujuEnv, "dummyenv")
}

func (s *ListKeysSuite) setAuthorisedKeys(c *gc.C, keys string) {
	err := statetesting.UpdateConfig(s.State, map[string]interface{}{"authorized-keys": keys})
	c.Assert(err, gc.IsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(envConfig.AuthorizedKeys(), gc.Equals, keys)
}

func (s *ListKeysSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2}, "\n"))

	context, err := coretesting.RunCommand(c, &ListKeysCommand{}, []string{})
	c.Assert(err, gc.IsNil)
	output := strings.TrimSpace(coretesting.Stdout(context))
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.Matches, "Keys for user admin:\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestListFullKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2}, "\n"))

	context, err := coretesting.RunCommand(c, &ListKeysCommand{}, []string{"--full"})
	c.Assert(err, gc.IsNil)
	output := strings.TrimSpace(coretesting.Stdout(context))
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.Matches, "Keys for user admin:\n.*user@host\n.*another@host")
}

func (s *ListKeysSuite) TestListKeysNonDefaultUser(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2}, "\n"))
	_, err := s.State.AddUser("fred", "password")
	c.Assert(err, gc.IsNil)

	context, err := coretesting.RunCommand(c, &ListKeysCommand{}, []string{"--user", "fred"})
	c.Assert(err, gc.IsNil)
	output := strings.TrimSpace(coretesting.Stdout(context))
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.Matches, "Keys for user fred:\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestTooManyArgs(c *gc.C) {
	_, err := coretesting.RunCommand(c, &ListKeysCommand{}, []string{"foo"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["foo"\]`)
}
