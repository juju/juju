// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"
	"os"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/backups"
)

type workspaceSuiteV0 struct {
	testing.IsolationSuite
	baseArchiveDataSuite
}

var _ = tc.Suite(&workspaceSuiteV0{})
var _ = tc.Suite(&workspaceSuiteV1{})

func (s *workspaceSuiteV0) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.baseArchiveDataSuite.setupMetadata(c, testMetadataV1)
}

func (s *workspaceSuiteV0) TestNewArchiveWorkspaceReader(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	defer ws.Close()

	c.Check(ws.RootDir, tc.Not(tc.Equals), "")
}

func (s *workspaceSuiteV0) TestClose(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	err = ws.Close()
	c.Assert(err, tc.ErrorIsNil)

	_, err = os.Stat(ws.RootDir)
	c.Check(err, tc.Satisfies, os.IsNotExist)
}

func (s *workspaceSuiteV0) TestUnpackFilesBundle(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	defer ws.Close()

	targetDir := c.MkDir()
	err = ws.UnpackFilesBundle(targetDir)
	c.Assert(err, tc.ErrorIsNil)

	_, err = os.Stat(targetDir + "/var/lib/juju/tools/1.21-alpha2.1-trusty-amd64/jujud")
	c.Assert(err, tc.ErrorIsNil)
	_, err = os.Stat(targetDir + "/var/lib/juju/system-identity")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *workspaceSuiteV0) TestOpenBundledFile(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	defer ws.Close()

	file, err := ws.OpenBundledFile("var/lib/juju/system-identity")
	c.Assert(err, tc.ErrorIsNil)

	data, err := io.ReadAll(file)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "<an ssh key goes here>")
}

func (s *workspaceSuiteV0) TestMetadata(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	defer ws.Close()

	meta, err := ws.Metadata()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(meta, tc.DeepEquals, s.meta)
}

type workspaceSuiteV1 struct {
	testing.IsolationSuite
	baseArchiveDataSuite
}

func (s *workspaceSuiteV1) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.baseArchiveDataSuite.setupMetadata(c, testMetadataV1)
}
