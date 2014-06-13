// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api"
	"github.com/juju/juju/testing"
)

type BackupCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&BackupCommandSuite{})

// a fake client and command

var fakeBackupFilename = fmt.Sprintf(api.BACKUP_FILENAME, 1402686077)

type fakeBackupClient struct {
	err error
}

func (c *fakeBackupClient) Backup(backupFilePath string) (string, error) {
	if c.err != nil {
		return "", c.err
	}

	if backupFilePath == "" {
		backupFilePath = fakeBackupFilename
	}
	/* Don't actually do anything! */
	return backupFilePath, c.err
}

type fakeBackupCommand struct {
	BackupCommand
	client fakeBackupClient
}

func (c *fakeBackupCommand) Run(ctx *cmd.Context) error {
	return c.run(ctx, &c.client)
}

// help tests

func (s *BackupCommandSuite) TestBackupHelp(c *gc.C) {
	// Check the help output.

	info := (&BackupCommand{}).Info()

	expected := fmt.Sprintf(`
usage: juju %s [options] %s
purpose: %s

options:
...

%s
`, info.Args, info.Purpose, info.Doc)

	ctx, err := testing.RunCommand(c, &BackupCommand{}, "--help")

	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Matches, expected)
}

// options tests

func (s *BackupCommandSuite) TestBackupDefaults(c *gc.C) {
	client := fakeBackupClient{}
	command := fakeBackupCommand{client: client}

	ctx, err := testing.RunCommand(c, &command)

	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Matches, fakeBackupFilename+"\n")
}

func (s *BackupCommandSuite) TestBackupFilename(c *gc.C) {
	filename := "foo.tgz"
	client := fakeBackupClient{}
	command := fakeBackupCommand{client: client}

	ctx, err := testing.RunCommand(c, &command, filename)

	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(ctx), gc.Matches, filename+"\n")
}

func (s *BackupCommandSuite) TestBackupError(c *gc.C) {
	client := fakeBackupClient{err: errors.New("something went wrong")}
	command := fakeBackupCommand{client: client}

	_, err := testing.RunCommand(c, &command)

	c.Assert(err, gc.NotNil)
}
