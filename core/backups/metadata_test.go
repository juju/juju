// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time" // Only used for time types and funcs, not Now().

	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testing"
)

type metadataSuite struct {
	testing.BaseSuite
}

func TestMetadataSuite(t *stdtesting.T) {
	tc.Run(t, &metadataSuite{})
}

// Register the suite.

func (s *metadataSuite) TestAsJSONBuffer(c *tc.C) {
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
		`"Base":"ubuntu@22.04",`+
		`"ControllerUUID":"",`+
		`"HANodes":0,`+
		`"ControllerMachineID":"",`+
		`"ControllerMachineInstanceID":""`+
		`}`+"\n")
}

func (s *metadataSuite) createTestMetadata(c *tc.C) *backups.Metadata {
	meta := backups.NewMetadata()
	meta.Origin = backups.Origin{
		Model:    "asdf-zxcv-qwe",
		Machine:  "0",
		Hostname: "myhost",
		Version:  semversion.MustParse("1.21-alpha3"),
		Base:     "ubuntu@22.04",
	}
	meta.Started = time.Date(2014, time.Month(9), 9, 11, 59, 34, 0, time.UTC)

	meta.SetID("20140909-115934.asdf-zxcv-qwe")

	return meta
}

func (s *metadataSuite) assertMetadata(c *tc.C, meta *backups.Metadata, expected string) {
	err := meta.MarkComplete(10, "123af2cef")
	c.Assert(err, tc.ErrorIsNil)
	finished := meta.Started.Add(time.Minute)
	meta.Finished = &finished

	buf, err := meta.AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(buf.(*bytes.Buffer).String(), tc.DeepEquals, expected)
}

func (s *metadataSuite) TestAsJSONBufferV1NonHA(c *tc.C) {
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
		`"Base":"ubuntu@22.04",`+
		`"ControllerUUID":"controller-uuid",`+
		`"HANodes":0,`+
		`"ControllerMachineID":"10",`+
		`"ControllerMachineInstanceID":"inst-10101010"`+
		`}`+"\n")
}

func (s *metadataSuite) TestAsJSONBufferV1HA(c *tc.C) {
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
		`"Base":"ubuntu@22.04",`+
		`"ControllerUUID":"controller-uuid",`+
		`"HANodes":3,`+
		`"ControllerMachineID":"10",`+
		`"ControllerMachineInstanceID":"inst-10101010"`+
		`}`+"\n")
}

func (s *metadataSuite) TestNewMetadataJSONReaderV1(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	c.Check(meta.ID(), tc.Equals, "20140909-115934.asdf-zxcv-qwe")
	c.Check(meta.Checksum(), tc.Equals, "123af2cef")
	c.Check(meta.ChecksumFormat(), tc.Equals, "SHA-1, base64 encoded")
	c.Check(meta.Size(), tc.Equals, int64(10))
	c.Check(meta.Stored(), tc.IsNil)
	c.Check(meta.Started.Unix(), tc.Equals, int64(1410263974))
	c.Check(meta.Finished.Unix(), tc.Equals, int64(1410264034))
	c.Check(meta.Notes, tc.Equals, "")
	c.Check(meta.Origin.Model, tc.Equals, "asdf-zxcv-qwe")
	c.Check(meta.Origin.Machine, tc.Equals, "0")
	c.Check(meta.Origin.Hostname, tc.Equals, "myhost")
	c.Check(meta.Origin.Version.String(), tc.Equals, "1.21-alpha3")
	c.Check(meta.FormatVersion, tc.Equals, int64(1))
	c.Check(meta.Controller.UUID, tc.Equals, "controller-uuid")
	c.Check(meta.Controller.HANodes, tc.Equals, int64(3))
	c.Check(meta.Controller.MachineInstanceID, tc.Equals, "inst-10101010")
	c.Check(meta.Controller.MachineID, tc.Equals, "10")
}

func (s *metadataSuite) TestNewMetadataJSONReaderUnsupported(c *tc.C) {
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
	c.Assert(meta, tc.IsNil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *metadataSuite) TestBuildMetadata(c *tc.C) {
	archive, err := os.Create(filepath.Join(c.MkDir(), "juju-backup.tgz"))
	c.Assert(err, tc.ErrorIsNil)
	_, err = archive.Write([]byte("<compressed data>"))
	c.Assert(err, tc.ErrorIsNil)

	fi, err := archive.Stat()
	c.Assert(err, tc.ErrorIsNil)
	finished := backups.FileTimestamp(fi).Unix()

	meta, err := backups.BuildMetadata(archive)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(meta.ID(), tc.Equals, "")
	c.Check(meta.Checksum(), tc.Equals, "2jmj7l5rSw0yVb/vlWAYkK/YBwk=")
	c.Check(meta.ChecksumFormat(), tc.Equals, "SHA-1, base64 encoded")
	c.Check(meta.Size(), tc.Equals, int64(17))
	c.Check(meta.Stored(), tc.IsNil)
	c.Check(meta.Started.Unix(), tc.Equals, testing.ZeroTime().Unix())
	c.Check(meta.Finished.Unix(), tc.Equals, finished)
	c.Check(meta.Notes, tc.Equals, "")
	c.Check(meta.Origin.Model, tc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Machine, tc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Hostname, tc.Equals, backups.UnknownString)
	c.Check(meta.Origin.Version.String(), tc.Equals, backups.UnknownVersion.String())
	c.Check(meta.FormatVersion, tc.Equals, backups.UnknownInt64)
	c.Check(meta.Controller.UUID, tc.Equals, backups.UnknownString)
	c.Check(meta.Controller.MachineInstanceID, tc.Equals, backups.UnknownString)
	c.Check(meta.Controller.MachineID, tc.Equals, backups.UnknownString)
	c.Check(meta.Controller.HANodes, tc.Equals, backups.UnknownInt64)
}
