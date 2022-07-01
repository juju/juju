// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/state/backups"
	bt "github.com/juju/juju/v3/state/backups/testing"
)

type archiveDataSuiteV0 struct {
	testing.IsolationSuite
	baseArchiveDataSuite
}

var _ = gc.Suite(&archiveDataSuiteV0{})
var _ = gc.Suite(&archiveDataSuite{})

func (s *archiveDataSuiteV0) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.baseArchiveDataSuite.setupMetadata(c, testMetadataV0)
}

func newArchiveFile(c *gc.C, meta *backups.Metadata) io.Reader {
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
	c.Assert(err, jc.ErrorIsNil)
	return archiveFile
}

func (s *archiveDataSuiteV0) TestNewArchiveData(c *gc.C) {
	ad := backups.NewArchiveData([]byte("<uncompressed>"))
	data := ad.NewBuffer().String()

	c.Check(ad.ContentDir, gc.Equals, "juju-backup")
	c.Check(data, gc.Equals, "<uncompressed>")
}

func (s *archiveDataSuiteV0) TestNewArchiveDataReader(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	data := ad.NewBuffer().Bytes()

	c.Check(ad.ContentDir, gc.Equals, "juju-backup")
	c.Check(data, jc.DeepEquals, s.data)
}

func (s *archiveDataSuiteV0) TestNewBuffer(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	buf := ad.NewBuffer()

	c.Check(buf.Bytes(), jc.DeepEquals, s.data)
}

func (s *archiveDataSuiteV0) TestNewBufferMultiple(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)

	buf1 := ad.NewBuffer()
	buf2 := ad.NewBuffer()

	c.Check(buf2, gc.Not(gc.Equals), buf1)
	c.Check(buf2.Bytes(), jc.DeepEquals, buf1.Bytes())
}

func (s *archiveDataSuiteV0) TestMetadata(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)

	meta, err := ad.Metadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(meta, jc.DeepEquals, s.meta)
}

func (s *archiveDataSuiteV0) TestVersionFound(c *gc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, jc.ErrorIsNil)

	version, err := ad.Version()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(version, jc.DeepEquals, &s.meta.Origin.Version)
}

func (s *archiveDataSuiteV0) TestVersionNotFound(c *gc.C) {
	archiveFile := newArchiveFile(c, nil)
	ad, err := backups.NewArchiveDataReader(archiveFile)
	c.Assert(err, jc.ErrorIsNil)

	version, err := ad.Version()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(version.String(), jc.DeepEquals, "1.20.0")
}

type baseArchiveDataSuite struct {
	archiveFile *bytes.Buffer
	data        []byte
	meta        *backups.Metadata
}

const (
	testMetadataV0 = `{` +
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
		`}` + "\n"

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

func (s *baseArchiveDataSuite) setupMetadata(c *gc.C, metadata string) {
	meta, err := backups.NewMetadataJSONReader(bytes.NewBufferString(metadata))
	c.Assert(err, jc.ErrorIsNil)

	archiveFile := newArchiveFile(c, meta)
	compressed, err := ioutil.ReadAll(archiveFile)
	c.Assert(err, jc.ErrorIsNil)
	gzr, err := gzip.NewReader(bytes.NewBuffer(compressed))
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadAll(gzr)
	c.Assert(err, jc.ErrorIsNil)

	s.archiveFile = bytes.NewBuffer(compressed)
	s.data = data
	s.meta = meta
}

type archiveDataSuite struct {
	archiveDataSuiteV0
}

func (s *archiveDataSuite) SetUpTest(c *gc.C) {
	s.archiveDataSuiteV0.SetUpTest(c)
	s.setupMetadata(c, testMetadataV1)
}
