// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	"fmt"
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/loggo"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type MetadataSuite struct{}

var _ = gc.Suite(&MetadataSuite{})

var (
	flagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")
)

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *testing.T) {
	if *flagRunMain {
		Main(flag.Args())
	}
}

func badrun(c *gc.C, exit int, args ...string) string {
	localArgs := append([]string{"-test.run", "TestRunMain", "-run-main", "--", "juju"}, args...)
	ps := exec.Command(os.Args[0], localArgs...)
	ps.Env = append(os.Environ(), "JUJU_HOME="+config.JujuHome())
	output, err := ps.CombinedOutput()
	if exit != 0 {
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}

func (s *MetadataSuite) TestTearDown(c *gc.C) {
	loggo.ResetLoggers()
}

var commandNames = []string{
	"generate-image",
	"help",
	"validate-image",
}

func (s *MetadataSuite) TestHelpCommands(c *gc.C) {
	// Check that we have correctly registered all the commands
	// by checking the help output.
	defer config.SetJujuHome(config.SetJujuHome(c.MkDir()))
	out := badrun(c, 0, "help", "commands")
	lines := strings.Split(out, "\n")
	var names []string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		names = append(names, f[0])
	}
	// The names should be output in alphabetical order, so don't sort.
	c.Assert(names, gc.DeepEquals, commandNames)
}

var globalFlags = []string{
	"--debug .*",
	"-h, --help .*",
	"--log-config .*",
	"--log-file .*",
	"-v, --verbose .*",
}

func (s *MetadataSuite) TestHelpGlobalOptions(c *gc.C) {
	// Check that we have correctly registered all the topics
	// by checking the help output.
	defer config.SetJujuHome(config.SetJujuHome(c.MkDir()))
	out := badrun(c, 0, "help", "global-options")
	c.Assert(out, gc.Matches, `Global Options

These options may be used with any command, and may appear in front of any
command\.(.|\n)*`)
	lines := strings.Split(out, "\n")
	var flags []string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) == 0 || line[0] != '-' {
			continue
		}
		flags = append(flags, line)
	}
	c.Assert(len(flags), gc.Equals, len(globalFlags))
	for i, line := range flags {
		c.Assert(line, gc.Matches, globalFlags[i])
	}
}
