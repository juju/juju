// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/testing"
)

type RestoreCommandSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&RestoreCommandSuite{})

// a fake client and command

type fakeRestoreClient struct {
	err error
}

func (c *fakeRestoreClient) Restore(backupFilePath string) error {
	if c.err != nil {
		return c.err
	}

	/* Don't actually do anything! */
	return c.err
}

type fakeRestoreCommand struct {
	RestoreCommand
	client fakeRestoreClient
}

func (c *fakeRestoreCommand) Run(ctx *cmd.Context) error {
	return c.runRestore(ctx, &c.client)
}

// help tests

// XXX Generalize for all commands?
func (s *RestoreCommandSuite) TestRestoreHelp(c *gc.C) {
	// Check the help output.
	info := (&RestoreCommand{}).Info()
	_usage := fmt.Sprintf("usage: juju %s [options] %s", info.Name, info.Args)
	_purpose := fmt.Sprintf("purpose: %s", info.Purpose)

	// Run the command, ensuring it is actually there.
	out := badrun(c, 0, "restore", "--help")
	out = out[:len(out)-1] // Strip the trailing \n.

	// Check the usage string.
	parts := strings.SplitN(out, "\n", 2)
	usage := parts[0]
	out = parts[1]
	c.Assert(usage, gc.Equals, _usage)

	// Check the purpose string.
	parts = strings.SplitN(out, "\n\n", 2)
	purpose := parts[0]
	out = parts[1]
	c.Assert(purpose, gc.Equals, _purpose)

	// Check the options.
	parts = strings.SplitN(out, "\n\n", 2)
	options := strings.Split(parts[0], "\n-")
	out = parts[1]
	c.Assert(options, gc.HasLen, 3)
	c.Assert(options[0], gc.Equals, "options:")
	c.Assert(strings.Contains(options[1], "constraints"), gc.Equals, true)
	c.Assert(strings.Contains(options[2], "environment"), gc.Equals, true)

	// Check the doc.
	doc := out
	c.Assert(doc, gc.Equals, info.Doc)
}

// options tests

func (s *RestoreCommandSuite) TestRestoreMissingFilename(c *gc.C) {
	client := fakeRestoreClient{}
	command := fakeRestoreCommand{client: client}

	_, err := testing.RunCommand(c, &command)

	c.Assert(err, gc.NotNil)
}

func (s *RestoreCommandSuite) TestRestoreFilename(c *gc.C) {
	filename := "foo.tgz"
	client := fakeRestoreClient{}
	command := fakeRestoreCommand{client: client}

	ctx, err := testing.RunCommand(c, &command, filename)

	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Matches, "restore from .* completed\n")
}

// failure tests

func (s *RestoreCommandSuite) TestRestoreError(c *gc.C) {
	filename := "foo.tgz"
	client := fakeRestoreClient{err: errors.New("something went wrong")}
	command := fakeRestoreCommand{client: client}

	_, err := testing.RunCommand(c, &command, filename)

	c.Assert(err, gc.NotNil)
}

func (s *RestoreCommandSuite) TestRestoreAPIIncompatibility(c *gc.C) {
	filename := "foo.tgz"
	failure := params.Error{
		Message: "bogus request",
		Code:    params.CodeNotImplemented,
	}
	client := fakeRestoreClient{err: &failure}
	command := fakeRestoreCommand{client: client}

	_, err := testing.RunCommand(c, &command, filename)

	c.Assert(err, gc.ErrorMatches, restoreAPIIncompatibility)
}
