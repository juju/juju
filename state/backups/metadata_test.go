// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type metadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&metadataSuite{}) // Register the suite.

func (s *metadataSuite) TestAsJSONBuffer(c *gc.C) {
	meta := backups.NewMetadata()
	meta.Origin = backups.Origin{
		Environment: "asdf-zxcv-qwe",
		Machine:     "0",
		Hostname:    "myhost",
		Version:     version.MustParse("1.21-alpha3"),
	}
	meta.Started = time.Date(2014, time.Month(9), 9, 11, 59, 34, 0, time.UTC)

	meta.SetID("20140909-115934.asdf-zxcv-qwe")
	err := meta.MarkComplete(10, "123af2cef")
	c.Assert(err, jc.ErrorIsNil)

	finished := meta.Started.Add(time.Minute)
	meta.Finished = &finished

	buf, err := meta.AsJSONBuffer()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(buf.(*bytes.Buffer).String(), gc.Equals, `{`+
		`"ID":"20140909-115934.asdf-zxcv-qwe",`+
		`"Checksum":"123af2cef",`+
		`"ChecksumFormat":"SHA-1, base64 encoded",`+
		`"Size":10,`+
		`"Stored":"0001-01-01T00:00:00Z",`+
		`"Started":"2014-09-09T11:59:34Z",`+
		`"Finished":"2014-09-09T12:00:34Z",`+
		`"Notes":"",`+
		`"Environment":"asdf-zxcv-qwe",`+
		`"Machine":"0",`+
		`"Hostname":"myhost",`+
		`"Version":"1.21-alpha3"`+
		`}`+"\n")
}

func (s *metadataSuite) TestNewMetadataJSONReader(c *gc.C) {
	file := bytes.NewBufferString(`{` +
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
		`}` + "\n")
	meta, err := backups.NewMetadataJSONReader(file)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(meta.ID(), gc.Equals, "20140909-115934.asdf-zxcv-qwe")
	c.Check(meta.Checksum(), gc.Equals, "123af2cef")
	c.Check(meta.ChecksumFormat(), gc.Equals, "SHA-1, base64 encoded")
	c.Check(meta.Size(), gc.Equals, int64(10))
	c.Check(meta.Stored(), gc.IsNil)
	c.Check(meta.Started.Unix(), gc.Equals, int64(1410263974))
	c.Check(meta.Finished.Unix(), gc.Equals, int64(1410264034))
	c.Check(meta.Notes, gc.Equals, "")
	c.Check(meta.Origin.Environment, gc.Equals, "asdf-zxcv-qwe")
	c.Check(meta.Origin.Machine, gc.Equals, "0")
	c.Check(meta.Origin.Hostname, gc.Equals, "myhost")
	c.Check(meta.Origin.Version.String(), gc.Equals, "1.21-alpha3")
}

func (s *metadataSuite) TestBuildMetadata(c *gc.C) {
	archive, err := os.Create(filepath.Join(c.MkDir(), "juju-backup.tgz"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = archive.Write([]byte("<compressed data>"))
	c.Assert(err, jc.ErrorIsNil)

	fi, err := archive.Stat()
	c.Assert(err, jc.ErrorIsNil)
	finished := backups.FileTimestamp(fi).Unix()

	meta, err := backups.BuildMetadata(archive)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(meta.ID(), gc.Equals, "")
	c.Check(meta.Checksum(), gc.Equals, "2jmj7l5rSw0yVb/vlWAYkK/YBwk=")
	c.Check(meta.ChecksumFormat(), gc.Equals, "SHA-1, base64 encoded")
	c.Check(meta.Size(), gc.Equals, int64(17))
	c.Check(meta.Stored(), gc.IsNil)
	c.Check(meta.Started.Unix(), gc.Equals, int64(time.Time{}.Unix()))
	c.Check(meta.Finished.Unix(), gc.Equals, finished)
	c.Check(meta.Notes, gc.Equals, "")
	c.Check(meta.Origin.Environment, gc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Machine, gc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Hostname, gc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Version.String(), gc.Equals, backups.UnknownVersion.String())
}
