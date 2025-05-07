// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/backups"
	bt "github.com/juju/juju/core/backups/testing"
)

type archiveDataSuiteV0 struct {
	testing.IsolationSuite
	baseArchiveDataSuite
}

var _ = tc.Suite(&archiveDataSuiteV0{})
var _ = tc.Suite(&archiveDataSuite{})

func (s *archiveDataSuiteV0) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.baseArchiveDataSuite.setupMetadata(c, testMetadataV1)
}

func newArchiveFile(c *tc.C, meta *backups.Metadata) io.Reader {
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
	var archiveFile io.Reader
	var err error
	if meta != nil && meta.FormatVersion == 0 {
		archiveFile, err = bt.NewArchiveV0(meta, files, dump)
	} else {
		archiveFile, err = bt.NewArchive(meta, files, dump)
	}
	c.Assert(err, tc.ErrorIsNil)
	return archiveFile
}

func (s *archiveDataSuiteV0) TestNewArchiveData(c *tc.C) {
	ad := backups.NewArchiveData([]byte("<uncompressed>"))
	data := ad.NewBuffer().String()

	c.Check(ad.ContentDir, tc.Equals, "juju-backup")
	c.Check(data, tc.Equals, "<uncompressed>")
}

func (s *archiveDataSuiteV0) TestNewArchiveDataReader(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	data := ad.NewBuffer().Bytes()

	c.Check(ad.ContentDir, tc.Equals, "juju-backup")
	c.Check(data, tc.DeepEquals, s.data)
}

func (s *archiveDataSuiteV0) TestNewBuffer(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	buf := ad.NewBuffer()

	c.Check(buf.Bytes(), tc.DeepEquals, s.data)
}

func (s *archiveDataSuiteV0) TestNewBufferMultiple(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	buf1 := ad.NewBuffer()
	buf2 := ad.NewBuffer()

	c.Check(buf2, tc.Not(tc.Equals), buf1)
	c.Check(buf2.Bytes(), tc.DeepEquals, buf1.Bytes())
}

func (s *archiveDataSuiteV0) TestMetadata(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	meta, err := ad.Metadata()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta, tc.DeepEquals, s.meta)
}

func (s *archiveDataSuiteV0) TestVersionFound(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	version, err := ad.Version()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(version, tc.DeepEquals, &s.meta.Origin.Version)
}

type baseArchiveDataSuite struct {
	archiveFile *bytes.Buffer
	data        []byte
	meta        *backups.Metadata
}

const (
	testMetadataV1 = `{` +
		`"ID":"20140909-115934.asdf-zxcv-qwe",` +
		`"FormatVersion":1,` +
		`"Checksum":"123af2cef",` +
		`"ChecksumFormat":"SHA-1, base64 encoded",` +
		`"Size":10,` +
		`"Stored":"0001-01-01T00:00:00Z",` +
		`"Started":"2014-09-09T11:59:34Z",` +
		`"Finished":"2014-09-09T12:00:34Z",` +
		`"Notes":"",` +
		`"ModelUUID":"asdf-zxcv-qwe",` +
		`"Machine":"0",` +
		`"Hostname":"myhost",` +
		`"Version":"1.21-alpha3",` +
		`"ControllerUUID":"controller-uuid",` +
		`"HANodes":3,` +
		`"ControllerMachineID":"10",` +
		`"ControllerMachineInstanceID":"inst-10101010"` +
		`}` + "\n"
)

func (s *baseArchiveDataSuite) setupMetadata(c *tc.C, metadata string) {
	meta, err := backups.NewMetadataJSONReader(bytes.NewBufferString(metadata))
	c.Assert(err, tc.ErrorIsNil)

	archiveFile := newArchiveFile(c, meta)
	compressed, err := io.ReadAll(archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	gzr, err := gzip.NewReader(bytes.NewBuffer(compressed))
	c.Assert(err, tc.ErrorIsNil)
	data, err := io.ReadAll(gzr)
	c.Assert(err, tc.ErrorIsNil)

	s.archiveFile = bytes.NewBuffer(compressed)
	s.data = data
	s.meta = meta
}

type archiveDataSuite struct {
	archiveDataSuiteV0
}

func (s *archiveDataSuite) SetUpTest(c *tc.C) {
	s.archiveDataSuiteV0.SetUpTest(c)
	s.setupMetadata(c, testMetadataV1)
}
