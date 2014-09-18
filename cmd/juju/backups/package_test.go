// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd/juju"
	"github.com/juju/juju/cmd/juju/backups"
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

type BaseBackupsSuite struct {
	jujutesting.FakeJujuHomeSuite
	command    *backups.Command
	metaresult *params.BackupsMetadataResult
}

func (s *BaseBackupsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.command = backups.NewCommand().(*backups.Command)
	s.metaresult = &params.BackupsMetadataResult{
		ID: "spam",
	}
}

func (s *BaseBackupsSuite) patchAPIClient(client backups.APIClient) {
	s.PatchValue(backups.NewAPIClient,
		func(c *backups.CommandBase) (backups.APIClient, error) {
			return client, nil
		},
	)
}

func (s *BaseBackupsSuite) setSuccess() {
	s.patchAPIClient(&fakeAPIClient{metaresult: s.metaresult})
}

func (s *BaseBackupsSuite) setFailure(failure string) {
	s.patchAPIClient(&fakeAPIClient{err: errors.New(failure)})
}

func (s *BaseBackupsSuite) diffStrings(c *gc.C, value, expected string) {
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

func (s *BaseBackupsSuite) checkString(c *gc.C, value, expected string) {
	if !c.Check(value, gc.Equals, expected) {
		s.diffStrings(c, value, expected)
	}
}

func (s *BaseBackupsSuite) checkStd(c *gc.C, ctx *cmd.Context, out, err string) {
	c.Check(ctx.Stdin.(*bytes.Buffer).String(), gc.Equals, "")
	s.checkString(c, ctx.Stdout.(*bytes.Buffer).String(), out)
	s.checkString(c, ctx.Stderr.(*bytes.Buffer).String(), err)
}

type fakeAPIClient struct {
	metaresult *params.BackupsMetadataResult
	err        error
}

func (c *fakeAPIClient) Create(notes string) (*params.BackupsMetadataResult, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.metaresult, nil
}

func (c *fakeAPIClient) Close() error {
	return nil
}
