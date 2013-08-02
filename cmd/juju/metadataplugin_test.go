// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	gc "launchpad.net/gocheck"

	"fmt"
	"launchpad.net/juju-core/testing"
)

type MetadataSuite struct {
	jujuHome *testing.FakeHome
}

var _ = gc.Suite(&MetadataSuite{})

var metadataCommandNames = []string{
	"generate-image",
	"help",
	"validate-images",
}

func (s *MetadataSuite) SetUpTest(c *gc.C) {
	s.jujuHome = testing.MakeEmptyFakeHome(c)
}

func (s *MetadataSuite) TearDownTest(c *gc.C) {
	s.jujuHome.Restore()
}

func (s *MetadataSuite) TestHelpCommands(c *gc.C) {
	// Check that we have correctly registered all the sub commands
	// by checking the help output.
	out := badrun(c, 0, "help", "metadata")
	lines := strings.Split(out, "\n")
	var names []string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 || !strings.HasPrefix(line, "    ") {
			continue
		}
		names = append(names, f[0])
	}
	// The names should be output in alphabetical order, so don't sort.
	c.Assert(names, gc.DeepEquals, metadataCommandNames)
}

func (s *MetadataSuite) assertHelpOutput(c *gc.C, cmd string) {
	expected := fmt.Sprintf("usage: juju metadata %s [options]", cmd)
	out := badrun(c, 0, "help", "metadata", cmd)
	lines := strings.Split(out, "\n")
	c.Assert(lines[0], gc.Equals, expected)
	out = badrun(c, 0, "metadata", cmd, "--help")
	lines = strings.Split(out, "\n")
	c.Assert(lines[0], gc.Equals, expected)
}

func (s *MetadataSuite) TestHelpValidateImages(c *gc.C) {
	s.assertHelpOutput(c, "validate-images")
}

func (s *MetadataSuite) TestHelpGenerateImage(c *gc.C) {
	s.assertHelpOutput(c, "generate-image")
}
