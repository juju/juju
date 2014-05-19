// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type MetadataSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&MetadataSuite{})

var metadataCommandNames = []string{
	"generate-image",
	"generate-tools",
	"help",
	"sign",
	"validate-images",
	"validate-tools",
}

var (
	flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")
)

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *flagRunMain {
		Main(flag.Args())
	}
}

func badrun(c *gc.C, exit int, args ...string) string {
	localArgs := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju-metadata"}, args...)

	ps := exec.Command(os.Args[0], localArgs...)

	ps.Env = append(os.Environ(), osenv.JujuHomeEnvKey+"="+osenv.JujuHome())
	output, err := ps.CombinedOutput()
	if exit != 0 {
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}

func (s *MetadataSuite) TestHelpCommands(c *gc.C) {
	// Check that we have correctly registered all the sub commands
	// by checking the help output.
	out := badrun(c, 0, "--help")
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
	out := badrun(c, 0, cmd, "--help")
	lines := strings.Split(out, "\n")
	c.Assert(lines[0], gc.Equals, expected)
}

func (s *MetadataSuite) TestHelpValidateImages(c *gc.C) {
	s.assertHelpOutput(c, "validate-images")
}

func (s *MetadataSuite) TestHelpValidateTools(c *gc.C) {
	s.assertHelpOutput(c, "validate-tools")
}

func (s *MetadataSuite) TestHelpGenerateImage(c *gc.C) {
	s.assertHelpOutput(c, "generate-image")
}
