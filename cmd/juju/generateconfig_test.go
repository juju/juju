package main

import (
	"io/ioutil"

	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/testing"
	"strings"
)

type GenerateConfigSuite struct {
}

var _ = Suite(&GenerateConfigSuite{})

func (*GenerateConfigSuite) TestBoilerPlateEnvironment(c *C) {
	defer makeFakeHome(c, "empty").restore()
	// run without an environments.yaml
	ctx := testing.Context(c)
	code := cmd.Main(&GenerateConfigCommand{}, ctx, []string{"-w"})
	c.Check(code, Equals, 0)
	outStr := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(outStr, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*A boilerplate environment configuration file has been written.*")
	environpath := homePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, IsNil)
	strippedData := strings.Replace(string(data), "\n", "", -1)
	c.Assert(strippedData, Matches, ".*## This is the Juju config file, which you can use.*")
}

func (*GenerateConfigSuite) TestExistingEnvironmentNotOverwritten(c *C) {
	defer makeFakeHome(c, "existing").restore()
	env := `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	environpath := homePath(".juju", "environments.yaml")
	_, err := environs.WriteEnvirons(environpath, env)
	c.Assert(err, IsNil)

	ctx := testing.Context(c)
	code := cmd.Main(&GenerateConfigCommand{}, ctx, []string{"-w"})
	c.Check(code, Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*A juju environment configuration already exists.*")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, env)
}

// Without the write (-w) option, any existing environmens.yaml file is preserved and the boilerplate is
// written to stdout.
func (*GenerateConfigSuite) TestPrintBoilerplate(c *C) {
	defer makeFakeHome(c, "existing").restore()
	env := `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	environpath := homePath(".juju", "environments.yaml")
	_, err := environs.WriteEnvirons(environpath, env)
	c.Assert(err, IsNil)

	ctx := testing.Context(c)
	code := cmd.Main(&GenerateConfigCommand{}, ctx, nil)
	c.Check(code, Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*## This is the Juju config file, which you can use.*")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, env)
}
