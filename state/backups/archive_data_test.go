// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	bt "github.com/juju/juju/state/backups/testing"
)

type archiveDataSuite struct {
	testing.IsolationSuite
	archiveFile *bytes.Buffer
	data        []byte
	meta        *backups.Metadata
}

var _ = gc.Suite(&archiveDataSuite{})

func (s *archiveDataSuite) SetUpTest(c *gc.C) {
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
	compressed, err := ioutil.ReadAll(archiveFile)
	c.Assert(err, gc.IsNil)
	gzr, err := gzip.NewReader(bytes.NewBuffer(compressed))
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadAll(gzr)
	c.Assert(err, gc.IsNil)

	s.archiveFile = bytes.NewBuffer(compressed)
	s.data = data
	s.meta = meta
}

func (s *archiveDataSuite) TestNewArchiveData(c *gc.C) {
	ad := backups.NewArchiveData([]byte("<uncompressed>"))
	data := ad.NewBuffer().String()

	c.Check(ad.RootDir, gc.Equals, "")
	c.Check(data, gc.Equals, "<uncompressed>")
}

func (s *archiveDataSuite) TestNewArchiveDataReader(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, gc.IsNil)
	data := ad.NewBuffer().Bytes()

	c.Check(ad.RootDir, gc.Equals, "")
	c.Check(data, jc.DeepEquals, s.data)
}

func (s *archiveDataSuite) TestNewBuffer(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, gc.IsNil)
	buf := ad.NewBuffer()

	c.Check(buf.Bytes(), jc.DeepEquals, s.data)
}

func (s *archiveDataSuite) TestNewBufferMultiple(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, gc.IsNil)

	buf1 := ad.NewBuffer()
	buf2 := ad.NewBuffer()

	c.Check(buf2, gc.Not(gc.Equals), buf1)
	c.Check(buf2.Bytes(), jc.DeepEquals, buf1.Bytes())
}

func (s *archiveDataSuite) TestMetadata(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, gc.IsNil)

	meta, err := ad.Metadata()
	c.Assert(err, gc.IsNil)

	c.Check(meta, jc.DeepEquals, s.meta)
}

func (s *archiveDataSuite) TestVersion(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, gc.IsNil)

	version, err := ad.Version()
	c.Assert(err, gc.IsNil)

	c.Check(version, jc.DeepEquals, &s.meta.Origin.Version)
}
