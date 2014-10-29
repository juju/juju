// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"
	"os"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type downloadSuite struct {
	BaseBackupsSuite
	subcommand *backups.DownloadCommand
	data       string
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.subcommand = &backups.DownloadCommand{}

	s.data = "<compressed archive data>"
}

func (s *downloadSuite) TearDownTest(c *gc.C) {
	filename := s.subcommand.ResolveFilename()
	err := os.Remove(filename)
	if !os.IsNotExist(err) {
		c.Check(err, gc.IsNil)
	}

	s.BaseBackupsSuite.TearDownTest(c)
}

func (s *downloadSuite) setSuccess() *fakeAPIClient {
	s.subcommand.ID = s.metaresult.ID
	client := s.BaseBackupsSuite.setSuccess()
	client.archive = ioutil.NopCloser(bytes.NewBufferString(s.data))
	return client
}

func (s *downloadSuite) checkArchive(c *gc.C, filename string) {
	archive, err := os.Open(filename)
	c.Assert(err, gc.IsNil)
	defer archive.Close()

	data, err := ioutil.ReadAll(archive)
	c.Check(string(data), gc.Equals, s.data)
}

func (s *downloadSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "download", "--help")
	c.Assert(err, gc.IsNil)

	info := s.subcommand.Info()
	expected := `(?sm)usage: juju backups download \[options] ` + info.Args + `$.*`
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
}

func (s *downloadSuite) TestOkay(c *gc.C) {
	s.setSuccess()
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, gc.IsNil)

	filename := "juju-backup-" + s.metaresult.ID + ".tar.gz"
	s.checkStd(c, ctx, filename+"\n", "")
	s.checkArchive(c, filename)
}

func (s *downloadSuite) TestFilename(c *gc.C) {
	s.setSuccess()
	s.subcommand.Filename = "backup.tar.gz"
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, gc.IsNil)

	filename := "backup.tar.gz"
	s.checkStd(c, ctx, filename+"\n", "")
	s.checkArchive(c, filename)
}

func (s *downloadSuite) TestError(c *gc.C) {
	s.setFailure("failed!")
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
