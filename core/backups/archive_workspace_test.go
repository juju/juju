// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/backups"
)

type workspaceSuiteV0 struct {
	testing.IsolationSuite
	baseArchiveDataSuite
}

var _ = gc.Suite(&workspaceSuiteV0{})
var _ = gc.Suite(&workspaceSuiteV1{})

func (s *workspaceSuiteV0) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.baseArchiveDataSuite.setupMetadata(c, testMetadataV1)
}

func (s *workspaceSuiteV0) TestNewArchiveWorkspaceReader(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	defer ws.Close()

	c.Check(ws.RootDir, gc.Not(gc.Equals), "")
}

func (s *workspaceSuiteV0) TestClose(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)

	err = ws.Close()
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(ws.RootDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *workspaceSuiteV0) TestUnpackFilesBundle(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	defer ws.Close()

	targetDir := c.MkDir()
	err = ws.UnpackFilesBundle(targetDir)
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(targetDir + "/var/lib/juju/tools/1.21-alpha2.1-trusty-amd64/jujud")
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(targetDir + "/var/lib/juju/system-identity")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workspaceSuiteV0) TestOpenBundledFile(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	defer ws.Close()

	file, err := ws.OpenBundledFile("var/lib/juju/system-identity")
	c.Assert(err, jc.ErrorIsNil)

	data, err := io.ReadAll(file)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, "<an ssh key goes here>")
}

func (s *workspaceSuiteV0) TestMetadata(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	defer ws.Close()

	meta, err := ws.Metadata()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(meta, jc.DeepEquals, s.meta)
}

type workspaceSuiteV1 struct {
	testing.IsolationSuite
	baseArchiveDataSuite
}

func (s *workspaceSuiteV1) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.baseArchiveDataSuite.setupMetadata(c, testMetadataV1)
}
