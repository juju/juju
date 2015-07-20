// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/cmd"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type InitSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&InitSuite{})

// The environments.yaml is created by default if it
// does not already exist.
func (*InitSuite) TestBoilerPlateEnvironment(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, nil)
	c.Check(code, gc.Equals, 0)
	outStr := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(outStr, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, ".*A boilerplate environment configuration file has been written.*")
	environpath := gitjujutesting.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, jc.ErrorIsNil)
	strippedData := strings.Replace(string(data), "\n", "", -1)
	c.Assert(strippedData, gc.Matches, ".*# This is the Juju config file, which you can use.*")
}

// The boilerplate is sent to stdout with --show, and the environments.yaml
// is not created.
func (*InitSuite) TestBoilerPlatePrinted(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, []string{"--show"})
	c.Check(code, gc.Equals, 0)
	outStr := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(outStr, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, ".*# This is the Juju config file, which you can use.*")
	environpath := gitjujutesting.HomePath(".juju", "environments.yaml")
	_, err = ioutil.ReadFile(environpath)
	c.Assert(err, gc.NotNil)
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
func (*InitSuite) TestExistingEnvironmentNotOverwritten(c *gc.C) {
	testing.WriteEnvironments(c, existingEnv)

	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, nil)
	c.Check(code, gc.Equals, 1)
	errOut := ctx.Stderr.(*bytes.Buffer).String()
	strippedOut := strings.Replace(errOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, ".*A juju environment configuration already exists.*")
	environpath := gitjujutesting.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, existingEnv)
}

// An existing environments.yaml will be overwritten when -f is
// given explicitly.
func (*InitSuite) TestExistingEnvironmentOverwritten(c *gc.C) {
	testing.WriteEnvironments(c, existingEnv)

	ctx := testing.Context(c)
	code := cmd.Main(&InitCommand{}, ctx, []string{"-f"})
	c.Check(code, gc.Equals, 0)
	stdOut := ctx.Stdout.(*bytes.Buffer).String()
	strippedOut := strings.Replace(stdOut, "\n", "", -1)
	c.Check(strippedOut, gc.Matches, ".*A boilerplate environment configuration file has been written.*")
	environpath := gitjujutesting.HomePath(".juju", "environments.yaml")
	data, err := ioutil.ReadFile(environpath)
	c.Assert(err, jc.ErrorIsNil)
	strippedData := strings.Replace(string(data), "\n", "", -1)
	c.Assert(strippedData, gc.Matches, ".*# This is the Juju config file, which you can use.*")
}
