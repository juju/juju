// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package archive_test

import (
	"bytes"
	"io/ioutil"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups/archive"
	"github.com/juju/juju/state/backups/metadata"
	bt "github.com/juju/juju/state/backups/testing"
)

type workspaceSuite struct {
	testing.IsolationSuite
	archiveFile *bytes.Buffer
	meta        *metadata.Metadata
}

var _ = gc.Suite(&workspaceSuite{}) // Register the suite.

func (s *workspaceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	meta, err := metadata.NewFromJSONBuffer(bytes.NewBufferString(`{` +
		`"ID":"20140909-115934.asdf-zxcv-qwe",` +
		`"Checksum":"123af2cef",` +
		`"ChecksumFormat":"SHA-1, base64 encoded",` +
		`"Size":10,` +
		`"Stored":"0001-01-01T00:00:00Z",` +
		`"Started":"2014-09-09T11:59:34Z",` +
		`"Finished":"2014-09-09T12:00:34Z",` +
		`"Notes":"",` +
		`"Environment":"asdf-zxcv-qwe",` +
		`"Machine":"0",` +
		`"Hostname":"myhost",` +
		`"Version":"1.21-alpha3"` +
		`}` + "\n"))
	c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)

	s.archiveFile = archiveFile
	s.meta = meta
}

func (s *workspaceSuite) TestNewWorkspace(c *gc.C) {
	ws, err := archive.NewWorkspace(s.archiveFile)
	c.Assert(err, gc.IsNil)
	defer ws.Close()

	root := ws.UnpackedRootDir + "/"
	c.Check(ws.Filename, gc.Equals, "")
	c.Check(ws.ContentDir(), gc.Equals, root+"juju-backup")
	c.Check(ws.FilesBundle(), gc.Equals, root+"juju-backup/root.tar")
	c.Check(ws.DBDumpDir(), gc.Equals, root+"juju-backup/dump")
	c.Check(ws.MetadataFile(), gc.Equals, root+"juju-backup/metadata.json")
}

func (s *workspaceSuite) TestClose(c *gc.C) {
	ws, err := archive.NewWorkspace(s.archiveFile)
	c.Assert(err, gc.IsNil)

	err = ws.Close()
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(ws.UnpackedRootDir)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *workspaceSuite) TestUnpackFiles(c *gc.C) {
	ws, err := archive.NewWorkspace(s.archiveFile)
	c.Assert(err, gc.IsNil)
	defer ws.Close()

	targetDir := c.MkDir()
	err = ws.UnpackFiles(targetDir)
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(targetDir + "/var/lib/juju/tools/1.21-alpha2.1-trusty-amd64/jujud")
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(targetDir + "/var/lib/juju/system-identity")
	c.Assert(err, gc.IsNil)
}

func (s *workspaceSuite) TestOpenFile(c *gc.C) {
	ws, err := archive.NewWorkspace(s.archiveFile)
	c.Assert(err, gc.IsNil)
	defer ws.Close()

	file, err := ws.OpenFile("var/lib/juju/system-identity")
	c.Assert(err, gc.IsNil)
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, "<an ssh key goes here>")
}

func (s *workspaceSuite) TestMetadata(c *gc.C) {
	ws, err := archive.NewWorkspace(s.archiveFile)
	c.Assert(err, gc.IsNil)
	defer ws.Close()

	meta, err := ws.Metadata()
	c.Assert(err, gc.IsNil)

	c.Check(meta, jc.DeepEquals, s.meta)
}
