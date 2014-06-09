// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"os"

	gitjujutesting "github.com/juju/testing"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	_ "github.com/juju/juju/juju"
	"github.com/juju/juju/testing"
)

type SwitchSimpleSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&SwitchSimpleSuite{})

var testCreds = configstore.APICredentials{
	User:     "joe",
	Password: "baloney",
}

var apiEndpoint = configstore.APIEndpoint{
	Addresses: []string{"example.com", "kremvax.ru"},
	CACert:    "cert",
}

func patchEnvWithUser(c *gc.C, envName string) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)

	// Patch APICredentials so that <envName>.jenv file is avaliable for
	// switch to read the envirionment user from.
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	info, err := store.CreateInfo(envName)
	info.SetAPIEndpoint(apiEndpoint)
	info.SetAPICredentials(testCreds)
	info.Write()
}

func (*SwitchSimpleSuite) TestNoEnvironment(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "erewhemos\n")
}

func (*SwitchSimpleSuite) TestNoJenvYAML(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--format", "yaml")
	c.Assert(err, gc.IsNil)
	output := EnvInfo{}
	goyaml.Unmarshal([]byte(testing.Stdout(context)), &output)
	expected := EnvInfo{
		Username:    "",
		EnvironName: "erewhemos",
	}
	c.Assert(output, gc.DeepEquals, expected)
}

func (*SwitchSimpleSuite) TestNoJenvJson(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--format", "json")
	c.Assert(err, gc.IsNil)
	output := EnvInfo{}
	json.Unmarshal([]byte(testing.Stdout(context)), &output)
	expected := EnvInfo{
		Username:    "",
		EnvironName: "erewhemos",
	}
	c.Assert(output, gc.DeepEquals, expected)
}

func (*SwitchSimpleSuite) TestEnvInfoOmitFields(c *gc.C) {
	jsonInfo, err := json.Marshal(EnvInfo{})
	c.Assert(err, gc.IsNil)
	c.Assert(string(jsonInfo), gc.Equals, `{"user-name":"","environ-name":""}`)
	yamlInfo, err := goyaml.Marshal(EnvInfo{})
	c.Assert(string(yamlInfo), gc.Equals, `user-name: ""`+"\n"+`environ-name: ""`+"\n")

}

func (s *SwitchSimpleSuite) TestCurrentEnvironmentHasPrecidence(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	s.FakeHomeSuite.Home.AddFiles(c, gitjujutesting.TestFile{".juju/current-environment", "fubar"})
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "fubar\n")
}

func (*SwitchSimpleSuite) TestShowsJujuEnv(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "using-env\n")
}

func (s *SwitchSimpleSuite) TestJujuEnvOverCurrentEnvironment(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	s.FakeHomeSuite.Home.AddFiles(c, gitjujutesting.TestFile{".juju/current-environment", "fubar"})
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "using-env\n")
}

func (*SwitchSimpleSuite) TestSettingWritesFile(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "erewhemos-2")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "erewhemos -> erewhemos-2\n")
	c.Assert(envcmd.ReadCurrentEnvironment(), gc.Equals, "erewhemos-2")
}

func (*SwitchSimpleSuite) TestSettingToUnknown(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "unknown")
	c.Assert(err, gc.ErrorMatches, `"unknown" is not a name of an existing defined environment`)
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
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, expectedEnvironments)
}

func (*SwitchSimpleSuite) TestListEnvironmentsOSJujuEnvSet(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--list")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, expectedEnvironments)
}

func (*SwitchSimpleSuite) TestListEnvironmentsAndChange(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "--list", "erewhemos-2")
	c.Assert(err, gc.ErrorMatches, "cannot switch and list at the same time")
}

func (*SwitchSimpleSuite) TestShowEnvironmentInfoYaml(c *gc.C) {
	patchEnvWithUser(c, "erewhemos")
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--format", "yaml")
	c.Assert(err, gc.IsNil)
	output := EnvInfo{}
	goyaml.Unmarshal([]byte(testing.Stdout(context)), &output)
	expected := EnvInfo{
		Username:     "joe",
		EnvironName:  "erewhemos",
		StateServers: []string{"example.com", "kremvax.ru"},
	}
	c.Assert(output, gc.DeepEquals, expected)
}

func (*SwitchSimpleSuite) TestShowEnvironmentInfoJson(c *gc.C) {
	patchEnvWithUser(c, "erewhemos")
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--format", "json")
	c.Assert(err, gc.IsNil)
	output := EnvInfo{}
	json.Unmarshal([]byte(testing.Stdout(context)), &output)
	expected := EnvInfo{
		Username:     "joe",
		EnvironName:  "erewhemos",
		StateServers: []string{"example.com", "kremvax.ru"},
	}
	c.Assert(output, gc.DeepEquals, expected)
}

func (*SwitchSimpleSuite) TestShowEnvironmentInfoAndOldEnv(c *gc.C) {
	patchEnvWithUser(c, "erewhemos-2")
	context, err := testing.RunCommand(c, &SwitchCommand{}, "erewhemos-2", "--format", "json")
	c.Assert(err, gc.IsNil)
	output := EnvInfo{}
	json.Unmarshal([]byte(testing.Stdout(context)), &output)
	expected := EnvInfo{
		Username:            "joe",
		EnvironName:         "erewhemos-2",
		PreviousEnvironName: "erewhemos",
		StateServers:        []string{"example.com", "kremvax.ru"},
	}
	c.Assert(output, gc.DeepEquals, expected)
}

func (*SwitchSimpleSuite) TestTooManyParams(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: ."bar".`)
}
