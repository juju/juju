// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/testing"
)

type uploadSuite struct {
	BaseBackupsSuite
	subcommand *backups.UploadCommand
	filename   string
}

var _ = gc.Suite(&uploadSuite{})

func (s *uploadSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)

	s.subcommand = &backups.UploadCommand{}
	s.filename = "juju-backup-20140912-130755.abcd-spam-deadbeef-eggs.tar.gz"
	s.subcommand.Filename = s.filename
}

func (s *uploadSuite) TearDownTest(c *gc.C) {
	if err := os.Remove(s.filename); err != nil {
		if !os.IsNotExist(err) {
			c.Check(err, jc.ErrorIsNil)
		}
	}

	s.BaseBackupsSuite.TearDownTest(c)
}

func (s *uploadSuite) createArchive(c *gc.C) {
	archive, err := os.Create(s.filename)
	c.Assert(err, jc.ErrorIsNil)
	defer archive.Close()

	compressed := gzip.NewWriter(archive)
	defer compressed.Close()

	tarball := tar.NewWriter(compressed)
	defer tarball.Close()

	var files = []struct{ Name, Body string }{
		{"root.tar", "<state config files>"},
		{"dump/oplog.bson", "<something here>"},
	}
	for _, file := range files {
		hdr := &tar.Header{
			Name: file.Name,
			Size: int64(len(file.Body)),
		}
		err := tarball.WriteHeader(hdr)
		c.Assert(err, jc.ErrorIsNil)
		_, err = tarball.Write([]byte(file.Body))
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *uploadSuite) TestHelp(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.command, "upload", "--help")
	c.Assert(err, jc.ErrorIsNil)

	info := s.subcommand.Info()
	expected := "(?sm)usage: juju backups upload [options] " + info.Args + "$.*"
	expected = strings.Replace(expected, "[", `\[`, -1)
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^purpose: " + info.Purpose + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
	expected = "(?sm).*^" + info.Doc + "$.*"
	c.Check(testing.Stdout(ctx), gc.Matches, expected)
}

func (s *uploadSuite) TestOkay(c *gc.C) {
	s.createArchive(c)
	s.setSuccess()
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)
	c.Check(err, jc.ErrorIsNil)

	out := MetaResultString
	s.checkStd(c, ctx, out, "")
}

func (s *uploadSuite) TestFileMissing(c *gc.C) {
	s.setSuccess()
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(os.IsNotExist(errors.Cause(err)), jc.IsTrue)
}

func (s *uploadSuite) TestError(c *gc.C) {
	s.createArchive(c)
	s.setFailure("failed!")
	ctx := cmdtesting.Context(c)
	err := s.subcommand.Run(ctx)

	c.Check(errors.Cause(err), gc.ErrorMatches, "failed!")
}
