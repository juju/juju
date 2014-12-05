// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	bt "github.com/juju/juju/state/backups/testing"
)

type workspaceSuite struct {
	testing.IsolationSuite
	archiveFile *bytes.Buffer
	meta        *backups.Metadata
}

var _ = gc.Suite(&workspaceSuite{})

func (s *workspaceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	meta, err := backups.NewMetadataJSONReader(bytes.NewBufferString(`{` +
		`"ID":"20140909-115934.asdf-zxcv-qwe",` +
		`"Checksum":"123af2cef",` +
		`"ChecksumFormat":"SHA-1, base64 encoded",` +
		`"Size":10,` +
		`"Stored":"0001-01-01T00:00:00Z",` +
		`"Started":"2014-09-09T11:59:34Z",` +
		`"Finished":"2014-09-09T12:00:34Z",` +
		`"Notes":"",` +
		`"Environment":"9f484882-2f18-4fd2-967d-db9663db7bea",` +
		`"Machine":"0",` +
		`"Hostname":"myhost",` +
		`"Version":"1.21-alpha3"` +
		`}` + "\n"))
	c.Assert(err, jc.ErrorIsNil)

	files := []bt.File{
		{
			Name:    "var/lib/juju/tools/1.21-alpha2.1-trusty-amd64/jujud",
			Content: "<some binary data goes here>",
		},
		{
			Name:    "var/lib/juju/system-identity",
			Content: "<an ssh key goes here>",
		},
	}
	dump := []bt.File{
		{
			Name:  "juju",
			IsDir: true,
		},
		{
			Name:    "juju/machines.bson",
			Content: "<BSON data goes here>",
		},
		{
			Name:    "oplog.bson",
			Content: "<BSON data goes here>",
		},
	}
	archiveFile, err := bt.NewArchive(meta, files, dump)
	c.Assert(err, jc.ErrorIsNil)

	s.archiveFile = archiveFile
	s.meta = meta
}

func (s *workspaceSuite) TestNewArchiveWorkspaceReader(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	defer ws.Close()

	c.Check(ws.RootDir, gc.Not(gc.Equals), "")
}

func (s *workspaceSuite) TestClose(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)

	err = ws.Close()
	c.Assert(err, jc.ErrorIsNil)

	_, err = os.Stat(ws.RootDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *workspaceSuite) TestUnpackFilesBundle(c *gc.C) {
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

func (s *workspaceSuite) TestOpenBundledFile(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	defer ws.Close()

	file, err := ws.OpenBundledFile("var/lib/juju/system-identity")
	c.Assert(err, jc.ErrorIsNil)

	data, err := ioutil.ReadAll(file)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, "<an ssh key goes here>")
}

func (s *workspaceSuite) TestMetadata(c *gc.C) {
	ws, err := backups.NewArchiveWorkspaceReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	defer ws.Close()

	meta, err := ws.Metadata()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(meta, jc.DeepEquals, s.meta)
}
