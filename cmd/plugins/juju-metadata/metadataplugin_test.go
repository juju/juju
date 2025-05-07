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

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}

type MetadataSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&MetadataSuite{})

var metadataCommandNames = []string{
	"add-image",
	"delete-image",
	"generate-agent-binaries",
	"generate-image",
	"images",
	"list-images",
	"sign",
	"validate-agent-binaries",
	"validate-images",
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

func badrun(c *tc.C, exit int, args ...string) string {
	localArgs := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju-metadata"}, args...)

	ps := exec.Command(os.Args[0], localArgs...)
	output, err := ps.CombinedOutput()
	if exit != 0 {
		c.Assert(err, tc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}

func getHelpCommandNames(c *tc.C) []string {
	out := badrun(c, 0, "--help")
	c.Log(out)
	var names []string
	commandHelpStrings := strings.SplitAfter(out, "commands:")
	c.Assert(len(commandHelpStrings), tc.Equals, 2)
	commandHelp := strings.TrimSpace(commandHelpStrings[1])
	for _, line := range strings.Split(commandHelp, "\n") {
		names = append(names, strings.TrimSpace(strings.Split(line, " - ")[0]))
	}
	return names
}

func (s *MetadataSuite) TestHelpCommands(c *tc.C) {
	// Check that we have correctly registered all the sub commands
	// by checking the help output.
	c.Assert(getHelpCommandNames(c), tc.SameContents, metadataCommandNames)
}

func (s *MetadataSuite) assertHelpOutput(c *tc.C, cmd string) {
	expected := fmt.Sprintf("Usage: juju metadata %s [options]", cmd)
	out := badrun(c, 0, cmd, "--help")
	lines := strings.Split(out, "\n")
	c.Assert(lines[0], tc.Equals, expected)
}

func (s *MetadataSuite) TestHelpValidateImages(c *tc.C) {
	s.assertHelpOutput(c, "validate-images")
}

func (s *MetadataSuite) TestHelpValidateTools(c *tc.C) {
	s.assertHelpOutput(c, "validate-agent-binaries")
}

func (s *MetadataSuite) TestHelpGenerateImage(c *tc.C) {
	s.assertHelpOutput(c, "generate-image")
}

func (s *MetadataSuite) TestHelpImages(c *tc.C) {
	s.assertHelpOutput(c, "images")
}

func (s *MetadataSuite) TestHelpAddImage(c *tc.C) {
	s.assertHelpOutput(c, "add-image")
}

func (s *MetadataSuite) TestHelpDeleteImage(c *tc.C) {
	s.assertHelpOutput(c, "delete-image")
}
