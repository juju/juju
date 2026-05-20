// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"
	"os"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/testhelpers"
)

type workspaceSuite struct {
	testhelpers.IsolationSuite
	baseArchiveDataSuite
}

func TestWorkspaceSuite(t *testing.T) {
	tc.Run(t, &workspaceSuite{})
}

func (s *workspaceSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.baseArchiveDataSuite.setupMetadata(c, testMetadataV2)
}

func (s *workspaceSuite) TestNewArchiveWorkspaceReader(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	defer ws.Close()

	c.Check(ws.RootDir, tc.Not(tc.Equals), "")
}

func (s *workspaceSuite) TestClose(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	err = ws.Close()
	c.Assert(err, tc.ErrorIsNil)

	_, err = os.Stat(ws.RootDir)
	c.Check(err, tc.Satisfies, os.IsNotExist)
}

func (s *workspaceSuite) TestUnpackFilesBundle(c *tc.C) {
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

func (s *workspaceSuite) TestOpenBundledFile(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	defer ws.Close()

	file, err := ws.OpenBundledFile("var/lib/juju/system-identity")
	c.Assert(err, tc.ErrorIsNil)

	data, err := io.ReadAll(file)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "<an ssh key goes here>")
}

func (s *workspaceSuite) TestMetadata(c *tc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	defer ws.Close()

	meta, err := ws.Metadata()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(meta, tc.DeepEquals, s.meta)
}
