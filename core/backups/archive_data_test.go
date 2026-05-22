// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	bt "github.com/juju/juju/core/backups/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type archiveDataSuite struct {
	testhelpers.IsolationSuite
	baseArchiveDataSuite
}

func TestArchiveDataSuite(t *testing.T) {
	tc.Run(t, &archiveDataSuite{})
}

func (s *archiveDataSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.setupMetadata(c, testMetadataV2)
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
		archiveFile, err = bt.NewArchive(meta, files, dump)
	} else {
		archiveFile, err = bt.NewArchive(meta, files, dump)
	}
	c.Assert(err, tc.ErrorIsNil)
	return archiveFile
}

func (s *archiveDataSuite) TestNewArchiveData(c *tc.C) {
	ad := backups.NewArchiveData([]byte("<uncompressed>"))
	data := ad.NewBuffer().String()

	c.Check(ad.ContentDir, tc.Equals, "juju-backup")
	c.Check(data, tc.Equals, "<uncompressed>")
}

func (s *archiveDataSuite) TestNewArchiveDataReader(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	data := ad.NewBuffer().Bytes()

	c.Check(ad.ContentDir, tc.Equals, "juju-backup")
	c.Check(data, tc.DeepEquals, s.data)
}

func (s *archiveDataSuite) TestNewBuffer(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)
	buf := ad.NewBuffer()

	c.Check(buf.Bytes(), tc.DeepEquals, s.data)
}

func (s *archiveDataSuite) TestNewBufferMultiple(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	buf1 := ad.NewBuffer()
	buf2 := ad.NewBuffer()

	c.Check(buf2, tc.Not(tc.Equals), buf1)
	c.Check(buf2.Bytes(), tc.DeepEquals, buf1.Bytes())
}

func (s *archiveDataSuite) TestMetadata(c *tc.C) {
	ad, err := backups.NewArchiveDataReader(s.archiveFile)
	c.Assert(err, tc.ErrorIsNil)

	meta, err := ad.Metadata()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta, tc.DeepEquals, s.meta)
}

func (s *archiveDataSuite) TestVersionFound(c *tc.C) {
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
	testMetadataV2 = `{` +
		`"ID":"20140909-115934.asdf-zxcv-qwe",` +
		`"FormatVersion":2,` +
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
