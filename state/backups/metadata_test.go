// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"os"
	"path/filepath"
	"time" // Only used for time types and funcs, not Now().

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

type metadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&metadataSuite{}) // Register the suite.

func (s *metadataSuite) TestAsJSONBuffer(c *gc.C) {
	meta := s.createTestMetadata(c)
	meta.FormatVersion = 0
	s.assertMetadata(c, meta, `{`+
		`"ID":"20140909-115934.asdf-zxcv-qwe",`+
		`"FormatVersion":0,`+
		`"Checksum":"123af2cef",`+
		`"ChecksumFormat":"SHA-1, base64 encoded",`+
		`"Size":10,`+
		`"Stored":"0001-01-01T00:00:00Z",`+
		`"Started":"2014-09-09T11:59:34Z",`+
		`"Finished":"2014-09-09T12:00:34Z",`+
		`"Notes":"",`+
		`"ModelUUID":"asdf-zxcv-qwe",`+
		`"Machine":"0",`+
		`"Hostname":"myhost",`+
		`"Version":"1.21-alpha3",`+
		`"Series":"trusty",`+
		`"ControllerUUID":"",`+
		`"HANodes":0,`+
		`"ControllerMachineID":"",`+
		`"ControllerMachineInstanceID":"",`+
		`"CACert":"ca-cert",`+
		`"CAPrivateKey":"ca-private-key"`+
		`}`+"\n")
}

func (s *metadataSuite) createTestMetadata(c *gc.C) *backups.Metadata {
	meta := backups.NewMetadata()
	meta.Origin = backups.Origin{
		Model:    "asdf-zxcv-qwe",
		Machine:  "0",
		Hostname: "myhost",
		Version:  version.MustParse("1.21-alpha3"),
		Series:   "trusty",
	}
	meta.Started = time.Date(2014, time.Month(9), 9, 11, 59, 34, 0, time.UTC)

	meta.SetID("20140909-115934.asdf-zxcv-qwe")

	meta.CACert = "ca-cert"
	meta.CAPrivateKey = "ca-private-key"
	return meta
}

func (s *metadataSuite) assertMetadata(c *gc.C, meta *backups.Metadata, expected string) {
	err := meta.MarkComplete(10, "123af2cef")
	c.Assert(err, jc.ErrorIsNil)
	finished := meta.Started.Add(time.Minute)
	meta.Finished = &finished

	buf, err := meta.AsJSONBuffer()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(buf.(*bytes.Buffer).String(), jc.DeepEquals, expected)
}

func (s *metadataSuite) TestAsJSONBufferV1NonHA(c *gc.C) {
	meta := s.createTestMetadata(c)
	meta.FormatVersion = 1
	meta.Controller = backups.ControllerMetadata{
		UUID:              "controller-uuid",
		MachineInstanceID: "inst-10101010",
		MachineID:         "10",
	}
	s.assertMetadata(c, meta, `{`+
		`"ID":"20140909-115934.asdf-zxcv-qwe",`+
		`"FormatVersion":1,`+
		`"Checksum":"123af2cef",`+
		`"ChecksumFormat":"SHA-1, base64 encoded",`+
		`"Size":10,`+
		`"Stored":"0001-01-01T00:00:00Z",`+
		`"Started":"2014-09-09T11:59:34Z",`+
		`"Finished":"2014-09-09T12:00:34Z",`+
		`"Notes":"",`+
		`"ModelUUID":"asdf-zxcv-qwe",`+
		`"Machine":"0",`+
		`"Hostname":"myhost",`+
		`"Version":"1.21-alpha3",`+
		`"Series":"trusty",`+
		`"ControllerUUID":"controller-uuid",`+
		`"HANodes":0,`+
		`"ControllerMachineID":"10",`+
		`"ControllerMachineInstanceID":"inst-10101010",`+
		`"CACert":"ca-cert",`+
		`"CAPrivateKey":"ca-private-key"`+
		`}`+"\n")
}

func (s *metadataSuite) TestAsJSONBufferV1HA(c *gc.C) {
	meta := s.createTestMetadata(c)
	meta.FormatVersion = 1
	meta.Controller = backups.ControllerMetadata{
		UUID:              "controller-uuid",
		MachineInstanceID: "inst-10101010",
		MachineID:         "10",
		HANodes:           3,
	}

	s.assertMetadata(c, meta, `{`+
		`"ID":"20140909-115934.asdf-zxcv-qwe",`+
		`"FormatVersion":1,`+
		`"Checksum":"123af2cef",`+
		`"ChecksumFormat":"SHA-1, base64 encoded",`+
		`"Size":10,`+
		`"Stored":"0001-01-01T00:00:00Z",`+
		`"Started":"2014-09-09T11:59:34Z",`+
		`"Finished":"2014-09-09T12:00:34Z",`+
		`"Notes":"",`+
		`"ModelUUID":"asdf-zxcv-qwe",`+
		`"Machine":"0",`+
		`"Hostname":"myhost",`+
		`"Version":"1.21-alpha3",`+
		`"Series":"trusty",`+
		`"ControllerUUID":"controller-uuid",`+
		`"HANodes":3,`+
		`"ControllerMachineID":"10",`+
		`"ControllerMachineInstanceID":"inst-10101010",`+
		`"CACert":"ca-cert",`+
		`"CAPrivateKey":"ca-private-key"`+
		`}`+"\n")
}

func (s *metadataSuite) TestNewMetadataJSONReaderV0(c *gc.C) {
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
	c.Check(meta.Origin.Model, gc.Equals, "asdf-zxcv-qwe")
	c.Check(meta.Origin.Machine, gc.Equals, "0")
	c.Check(meta.Origin.Hostname, gc.Equals, "myhost")
	c.Check(meta.Origin.Version.String(), gc.Equals, "1.21-alpha3")
	c.Check(meta.FormatVersion, gc.Equals, int64(0))
	c.Check(meta.Controller.UUID, gc.Equals, "")
	c.Check(meta.Controller.HANodes, gc.Equals, int64(0))
	c.Check(meta.Controller.MachineInstanceID, gc.Equals, "")
	c.Check(meta.Controller.MachineID, gc.Equals, "")
}

func (s *metadataSuite) TestNewMetadataJSONReaderV1(c *gc.C) {
	file := bytes.NewBufferString(`{` +
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
	c.Check(meta.Origin.Model, gc.Equals, "asdf-zxcv-qwe")
	c.Check(meta.Origin.Machine, gc.Equals, "0")
	c.Check(meta.Origin.Hostname, gc.Equals, "myhost")
	c.Check(meta.Origin.Version.String(), gc.Equals, "1.21-alpha3")
	c.Check(meta.FormatVersion, gc.Equals, int64(1))
	c.Check(meta.Controller.UUID, gc.Equals, "controller-uuid")
	c.Check(meta.Controller.HANodes, gc.Equals, int64(3))
	c.Check(meta.Controller.MachineInstanceID, gc.Equals, "inst-10101010")
	c.Check(meta.Controller.MachineID, gc.Equals, "10")
}

func (s *metadataSuite) TestNewMetadataJSONReaderUnsupported(c *gc.C) {
	file := bytes.NewBufferString(`{` +
		`"ID":"20140909-115934.asdf-zxcv-qwe",` +
		`"FormatVersion":2,` +
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
		`}` + "\n")
	meta, err := backups.NewMetadataJSONReader(file)
	c.Assert(meta, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
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
	c.Check(meta.Started.Unix(), gc.Equals, testing.ZeroTime().Unix())
	c.Check(meta.Finished.Unix(), gc.Equals, finished)
	c.Check(meta.Notes, gc.Equals, "")
	c.Check(meta.Origin.Model, gc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Machine, gc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Hostname, gc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Version.String(), gc.Equals, backups.UnknownVersion.String())
	c.Check(meta.FormatVersion, gc.Equals, backups.UnknownInt64)
	c.Check(meta.Controller.UUID, gc.Equals, backups.UnknownString)
	c.Check(meta.Controller.MachineInstanceID, gc.Equals, backups.UnknownString)
	c.Check(meta.Controller.MachineID, gc.Equals, backups.UnknownString)
	c.Check(meta.Controller.HANodes, gc.Equals, backups.UnknownInt64)
}
