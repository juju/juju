// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd/juju"
	cmdtesting "github.com/juju/juju/cmd/testing"
	jujutesting "github.com/juju/juju/testing"
)

// MetaResultString is the expected output of running dumpMetadata() on
// s.metaresult.
var MetaResultString = `
backup ID:       "spam"
started:         0001-01-01 00:00:00 +0000 UTC
finished:        0001-01-01 00:00:00 +0000 UTC
checksum:        ""
checksum format: ""
size (B):        0
stored:          false
notes:           ""
environment ID:  ""
machine ID:      ""
created on host: ""
juju version:    0.0.0
`[1:]

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *testing.T) {
	if *cmdtesting.FlagRunMain {
		jujucmd.Main(flag.Args())
	}
}

type BackupsSuite struct {
	jujutesting.FakeJujuHomeSuite
	metaresult *params.BackupsMetadataResult
}

func (s *BackupsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.metaresult = &params.BackupsMetadataResult{
		ID: "spam",
	}
}

func (s *BackupsSuite) diffStrings(c *gc.C, value, expected string) {
	// If only Go had a diff library.
	vlines := strings.Split(value, "\n")
	elines := strings.Split(expected, "\n")
	vsize := len(vlines)
	esize := len(elines)

	if vsize < 2 || esize < 2 {
		return
	}

	smaller := elines
	if vsize < esize {
		smaller = vlines
	}

	for i, _ := range smaller {
		vline := vlines[i]
		eline := elines[i]
		if vline != eline {
			c.Log("first mismatched line:")
			c.Log("expected: " + eline)
			c.Log("got:      " + vline)
			break
		}
	}

}

func (s *BackupsSuite) checkString(c *gc.C, value, expected string) {
	if !c.Check(value, gc.Equals, expected) {
		s.diffStrings(c, value, expected)
	}
}

func (s *BackupsSuite) checkStd(c *gc.C, ctx *cmd.Context, out, err string) {
	c.Check(ctx.Stdin.(*bytes.Buffer).String(), gc.Equals, "")
	s.checkString(c, ctx.Stdout.(*bytes.Buffer).String(), out)
	s.checkString(c, ctx.Stderr.(*bytes.Buffer).String(), err)
}

func (s *BackupsSuite) checkHelp(c *gc.C, subcommand, expected string) {

	// Run the command, ensuring it is actually there.
	args := []string{"juju", "backups"}
	if subcommand != "" {
		args = append(args, subcommand)
	}
	args = append(args, "--help")
	out := cmdtesting.BadRun(c, 0, args...)

	// Check the output.
	s.checkString(c, out, expected)
}
