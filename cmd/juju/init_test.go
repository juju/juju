// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"strings"
)

type InitSuite struct {
}

var _ = Suite(&InitSuite{})

// The environments.yaml is created by default if it
// does not already exist.
func (*InitSuite) TestBoilerPlateEnvironment(c *C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, nil)
	c.Check(code, Equals, 0)
	outStr := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(outStr, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*A boilerplate environment configuration file has been written.*")
	environpath := testing.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, IsNil)
	strippedData := strings.Replace(string(data), "\n", "", -1)
	c.Assert(strippedData, Matches, ".*## This is the Juju config file, which you can use.*")
}

// The boilerplate is sent to stdout with --show, and the environments.yaml
// is not created.
func (*InitSuite) TestBoilerPlatePrinted(c *C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, []string{"--show"})
	c.Check(code, Equals, 0)
	outStr := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(outStr, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*## This is the Juju config file, which you can use.*")
	environpath := testing.HomePath(".juju", "environments.yaml")
	_, err := ioutil.ReadFile(environpath)
	c.Assert(err, NotNil)
}

const existingEnv = `
environments:
    test:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`

// An existing environments.yaml will not be overwritten without
// the explicit -f option.
func (*InitSuite) TestExistingEnvironmentNotOverwritten(c *C) {
	defer testing.MakeFakeHome(c, existingEnv, "existing").Restore()

	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, nil)
	c.Check(code, Equals, 1)
	errOut := ctx.Stderr.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*A juju environment configuration already exists.*")
	environpath := testing.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, existingEnv)
}

// An existing environments.yaml will be overwritten when -f is
// given explicitly.
func (*InitSuite) TestExistingEnvironmentOverwritten(c *C) {
	defer testing.MakeFakeHome(c, existingEnv, "existing").Restore()

	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, []string{"-f"})
	c.Check(code, Equals, 0)
	stdOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(stdOut, "\n", "", -1)
	c.Check(strippedOut, Matches, ".*A boilerplate environment configuration file has been written.*")
	environpath := testing.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, IsNil)
	strippedData := strings.Replace(string(data), "\n", "", -1)
	c.Assert(strippedData, Matches, ".*## This is the Juju config file, which you can use.*")
}
