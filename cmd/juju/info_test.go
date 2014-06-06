// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"os"

	jujutesting "github.com/juju/testing"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	_ "github.com/juju/juju/juju"
	"github.com/juju/juju/testing"
)

type InfoSimpleSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&InfoSimpleSuite{})

func (s *InfoSimpleSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
}

var testCreds = configstore.APICredentials{
	User:     "joe",
	Password: "baloney",
}

var apiEndpoint = configstore.APIEndpoint{
	Addresses: []string{"example.com", "kremvax.ru"},
	CACert:    "cert",
}

func patchEnvWithUser(c *gc.C, envName string) {
	// Patch APICredentials so that <envName>.jenv file is avaliable for
	// info to read the envirionment user from.
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	info, err := store.CreateInfo(envName)
	info.SetAPIEndpoint(apiEndpoint)
	info.SetAPICredentials(testCreds)
	info.Write()
}

func infoCommand() cmd.Command {
	return envcmd.Wrap(&InfoCommand{})
}

func (*InfoSimpleSuite) TestNoEnvironment(c *gc.C) {
	envPath := jujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, gc.IsNil)
	_, err = testing.RunCommand(c, infoCommand())
	c.Assert(err, gc.ErrorMatches, "open .*: no such file or directory")
}

func (*InfoSimpleSuite) TestNoDefault(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfigNoDefault)
	_, err := testing.RunCommand(c, infoCommand())
	c.Assert(err, gc.ErrorMatches, "no environment specified")
}

func (*InfoSimpleSuite) TestShowsDefault(c *gc.C) {
	context, err := testing.RunCommand(c, infoCommand())
	c.Assert(err, gc.IsNil)
	expected := `
environment-name: erewhemos
status: not running
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (*InfoSimpleSuite) TestUnknownEnvironment(c *gc.C) {
	context, err := testing.RunCommand(c, infoCommand(), "-e", "unknown-environment")
	c.Assert(err, gc.IsNil)
	expected := `
environment-name: unknown-environment
status: unknown
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (*InfoSimpleSuite) TestShowsRunning(c *gc.C) {
	patchEnvWithUser(c, "erewhemos")
	context, err := testing.RunCommand(c, infoCommand())
	c.Assert(err, gc.IsNil)
	expected := `
environment-name: erewhemos
user-name: joe
status: running
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (*InfoSimpleSuite) TestShowsRunningEnvironmentNotInEnvironmentsYAML(c *gc.C) {
	patchEnvWithUser(c, "new-environment")
	context, err := testing.RunCommand(c, infoCommand(), "-e", "new-environment")
	c.Assert(err, gc.IsNil)
	expected := `
environment-name: new-environment
user-name: joe
status: running
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (*InfoSimpleSuite) TestShowsRunningWithAll(c *gc.C) {
	patchEnvWithUser(c, "erewhemos")
	context, err := testing.RunCommand(c, infoCommand(), "--all")
	c.Assert(err, gc.IsNil)
	expected := `
environment-name: erewhemos
user-name: joe
status: running
api-endpoints:
- example.com
- kremvax.ru
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
}

func (*InfoSimpleSuite) TestShowsWithAllYAML(c *gc.C) {
	patchEnvWithUser(c, "erewhemos")
	context, err := testing.RunCommand(c, infoCommand(), "--all", "--format", "yaml")
	c.Assert(err, gc.IsNil)
	out := EnvInfo{}
	goyaml.Unmarshal([]byte(testing.Stdout(context)), &out)
	expected := EnvInfo{
		EnvironName:  "erewhemos",
		Username:     "joe",
		Status:       "running",
		APIEndpoints: []string{"example.com", "kremvax.ru"},
	}
	c.Assert(out, gc.DeepEquals, expected)
}

func (*InfoSimpleSuite) TestShowsWithAllJSON(c *gc.C) {
	patchEnvWithUser(c, "erewhemos")
	context, err := testing.RunCommand(c, infoCommand(), "--all", "--format", "json")
	c.Assert(err, gc.IsNil)
	out := EnvInfo{}
	json.Unmarshal([]byte(testing.Stdout(context)), &out)
	expected := EnvInfo{
		EnvironName:  "erewhemos",
		Username:     "joe",
		Status:       "running",
		APIEndpoints: []string{"example.com", "kremvax.ru"},
	}
	c.Assert(out, gc.DeepEquals, expected)
}

func (*InfoSimpleSuite) TestList(c *gc.C) {
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	_, err = store.CreateInfo("enva")
	c.Assert(err, gc.IsNil)
	_, err = store.CreateInfo("envb")
	c.Assert(err, gc.IsNil)
	_, err = store.CreateInfo("envc")
	c.Assert(err, gc.IsNil)

	context, err := testing.RunCommand(c, infoCommand(), "--list")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
enva
envb
envc
erewhemos
erewhemos-2
`[1:])
}

func (*InfoSimpleSuite) TestListAll(c *gc.C) {
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	_, err = store.CreateInfo("enva")
	c.Assert(err, gc.IsNil)

	context, err := testing.RunCommand(c, infoCommand(), "--list", "--all")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
- environment-name: enva
  status: running
- environment-name: erewhemos
  status: not running
- environment-name: erewhemos-2
  status: not running
`[1:])
}

func (*InfoSimpleSuite) TestListAllShortFlags(c *gc.C) {
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	_, err = store.CreateInfo("enva")
	c.Assert(err, gc.IsNil)

	context, err := testing.RunCommand(c, infoCommand(), "-l", "-a")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
- environment-name: enva
  status: running
- environment-name: erewhemos
  status: not running
- environment-name: erewhemos-2
  status: not running
`[1:])
}

func (*InfoSimpleSuite) TestListAllJSON(c *gc.C) {
	store, err := configstore.Default()
	c.Assert(err, gc.IsNil)
	_, err = store.CreateInfo("enva")
	c.Assert(err, gc.IsNil)

	context, err := testing.RunCommand(c, infoCommand(), "--list", "--all", "--format", "json")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
[{"environment-name":"enva","status":"running"},{"environment-name":"erewhemos","status":"not running"},{"environment-name":"erewhemos-2","status":"not running"}]
`[1:])
}

func (*InfoSimpleSuite) TestArgParsing(c *gc.C) {
	for i, test := range []struct {
		args     []string
		errMatch string
		envName  string
		all      bool
		list     bool
	}{
		{
			envName: "erewhemos",
		}, {
			args:    []string{"-e", "foo"},
			envName: "foo",
		}, {
			args:    []string{"--environment=foo"},
			envName: "foo",
		}, {
			args:    []string{"--all"},
			envName: "erewhemos",
			all:     true,
		}, {
			args:    []string{"-a"},
			envName: "erewhemos",
			all:     true,
		}, {
			args:    []string{"--list"},
			envName: "erewhemos",
			list:    true,
		}, {
			args:    []string{"-l"},
			envName: "erewhemos",
			list:    true,
		}, {
			args:     []string{"foo"},
			errMatch: `unrecognized args: \["foo"\]`,
		},
	} {
		c.Logf("test %v", i)
		command := &InfoCommand{}
		err := testing.InitCommand(envcmd.Wrap(command), test.args)
		if test.errMatch == "" {
			c.Check(err, gc.IsNil)
			c.Check(command.EnvName, gc.Equals, test.envName)
			c.Check(command.All, gc.Equals, test.all)
			c.Check(command.List, gc.Equals, test.list)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}
