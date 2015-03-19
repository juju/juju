// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	awsec2 "gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/ec2/ec2test"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type storageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (*storageSuite) TestValidateConfigInvalidConfig(c *gc.C) {
	p := ec2.EBSProvider()
	cfg, err := storage.NewConfig("foo", ec2.EBS_ProviderType, map[string]interface{}{
		"invalid": "config",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, `unknown provider config option "invalid"`)
}

func (s *storageSuite) TestSupports(c *gc.C) {
	p := ec2.EBSProvider()
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

func (*storageSuite) TestTranslateUserEBSOptions(c *gc.C) {
	for _, vType := range []string{"magnetic", "ssd", "provisioned-iops"} {
		in := map[string]interface{}{
			"volume-type": vType,
			"foo":         "bar",
		}
		var expected string
		switch vType {
		case "magnetic":
			expected = "standard"
		case "ssd":
			expected = "gp2"
		case "provisioned-iops":
			expected = "io1"
		}
		out := ec2.TranslateUserEBSOptions(in)
		c.Assert(out, jc.DeepEquals, map[string]interface{}{
			"volume-type": expected,
			"foo":         "bar",
		})
	}
}

var _ = gc.Suite(&ebsVolumeSuite{})

type ebsVolumeSuite struct {
	testing.BaseSuite
	jujutest.Tests
	srv                localServer
	restoreEC2Patching func()
}

func (s *ebsVolumeSuite) SetUpSuite(c *gc.C) {
	// Upload arches that ec2 supports; add to this
	// as ec2 coverage expands.
	s.UploadArches = []string{arch.AMD64, arch.I386}
	s.TestConfig = localConfigAttrs
	s.restoreEC2Patching = patchEC2ForTesting()
	s.BaseSuite.SetUpSuite(c)
}

func (s *ebsVolumeSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.restoreEC2Patching()
}

func (s *ebsVolumeSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.srv.startServer(c)
	s.Tests.SetUpTest(c)
	s.PatchValue(&version.Current, version.Binary{
		Number: version.Current.Number,
		Series: testing.FakeDefaultSeries,
		Arch:   arch.AMD64,
	})

}

func (s *ebsVolumeSuite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	s.srv.stopServer(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *ebsVolumeSuite) volumeSource(c *gc.C, cfg *storage.Config) storage.VolumeSource {
	envCfg, err := config.New(config.NoDefaults, s.TestConfig)
	c.Assert(err, jc.ErrorIsNil)
	p := ec2.EBSProvider()
	vs, err := p.VolumeSource(envCfg, cfg)
	c.Assert(err, jc.ErrorIsNil)
	return vs
}

func (s *ebsVolumeSuite) assertCreateVolumes(c *gc.C, vs storage.VolumeSource, zone string) {
	volume0 := names.NewVolumeTag("0")
	volume1 := names.NewVolumeTag("1")
	params := []storage.VolumeParams{
		{
			Tag:      volume0,
			Size:     10 * 1000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"availability-zone": zone,
				"volume-type":       "io1",
			},
		},
		{
			Tag:      volume1,
			Size:     20 * 1000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"availability-zone": zone,
			},
		},
	}
	vols, _, err := vs.CreateVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, gc.HasLen, 2)
	c.Assert(vols, jc.SameContents, []storage.Volume{
		{
			Tag:      volume0,
			Size:     10240,
			VolumeId: "vol-0",
		},
		{
			Tag:      volume1,
			Size:     20480,
			VolumeId: "vol-1",
		},
	})
	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 2)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Size, gc.Equals, 10)
	c.Assert(ec2Vols.Volumes[1].Size, gc.Equals, 20)
}

type volumeSorter struct {
	vols []awsec2.Volume
	less func(i, j awsec2.Volume) bool
}

func sortBySize(vols []awsec2.Volume) {
	sort.Sort(volumeSorter{vols, func(i, j awsec2.Volume) bool {
		return i.Size < j.Size
	}})
}

func (s volumeSorter) Len() int {
	return len(s.vols)
}

func (s volumeSorter) Swap(i, j int) {
	s.vols[i], s.vols[j] = s.vols[j], s.vols[i]
}

func (s volumeSorter) Less(i, j int) bool {
	return s.less(s.vols[i], s.vols[j])
}

func (s *ebsVolumeSuite) TestCreateVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.assertCreateVolumes(c, vs, "us-east-1")
}

func (s *ebsVolumeSuite) TestDeleteVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.assertCreateVolumes(c, vs, "us-east-1")
	vs.DestroyVolumes([]string{"vol-0"})
	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 1)
	c.Assert(ec2Vols.Volumes[0].Size, gc.Equals, 20)
}

func (s *ebsVolumeSuite) TestVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.assertCreateVolumes(c, vs, "us-east-1")

	vols, err := vs.DescribeVolumes([]string{"vol-0", "vol-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, gc.HasLen, 2)
	c.Assert(vols, jc.SameContents, []storage.Volume{
		{
			Size:     10240,
			VolumeId: "vol-0",
		},
		{
			Size:     20480,
			VolumeId: "vol-1",
		},
	})
}

func (s *ebsVolumeSuite) TestCreateVolumesErrors(c *gc.C) {
	vs := s.volumeSource(c, nil)
	volume0 := names.NewVolumeTag("0")

	for _, test := range []struct {
		params storage.VolumeParams
		err    string
	}{
		{
			params: storage.VolumeParams{
				Provider: ec2.EBS_ProviderType,
			},
			err: "missing availability zone",
		},
		{
			params: storage.VolumeParams{
				Size:     100000000,
				Provider: ec2.EBS_ProviderType,
				Attributes: map[string]interface{}{
					"availability-zone": "us-east-1",
				},
			},
			err: "97657 GiB exceeds the maximum of 1024 GiB",
		},
		{
			params: storage.VolumeParams{
				Tag:      volume0,
				Size:     1000,
				Provider: ec2.EBS_ProviderType,
				Attributes: map[string]interface{}{
					"availability-zone": "us-east-1",
					"volume-type":       "io1",
					"iops":              "1234",
				},
			},
			err: "volume size is 1 GiB, must be at least 10 GiB for provisioned IOPS",
		},
		{
			params: storage.VolumeParams{
				Tag:      volume0,
				Size:     10000,
				Provider: ec2.EBS_ProviderType,
				Attributes: map[string]interface{}{
					"availability-zone": "us-east-1",
					"volume-type":       "io1",
					"iops":              "1234",
				},
			},
			err: "volume size is 10 GiB, must be at least 41 GiB to support 1234 IOPS",
		},
		{
			params: storage.VolumeParams{
				Tag:      volume0,
				Size:     10000,
				Provider: ec2.EBS_ProviderType,
				Attributes: map[string]interface{}{
					"availability-zone": "us-east-1",
					"volume-type":       "standard",
					"iops":              "1234",
				},
			},
			err: `IOPS specified, but volume type is "standard"`,
		}} {
		_, _, err := vs.CreateVolumes([]storage.VolumeParams{test.params})
		c.Check(err, gc.ErrorMatches, test.err)
	}
}

var imageId = "ami-ccf405a5" // Ubuntu Maverick, i386, EBS store

func (s *ebsVolumeSuite) setupAttachVolumesTest(
	c *gc.C, vs storage.VolumeSource, zone string, state awsec2.InstanceState,
) []storage.VolumeAttachmentParams {

	s.assertCreateVolumes(c, vs, zone)
	ids := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, state, nil)

	return []storage.VolumeAttachmentParams{{
		Volume:   names.NewVolumeTag("0"),
		VolumeId: "vol-0",
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("1"),
			InstanceId: instance.Id(ids[0]),
		},
	}}
}

func (s *ebsVolumeSuite) TestAttachVolumesNotRunning(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, "us-east-1c", ec2test.Pending)
	_, err := vs.AttachVolumes(params)
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".* these instances are not running: i-3")
}

func (s *ebsVolumeSuite) TestAttachVolumesWrongZone(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, "us-east-1", ec2test.Running)
	_, err := vs.AttachVolumes(params)
	c.Assert(err, gc.ErrorMatches, `.* volume availability zone "us-east-1" must match instance zone "us-east-1c" .*`)
}

func (s *ebsVolumeSuite) TestAttachVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, "us-east-1c", ec2test.Running)
	result, err := vs.AttachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0], gc.Equals, storage.VolumeAttachment{
		Volume:     names.NewVolumeTag("0"),
		Machine:    names.NewMachineTag("1"),
		DeviceName: "/dev/sdf",
		ReadOnly:   false,
	})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 2)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Attachments, jc.DeepEquals, []awsec2.VolumeAttachment{{
		VolumeId:   "vol-0",
		InstanceId: "i-3",
		Device:     "/dev/sdf",
		Status:     "attaching",
	}})

	// Test idempotent.
	result, err = vs.AttachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0], gc.Equals, storage.VolumeAttachment{
		Volume:     names.NewVolumeTag("0"),
		Machine:    names.NewMachineTag("1"),
		DeviceName: "/dev/sdf",
		ReadOnly:   false,
	})
}

func (s *ebsVolumeSuite) TestDetachVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, "us-east-1c", ec2test.Running)
	_, err := vs.AttachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	err = vs.DetachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 2)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Attachments, gc.HasLen, 0)

	// Test idempotent
	err = vs.DetachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
}
