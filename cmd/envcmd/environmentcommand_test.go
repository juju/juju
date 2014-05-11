// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd_test

import (
	"io/ioutil"
	"os"
	"testing"

	jc "github.com/juju/testing/checkers"
	"launchpad.net/gnuflag"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/osenv"
	coretesting "launchpad.net/juju-core/testing"
)

type EnvironmentCommandSuite struct {
	home *coretesting.FakeHome
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

func Test(t *testing.T) { gc.TestingT(t) }

func (s *EnvironmentCommandSuite) SetUpTest(c *gc.C) {
	s.home = coretesting.MakeEmptyFakeHome(c)
}

func (s *EnvironmentCommandSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentUnset(c *gc.C) {
	env := envcmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "")
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env := envcmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentNothingSet(c *gc.C) {
	env := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentJujuEnvSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	env := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentBothSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
}

func (s *EnvironmentCommandSuite) TestWriteAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentEnvironmentFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (*EnvironmentCommandSuite) TestErrorWritingFile(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentEnvironmentFilePath(), 0777)
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the environment file: .*")
}

func (s *EnvironmentCommandSuite) TestEnsureEnvName(c *gc.C) {
	// Take environment name from command line arg.
	cmd := initEnvCommandBase(c, "explicit")
	err := cmd.EnsureEnvName()
	c.Assert(err, gc.IsNil)
	c.Assert(cmd.EnvName, gc.Equals, "explicit")

	// Take environment name from the default.
	defer coretesting.MakeFakeHome(c, coretesting.MultipleEnvConfig).Restore()
	testEnsureEnvName(c, coretesting.SampleEnvName)

	// Take environment name from the one and only environment,
	// even if it is not explicitly marked as default.
	defer coretesting.MakeFakeHome(c, coretesting.SingleEnvConfigNoDefault).Restore()
	testEnsureEnvName(c, coretesting.SampleEnvName)

	// If there is a current-environment file, use that.
	err = envcmd.WriteCurrentEnvironment("fubar")
	testEnsureEnvName(c, "fubar")
}

func (s *EnvironmentCommandSuite) TestEnsureEnvNameErrors(c *gc.C) {
	err := initEnvCommandBase(c, "").EnsureEnvName()
	c.Assert(err, jc.Satisfies, environs.IsNoEnv)

	// If there are multiple environments but no default,
	// an error should be returned.
	defer coretesting.MakeFakeHome(c, coretesting.MultipleEnvConfigNoDefault).Restore()
	err = initEnvCommandBase(c, "").EnsureEnvName()
	c.Assert(err, gc.Equals, envcmd.ErrNoEnvironmentSpecified)
}

func initEnvCommandBase(c *gc.C, name string) *envcmd.EnvCommandBase {
	var flags gnuflag.FlagSet
	var cmd envcmd.EnvCommandBase
	cmd.SetFlags(&flags)
	var args []string
	if name != "" {
		args = []string{"-e", name}
	}
	err := flags.Parse(false, args)
	c.Assert(err, gc.IsNil)
	return &cmd
}

func testEnsureEnvName(c *gc.C, expect string) {
	cmd := initEnvCommandBase(c, "")
	err := cmd.EnsureEnvName()
	c.Assert(err, gc.IsNil)
	c.Assert(cmd.EnvName, gc.Equals, expect)
}
