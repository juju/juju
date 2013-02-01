package main

import (
	"io/ioutil"

	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"strings"
)

type GenerateConfigSuite struct {
}

var _ = Suite(&GenerateConfigSuite{})

func (*GenerateConfigSuite) TestBoilerPlateEnvironment(c *C) {
	defer makeFakeHome(c, "empty").restore()
	// run without an environments.yaml
	ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
	code := cmd.Main(&GenerateConfigCommand{}, ctx, nil)
	c.Check(code, Equals, 0)
	outStr := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(outStr, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*A boilerplate environment configuration file has been written.*")
	environpath := homePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, environs.BoilerPlateConfig())
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

	// run without an environments.yaml
	ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
	code := cmd.Main(&GenerateConfigCommand{}, ctx, nil)
	c.Check(code, Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*A juju environment configuration already exists.*")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, env)
}
