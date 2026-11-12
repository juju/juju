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

func (s *metadataSuite) TestAsJSONBuffer(c *tc.C) {
	meta := s.createTestMetadata(c)
	meta.FormatVersion = 0
	s.assertMetadata(c, meta, `{`+
		`"ID":"20140909-115934.asdf-zxcv-qwe",`+
		`"FormatVersion":0,`+
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
		`"Instance":{"MachineID":"","MachineInstanceID":"","AgentImage":"","CharmImage":"","Architecture":"","StorageClass":"","StorageCapacityMiB":0,"ServiceType":"","ExternalName":"","ExternalIPs":null,"LoadBalancerIP":"","ServiceAnnotations":null}`+
		`}`+"\n")
}

func (s *metadataSuite) createTestMetadata(c *tc.C) *backups.Metadata {
	meta := backups.NewMetadata(time.Date(
		2014, time.Month(9), 9, 11, 59, 34, 0, time.UTC))
	meta.Origin = backups.Origin{
		Model:    "asdf-zxcv-qwe",
		Machine:  "0",
		Hostname: "myhost",
		Version:  semversion.MustParse("1.21-alpha3"),
		Base:     "ubuntu@22.04",
	}

	meta.SetID("20140909-115934.asdf-zxcv-qwe")

	return meta
}

func (s *metadataSuite) assertMetadata(c *tc.C, meta *backups.Metadata, expected string) {
	err := meta.MarkComplete(10, "123af2cef", meta.Started.Add(time.Minute))
	c.Assert(err, tc.ErrorIsNil)

	buf, err := meta.AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(buf.(*bytes.Buffer).String(), tc.DeepEquals, expected)
}

func (s *metadataSuite) TestAsJSONBufferV2NonHA(c *tc.C) {
	meta := s.createTestMetadata(c)
	meta.FormatVersion = 2
	meta.Controller = backups.ControllerMetadata{
		UUID: "controller-uuid",
	}
	meta.Instance = backups.InstanceMetadata{
		MachineID:         "10",
		MachineInstanceID: "inst-10101010",
	}
	s.assertMetadata(c, meta, `{`+
		`"ID":"20140909-115934.asdf-zxcv-qwe",`+
		`"FormatVersion":2,`+
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
		`"Instance":{"MachineID":"10","MachineInstanceID":"inst-10101010","AgentImage":"","CharmImage":"","Architecture":"","StorageClass":"","StorageCapacityMiB":0,"ServiceType":"","ExternalName":"","ExternalIPs":null,"LoadBalancerIP":"","ServiceAnnotations":null}`+
		`}`+"\n")
}

func (s *metadataSuite) TestAsJSONBufferV2HA(c *tc.C) {
	meta := s.createTestMetadata(c)
	meta.FormatVersion = 2
	meta.Controller = backups.ControllerMetadata{
		UUID:    "controller-uuid",
		HANodes: 3,
	}
	meta.Instance = backups.InstanceMetadata{
		MachineID:         "10",
		MachineInstanceID: "inst-10101010",
	}

	s.assertMetadata(c, meta, `{`+
		`"ID":"20140909-115934.asdf-zxcv-qwe",`+
		`"FormatVersion":2,`+
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
		`"Instance":{"MachineID":"10","MachineInstanceID":"inst-10101010","AgentImage":"","CharmImage":"","Architecture":"","StorageClass":"","StorageCapacityMiB":0,"ServiceType":"","ExternalName":"","ExternalIPs":null,"LoadBalancerIP":"","ServiceAnnotations":null}`+
		`}`+"\n")
}

func (s *metadataSuite) TestNewMetadataJSONReaderV2(c *tc.C) {
	file := bytes.NewBufferString(`{` +
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
		`"Instance":{"MachineID":"10","MachineInstanceID":"inst-10101010"}` +
		`}` + "\n")
	meta, err := backups.NewMetadataJSONReader(file)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(meta.ID(), tc.Equals, "20140909-115934.asdf-zxcv-qwe")
	c.Check(meta.Checksum(), tc.Equals, "")
	c.Check(meta.ChecksumFormat(), tc.Equals, "")
	c.Check(meta.Size(), tc.Equals, int64(0))
	c.Check(meta.Stored(), tc.IsNil)
	c.Check(meta.Started.Unix(), tc.Equals, int64(1410263974))
	c.Check(meta.Finished.Unix(), tc.Equals, int64(1410264034))
	c.Check(meta.Notes, tc.Equals, "")
	c.Check(meta.Origin.Model, tc.Equals, "asdf-zxcv-qwe")
	c.Check(meta.Origin.Machine, tc.Equals, "0")
	c.Check(meta.Origin.Hostname, tc.Equals, "myhost")
	c.Check(meta.Origin.Version.String(), tc.Equals, "1.21-alpha3")
	c.Check(meta.FormatVersion, tc.Equals, int64(2))
	c.Check(meta.DatabaseSizeBytes, tc.Equals, int64(0))
	c.Check(meta.Controller.UUID, tc.Equals, "controller-uuid")
	c.Check(meta.Controller.HANodes, tc.Equals, int64(3))
	c.Check(meta.Instance.MachineID, tc.Equals, "10")
	c.Check(meta.Instance.MachineInstanceID, tc.Equals, "inst-10101010")
}

func (s *metadataSuite) TestNewMetadataJSONReaderUnsupported(c *tc.C) {
	file := bytes.NewBufferString(`{` +
		`"ID":"20140909-115934.asdf-zxcv-qwe",` +
		`"FormatVersion":3,` +
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
		`"Instance":{"MachineID":"10","MachineInstanceID":"inst-10101010"}` +
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
	c.Check(meta.Checksum(), tc.Equals, "sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	c.Check(meta.ChecksumFormat(), tc.Equals, "SHA-256")
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
	c.Check(meta.Controller.HANodes, tc.Equals, backups.UnknownInt64)
	c.Check(meta.Instance.MachineID, tc.Equals, backups.UnknownString)
	c.Check(meta.Instance.MachineInstanceID, tc.Equals, backups.UnknownString)
}

// TestInstanceMetadataRoundTrip verifies that the instance metadata and
// the database size survive a serialise and decode cycle in the archive
// metadata.
func (s *metadataSuite) TestInstanceMetadataRoundTrip(c *tc.C) {
	meta := s.createTestMetadata(c)
	meta.DatabaseSizeBytes = 1 << 40
	meta.Instance = backups.InstanceMetadata{
		AgentImage: "example.com/agent@sha256:source", CharmImage: "example.com/charm@sha256:source",
		Architecture: "arm64", StorageClass: "source", StorageCapacityMiB: 65536,
		ServiceType: "LoadBalancer", ExternalIPs: []string{"192.0.2.1"},
		ServiceAnnotations: map[string]string{"source": "value"},
	}
	reader, err := meta.AsJSONBuffer()
	c.Assert(err, tc.ErrorIsNil)
	decoded, err := backups.NewMetadataJSONReader(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(decoded.FormatVersion, tc.Equals, int64(2))
	c.Check(decoded.Instance, tc.DeepEquals, meta.Instance)
	c.Check(decoded.DatabaseSizeBytes, tc.Equals, meta.DatabaseSizeBytes)
}
