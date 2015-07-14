// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"os"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/feature"
	_ "github.com/juju/juju/juju"
	"github.com/juju/juju/testing"
)

type SwitchSimpleSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&SwitchSimpleSuite{})

func (*SwitchSimpleSuite) TestNoEnvironment(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, jc.ErrorIsNil)
	_, err = testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.ErrorMatches, "couldn't read the environment")
}

func (*SwitchSimpleSuite) TestNoDefault(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfigNoDefault)
	_, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.ErrorMatches, "no currently specified environment")
}

func (*SwitchSimpleSuite) TestShowsDefault(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "erewhemos\n")
}

func (s *SwitchSimpleSuite) TestCurrentEnvironmentHasPrecedence(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	envcmd.WriteCurrentEnvironment("fubar")
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "fubar\n")
}

func (s *SwitchSimpleSuite) TestCurrentSystemHasPrecedence(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	envcmd.WriteCurrentSystem("fubar")
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "fubar (system)\n")
}

func (*SwitchSimpleSuite) TestShowsJujuEnv(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "using-env\n")
}

func (s *SwitchSimpleSuite) TestJujuEnvOverCurrentEnvironment(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	s.FakeHomeSuite.Home.AddFiles(c, gitjujutesting.TestFile{".juju/current-environment", "fubar"})
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "using-env\n")
}

func (*SwitchSimpleSuite) TestSettingWritesFile(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "erewhemos-2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "-> erewhemos-2\n")
	currentEnv, err := envcmd.ReadCurrentEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentEnv, gc.Equals, "erewhemos-2")
}

func (s *SwitchSimpleSuite) addTestSystem(c *gc.C) {
	// First set up a system in the config store.
	s.SetFeatureFlags(feature.JES)
	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	info := store.CreateInfo("a-system")
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:  []string{"localhost"},
		CACert:     testing.CACert,
		ServerUUID: "server-uuid",
	})
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SwitchSimpleSuite) TestSettingWritesSystemFile(c *gc.C) {
	s.addTestSystem(c)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "a-system")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "-> a-system (system)\n")
	currSystem, err := envcmd.ReadCurrentSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currSystem, gc.Equals, "a-system")
}

func (s *SwitchSimpleSuite) TestListWithSystem(c *gc.C) {
	s.addTestSystem(c)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
a-system (system)
erewhemos
`[1:])
}

func (*SwitchSimpleSuite) TestSettingToUnknown(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "unknown")
	c.Assert(err, gc.ErrorMatches, `"unknown" is not a name of an existing defined environment or system`)
}

func (*SwitchSimpleSuite) TestSettingWhenJujuEnvSet(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	os.Setenv("JUJU_ENV", "using-env")
	_, err := testing.RunCommand(c, &SwitchCommand{}, "erewhemos-2")
	c.Assert(err, gc.ErrorMatches, `cannot switch when JUJU_ENV is overriding the environment \(set to "using-env"\)`)
}

const expectedEnvironments = `erewhemos
erewhemos-2
`

func (*SwitchSimpleSuite) TestListEnvironments(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, expectedEnvironments)
}

func (s *SwitchSimpleSuite) TestListEnvironmentsWithConfigstore(c *gc.C) {
	memstore := configstore.NewMem()
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return memstore, nil
	})
	info := memstore.CreateInfo("testing")
	err := info.Write()
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--list")
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedEnvironments + "testing\n"
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (*SwitchSimpleSuite) TestListEnvironmentsOSJujuEnvSet(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, expectedEnvironments)
}

func (*SwitchSimpleSuite) TestListEnvironmentsAndChange(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "--list", "erewhemos-2")
	c.Assert(err, gc.ErrorMatches, "cannot switch and list at the same time")
}

func (*SwitchSimpleSuite) TestTooManyParams(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: ."bar".`)
}
