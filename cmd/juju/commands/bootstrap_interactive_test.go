// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type BSInteractSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(BSInteractSuite{})

func (BSInteractSuite) TestInitEmpty(c *gc.C) {
	cmd := &bootstrapCommand{}
	err := jujutesting.InitCommand(cmd, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.interactive, jc.IsTrue)
}

func (BSInteractSuite) TestInitBuildAgent(c *gc.C) {
	cmd := &bootstrapCommand{}
	err := jujutesting.InitCommand(cmd, []string{"--build-agent"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.interactive, jc.IsTrue)
	c.Assert(cmd.BuildAgent, jc.IsTrue)
}

func (BSInteractSuite) TestInitArg(c *gc.C) {
	cmd := &bootstrapCommand{}
	err := jujutesting.InitCommand(cmd, []string{"foo"})
	c.Assert(err, gc.ErrorMatches, "controller name and cloud name are required")
	c.Assert(cmd.interactive, jc.IsFalse)
}

func (BSInteractSuite) TestInitTwoArgs(c *gc.C) {
	cmd := &bootstrapCommand{}
	err := jujutesting.InitCommand(cmd, []string{"foo", "bar"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.interactive, jc.IsFalse)
}

func (BSInteractSuite) TestInitInfoOnlyFlag(c *gc.C) {
	cmd := &bootstrapCommand{}
	err := jujutesting.InitCommand(cmd, []string{"--clouds"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.interactive, jc.IsFalse)
}

func (BSInteractSuite) TestInitVariousFlags(c *gc.C) {
	cmd := &bootstrapCommand{}
	err := jujutesting.InitCommand(cmd, []string{"--keep-broken", "--agent-version", version.Current.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.interactive, jc.IsTrue)
}

func (BSInteractSuite) TestQueryCloud(c *gc.C) {
	input := "search\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	clouds := []string{"books", "books-china", "search", "local"}

	buf := bytes.Buffer{}
	cloud, err := queryCloud(clouds, "local", scanner, &buf)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, gc.Equals, "search")

	// clouds should be printed out in the same order as they're given.
	expected := `
Clouds
books
books-china
search
local

Select a cloud [local]: 
`[1:]
	c.Assert(buf.String(), gc.Equals, expected)
}

func (BSInteractSuite) TestQueryCloudDefault(c *gc.C) {
	input := "\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	clouds := []string{"books", "local"}

	cloud, err := queryCloud(clouds, "local", scanner, ioutil.Discard)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, gc.Equals, "local")
}

func (BSInteractSuite) TestQueryRegion(c *gc.C) {
	input := "mars-west1\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	regions := []jujucloud.Region{
		{Name: "mars-east1"},
		{Name: "mars-west1"},
		{Name: "jupiter-central"},
	}

	buf := bytes.Buffer{}
	region, err := queryRegion("goggles", regions, scanner, &buf)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(region, gc.Equals, "mars-west1")

	// regions should be alphabetized, and the first one in the original list
	// should be the default.
	expected := `
Regions in goggles:
jupiter-central
mars-east1
mars-west1

Select a region in goggles [mars-east1]: 
`[1:]
	c.Assert(buf.String(), gc.Equals, expected)
}

func (BSInteractSuite) TestQueryRegionDefault(c *gc.C) {
	input := "\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	regions := []jujucloud.Region{
		{Name: "mars-east1"},
		{Name: "jupiter-central"},
	}

	region, err := queryRegion("goggles", regions, scanner, ioutil.Discard)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(region, gc.Equals, regions[0].Name)
}

func (BSInteractSuite) TestQueryName(c *gc.C) {
	input := "awesome-cloud\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	buf := bytes.Buffer{}
	name, err := queryName("default-cloud", scanner, &buf)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "awesome-cloud")

	expected := `
Enter a name for the Controller [default-cloud]: 
`[1:]
	c.Assert(buf.String(), gc.Equals, expected)
}

func (BSInteractSuite) TestQueryNameDefault(c *gc.C) {
	input := "\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	name, err := queryName("default-cloud", scanner, ioutil.Discard)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "default-cloud")
}
