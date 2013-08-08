// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"
	"strings"

	"launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type InitSuite struct {
}

var _ = gocheck.Suite(&InitSuite{})

func (*InitSuite) TestBoilerPlateEnvironment(c *gocheck.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	// run without an environments.yaml
	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, []string{"-w"})
	c.Check(code, gocheck.Equals, 0)
	outStr := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(outStr, "\n", "", -1)
	c.Check(strippedOut, gocheck.Matches, ".*A boilerplate environment configuration file has been written.*")
	environpath := testing.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, gocheck.IsNil)
	strippedData := strings.Replace(string(data), "\n", "", -1)
	c.Assert(strippedData, gocheck.Matches, ".*## This is the Juju config file, which you can use.*")
}

const existingEnv = `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`

func (*InitSuite) TestExistingEnvironmentNotOverwritten(c *gocheck.C) {
	defer testing.MakeFakeHome(c, existingEnv, "existing").Restore()

	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, []string{"-w"})
	c.Check(code, gocheck.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gocheck.Matches, ".*A juju environment configuration already exists.*")
	environpath := testing.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(data), gocheck.Equals, existingEnv)
}

// Without the write (-w) option, any existing environmens.yaml file is preserved and the boilerplate is
// written to stdout.
func (*InitSuite) TestPrintBoilerplate(c *gocheck.C) {
	defer testing.MakeFakeHome(c, existingEnv, "existing").Restore()

	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, nil)
	c.Check(code, gocheck.Equals, 0)
	errOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gocheck.Matches, ".*## This is the Juju config file, which you can use.*")
	environpath := testing.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, gocheck.IsNil)
	c.Assert(string(data), gocheck.Equals, existingEnv)
}
