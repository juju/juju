// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"github.com/juju/tc"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

type BSInteractSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(BSInteractSuite{})

func (BSInteractSuite) TestInitEmpty(c *tc.C) {
	cmd := &bootstrapCommand{}
	err := cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.interactive, tc.IsTrue)
}

func (BSInteractSuite) TestInitBuildAgent(c *tc.C) {
	cmd := &bootstrapCommand{}
	err := cmdtesting.InitCommand(cmd, []string{"--build-agent"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.interactive, tc.IsTrue)
	c.Assert(cmd.BuildAgent, tc.IsTrue)
}

func (BSInteractSuite) TestInitArg(c *tc.C) {
	cmd := &bootstrapCommand{}
	err := cmdtesting.InitCommand(cmd, []string{"foo"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.interactive, tc.IsFalse)
}

func (BSInteractSuite) TestInitTwoArgs(c *tc.C) {
	cmd := &bootstrapCommand{}
	err := cmdtesting.InitCommand(cmd, []string{"foo", "bar"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.interactive, tc.IsFalse)
}

func (BSInteractSuite) TestInitInfoOnlyFlag(c *tc.C) {
	cmd := &bootstrapCommand{}
	err := cmdtesting.InitCommand(cmd, []string{"--clouds"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.interactive, tc.IsFalse)
}

func (BSInteractSuite) TestInitVariousFlags(c *tc.C) {
	cmd := &bootstrapCommand{}
	err := cmdtesting.InitCommand(cmd, []string{"--keep-broken", "--agent-version", version.Current.String()})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd.interactive, tc.IsTrue)
}

func (BSInteractSuite) TestQueryCloud(c *tc.C) {
	input := "search\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	clouds := []string{"books", "books-china", "search", "local"}

	buf := bytes.Buffer{}
	cloud, err := queryCloud(clouds, "local", scanner, &buf)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloud, tc.Equals, "search")

	// clouds should be printed out in the same order as they're given.
	expected := `
Clouds
books
books-china
search
local

Select a cloud [local]: 
`[1:]
	c.Assert(buf.String(), tc.Equals, expected)
}

func (BSInteractSuite) TestQueryCloudDefault(c *tc.C) {
	input := "\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	clouds := []string{"books", "local"}

	cloud, err := queryCloud(clouds, "local", scanner, io.Discard)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloud, tc.Equals, "local")
}

func (BSInteractSuite) TestInvalidCloud(c *tc.C) {
	input := "bad^cloud\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	clouds := []string{"books", "local", "bad^cloud"}

	_, err := queryCloud(clouds, "local", scanner, io.Discard)
	c.Assert(err, tc.ErrorMatches, `cloud name "bad\^cloud" not valid`)
}

func (BSInteractSuite) TestQueryRegion(c *tc.C) {
	input := "mars-west1\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	regions := []jujucloud.Region{
		{Name: "mars-east1"},
		{Name: "mars-west1"},
		{Name: "jupiter-central"},
	}

	buf := bytes.Buffer{}
	region, err := queryRegion("goggles", regions, scanner, &buf)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(region, tc.Equals, "mars-west1")

	// regions should be alphabetized, and the first one in the original list
	// should be the default.
	expected := `
Regions in goggles:
jupiter-central
mars-east1
mars-west1

Select a region in goggles [mars-east1]: 
`[1:]
	c.Assert(buf.String(), tc.Equals, expected)
}

func (BSInteractSuite) TestQueryRegionDefault(c *tc.C) {
	input := "\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	regions := []jujucloud.Region{
		{Name: "mars-east1"},
		{Name: "jupiter-central"},
	}

	region, err := queryRegion("goggles", regions, scanner, io.Discard)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(region, tc.Equals, regions[0].Name)
}

func (BSInteractSuite) TestQueryName(c *tc.C) {
	input := "awesome-cloud\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	buf := bytes.Buffer{}
	name, err := queryName("default-cloud", scanner, &buf)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "awesome-cloud")

	expected := `
Enter a name for the Controller [default-cloud]: 
`[1:]
	c.Assert(buf.String(), tc.Equals, expected)
}

func (BSInteractSuite) TestQueryNameDefault(c *tc.C) {
	input := "\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	name, err := queryName("default-cloud", scanner, io.Discard)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "default-cloud")
}
