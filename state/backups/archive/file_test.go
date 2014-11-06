// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package archive_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups/archive"
	"github.com/juju/juju/state/backups/metadata"
	bt "github.com/juju/juju/state/backups/testing"
)

type fileSuite struct {
	testing.IsolationSuite
	archiveFile *bytes.Buffer
	meta        *metadata.Metadata
}

var _ = gc.Suite(&fileSuite{})

func (s *fileSuite) SetUpTest(c *gc.C) {
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

func (s *fileSuite) dump(c *gc.C) string {
	filename := filepath.Join(c.MkDir(), "juju-backup.tgz")
	file, err := os.Create(filename)
	c.Assert(err, gc.IsNil)
	defer file.Close()

	_, err = io.Copy(file, s.archiveFile)
	c.Assert(err, gc.IsNil)

	return filename
}

func (s *fileSuite) TestNewArchiveData(c *gc.C) {
	ad, err := archive.NewArchiveData(s.archiveFile, "")
	c.Assert(err, gc.IsNil)

	c.Check(ad.Filename, gc.Equals, "")
}

func (s *fileSuite) TestNewArchiveFile(c *gc.C) {
	filename := s.dump(c)
	ad := archive.NewArchiveFile(filename)

	c.Check(ad.Filename, gc.Equals, filename)
}

func (s *fileSuite) TestMetadata(c *gc.C) {
	ad, err := archive.NewArchiveData(s.archiveFile, "")
	c.Assert(err, gc.IsNil)

	meta, err := ad.Metadata()
	c.Assert(err, gc.IsNil)

	c.Check(meta, jc.DeepEquals, s.meta)
}

func (s *fileSuite) TestMetadataUncached(c *gc.C) {
	filename := s.dump(c)
	ad := archive.NewArchiveFile(filename)

	meta, err := ad.Metadata()
	c.Assert(err, gc.IsNil)

	c.Check(meta, jc.DeepEquals, s.meta)
}

func (s *fileSuite) TestVersion(c *gc.C) {
	ad, err := archive.NewArchiveData(s.archiveFile, "")
	c.Assert(err, gc.IsNil)

	meta, err := ad.Metadata()
	c.Assert(err, gc.IsNil)

	c.Check(meta, jc.DeepEquals, s.meta)
}
