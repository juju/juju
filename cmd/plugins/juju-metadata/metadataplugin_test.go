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

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type MetadataSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&MetadataSuite{})

var metadataCommandNames = []string{
	"add-image",
	"delete-image",
	"generate-image",
	"generate-tools",
	"help",
	"list-images",
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

	ps.Env = append(os.Environ(), osenv.JujuXDGDataHomeEnvKey+"="+osenv.JujuXDGDataHome())
	output, err := ps.CombinedOutput()
	if exit != 0 {
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}

func getHelpCommandNames(c *gc.C) []string {
	out := badrun(c, 0, "--help")
	c.Log(out)
	var names []string
	commandHelpStrings := strings.SplitAfter(out, "commands:")
	c.Assert(len(commandHelpStrings), gc.Equals, 2)
	commandHelp := strings.TrimSpace(commandHelpStrings[1])
	for _, line := range strings.Split(commandHelp, "\n") {
		names = append(names, strings.TrimSpace(strings.Split(line, " - ")[0]))
	}
	return names
}

func (s *MetadataSuite) TestHelpCommands(c *gc.C) {
	// Check that we have correctly registered all the sub commands
	// by checking the help output.

	// Remove add/list-image for the first test because the feature is not
	// enabled by default.
	devFeatures := set.NewStrings("add-image", "list-images", "delete-image")

	// Remove features behind dev_flag for the first test since they are not
	// enabled.
	cmdSet := set.NewStrings(metadataCommandNames...).Difference(devFeatures)

	// Test default commands.
	// The names should be output in alphabetical order, so don't sort.
	c.Assert(getHelpCommandNames(c), jc.SameContents, cmdSet.Values())

	// Enable development features, and test again. We should now see the
	// development commands.
	s.SetFeatureFlags(feature.ImageMetadata)
	c.Assert(getHelpCommandNames(c), jc.SameContents, metadataCommandNames)
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

func (s *MetadataSuite) TestHelpListImages(c *gc.C) {
	s.SetFeatureFlags(feature.ImageMetadata)
	s.assertHelpOutput(c, "list-images")
}

func (s *MetadataSuite) TestHelpAddImage(c *gc.C) {
	s.SetFeatureFlags(feature.ImageMetadata)
	s.assertHelpOutput(c, "add-image")
}

func (s *MetadataSuite) TestHelpDeleteImage(c *gc.C) {
	s.SetFeatureFlags(feature.ImageMetadata)
	s.assertHelpOutput(c, "delete-image")
}
