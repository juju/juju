// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	awsec2 "gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/ec2/ec2test"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
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

func (*storageSuite) TestValidateConfigUnknownConfig(c *gc.C) {
	p := ec2.EBSProvider()
	cfg, err := storage.NewConfig("foo", ec2.EBS_ProviderType, map[string]interface{}{
		"unknown": "config",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, jc.ErrorIsNil) // unknown attrs ignored
}

func (s *storageSuite) TestSupports(c *gc.C) {
	p := ec2.EBSProvider()
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

var _ = gc.Suite(&ebsVolumeSuite{})

type ebsVolumeSuite struct {
	testing.BaseSuite
	jujutest.Tests
	srv                localServer
	restoreEC2Patching func()

	instanceId string
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
	instanceIdRunning := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Running, nil)[0]

	volume0 := names.NewVolumeTag("0")
	volume1 := names.NewVolumeTag("1")
	volume2 := names.NewVolumeTag("2")
	params := []storage.VolumeParams{{
		Tag:      volume0,
		Size:     10 * 1000,
		Provider: ec2.EBS_ProviderType,
		Attributes: map[string]interface{}{
			"persistent":        true,
			"availability-zone": zone,
			"volume-type":       "io1",
			"iops":              100,
		},
	}, {
		Tag:      volume1,
		Size:     20 * 1000,
		Provider: ec2.EBS_ProviderType,
		Attributes: map[string]interface{}{
			"persistent":        true,
			"availability-zone": zone,
		},
	}, {
		Tag:      volume2,
		Size:     30 * 1000,
		Provider: ec2.EBS_ProviderType,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: instance.Id(instanceIdRunning),
			},
		},
	}}
	vols, _, err := vs.CreateVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, gc.HasLen, 3)
	c.Assert(vols, jc.SameContents, []storage.Volume{{
		Tag:        volume0,
		Size:       10240,
		VolumeId:   "vol-0",
		Persistent: true,
	}, {
		Tag:        volume1,
		Size:       20480,
		VolumeId:   "vol-1",
		Persistent: true,
	}, {
		Tag:        volume2,
		Size:       30720,
		VolumeId:   "vol-2",
		Persistent: false,
	}})
	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 3)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Size, gc.Equals, 10)
	c.Assert(ec2Vols.Volumes[1].Size, gc.Equals, 20)
	c.Assert(ec2Vols.Volumes[2].Size, gc.Equals, 30)
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

func (s *ebsVolumeSuite) TestVolumeTypeAliases(c *gc.C) {
	vs := s.volumeSource(c, nil)
	ec2Client := ec2.StorageEC2(vs)
	zone := "us-east-1"
	aliases := [][2]string{
		{"magnetic", "standard"},
		{"ssd", "gp2"},
		{"provisioned-iops", "io1"},
	}
	for i, alias := range aliases {
		params := []storage.VolumeParams{{
			Tag:      names.NewVolumeTag("0"),
			Size:     10 * 1000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"persistent":        true,
				"availability-zone": zone,
				"volume-type":       alias[0],
			},
		}}
		if alias[1] == "io1" {
			params[0].Attributes["iops"] = 100
		}
		vols, _, err := vs.CreateVolumes(params)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(vols, gc.HasLen, 1)
		c.Assert(vols[0].VolumeId, gc.Equals, fmt.Sprintf("vol-%d", i))
	}
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, len(aliases))
	sort.Sort(volumeSorter{ec2Vols.Volumes, func(i, j awsec2.Volume) bool {
		return i.Id < j.Id
	}})
	for i, alias := range aliases {
		c.Assert(ec2Vols.Volumes[i].VolumeType, gc.Equals, alias[1])
	}
}

func (s *ebsVolumeSuite) TestDeleteVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.assertCreateVolumes(c, vs, "us-east-1")
	vs.DestroyVolumes([]string{"vol-0"})
	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 2)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Size, gc.Equals, 20)
}

func (s *ebsVolumeSuite) TestVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.assertCreateVolumes(c, vs, "us-east-1")

	vols, err := vs.DescribeVolumes([]string{"vol-0", "vol-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, gc.HasLen, 2)
	c.Assert(vols, jc.SameContents, []storage.Volume{{
		Size:     10240,
		VolumeId: "vol-0",
	}, {
		Size:     20480,
		VolumeId: "vol-1",
	}})
}

func (s *ebsVolumeSuite) TestCreateVolumesErrors(c *gc.C) {
	vs := s.volumeSource(c, nil)
	volume0 := names.NewVolumeTag("0")

	instanceIdPending := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Pending, nil)[0]
	instanceIdRunning := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Running, nil)[0]
	attachmentParams := storage.VolumeAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			InstanceId: instance.Id(instanceIdRunning),
		},
	}

	for _, test := range []struct {
		params storage.VolumeParams
		err    string
	}{{
		params: storage.VolumeParams{
			Provider:   ec2.EBS_ProviderType,
			Attachment: &storage.VolumeAttachmentParams{},
		},
		err: "need running instance to provision volume",
	}, {
		params: storage.VolumeParams{
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"persistent":        false,
				"availability-zone": "us-east-1",
			},
			Attachment: &attachmentParams,
		},
		err: "cannot specify availability zone for non-persistent volume",
	}, {
		params: storage.VolumeParams{
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"persistent": true,
			},
		},
		err: "missing availability zone for persistent volume",
	}, {
		params: storage.VolumeParams{
			Provider: ec2.EBS_ProviderType,
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					InstanceId: instance.Id(instanceIdPending),
				},
			},
		},
		err: "querying instance details: volumes can only be attached to running instances, these instances are not running: i-3",
	}, {
		params: storage.VolumeParams{
			Size:       100000000,
			Provider:   ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{},
			Attachment: &attachmentParams,
		},
		err: "97657 GiB exceeds the maximum of 1024 GiB",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     1000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "io1",
				"iops":        "1234",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size is 1 GiB, must be at least 10 GiB for provisioned IOPS",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     10000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "io1",
				"iops":        "1234",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size is 10 GiB, must be at least 41 GiB to support 1234 IOPS",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     10000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "standard",
				"iops":        "1234",
			},
			Attachment: &attachmentParams,
		},
		err: `IOPS specified, but volume type is "standard"`,
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     10000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "what",
			},
			Attachment: &attachmentParams,
		},
		err: "validating EBS storage config: volume-type: unexpected value \"what\"",
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
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".* these instances are not running: i-4")
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
		DeviceName: "xvdf",
		ReadOnly:   false,
	})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 3)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Attachments, jc.DeepEquals, []awsec2.VolumeAttachment{{
		VolumeId:   "vol-0",
		InstanceId: "i-4",
		Device:     "/dev/sdf",
		Status:     "attached",
	}})

	// Test idempotency.
	result, err = vs.AttachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0], gc.Equals, storage.VolumeAttachment{
		Volume:     names.NewVolumeTag("0"),
		Machine:    names.NewMachineTag("1"),
		DeviceName: "xvdf",
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
	c.Assert(ec2Vols.Volumes, gc.HasLen, 3)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Attachments, gc.HasLen, 0)

	// Test idempotent
	err = vs.DetachVolumes(params)
	c.Assert(err, jc.ErrorIsNil)
}

type blockDeviceMappingSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&blockDeviceMappingSuite{})

func (*blockDeviceMappingSuite) TestBlockDeviceNamer(c *gc.C) {
	var nextName func() (string, string, error)
	expect := func(expectRequest, expectActual string) {
		request, actual, err := nextName()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(request, gc.Equals, expectRequest)
		c.Assert(actual, gc.Equals, expectActual)
	}
	expectN := func(expectRequest, expectActual string) {
		for i := 1; i <= 6; i++ {
			request, actual, err := nextName()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(request, gc.Equals, expectRequest+strconv.Itoa(i))
			c.Assert(actual, gc.Equals, expectActual+strconv.Itoa(i))
		}
	}
	expectErr := func(expectErr string) {
		_, _, err := nextName()
		c.Assert(err, gc.ErrorMatches, expectErr)
	}

	// First without numbers.
	nextName = ec2.BlockDeviceNamer(awsec2.Instance{
		VirtType: "hvm",
	})
	expect("/dev/sdf", "xvdf")
	expect("/dev/sdg", "xvdg")
	expect("/dev/sdh", "xvdh")
	expect("/dev/sdi", "xvdi")
	expect("/dev/sdj", "xvdj")
	expect("/dev/sdk", "xvdk")
	expect("/dev/sdl", "xvdl")
	expect("/dev/sdm", "xvdm")
	expect("/dev/sdn", "xvdn")
	expect("/dev/sdo", "xvdo")
	expect("/dev/sdp", "xvdp")
	expectErr("too many EBS volumes to attach")

	// Now with numbers.
	nextName = ec2.BlockDeviceNamer(awsec2.Instance{
		VirtType: "paravirtual",
	})
	expect("/dev/sdf1", "xvdf1")
	expect("/dev/sdf2", "xvdf2")
	expect("/dev/sdf3", "xvdf3")
	expect("/dev/sdf4", "xvdf4")
	expect("/dev/sdf5", "xvdf5")
	expect("/dev/sdf6", "xvdf6")
	expectN("/dev/sdg", "xvdg")
	expectN("/dev/sdh", "xvdh")
	expectN("/dev/sdi", "xvdi")
	expectN("/dev/sdj", "xvdj")
	expectN("/dev/sdk", "xvdk")
	expectN("/dev/sdl", "xvdl")
	expectN("/dev/sdm", "xvdm")
	expectN("/dev/sdn", "xvdn")
	expectN("/dev/sdo", "xvdo")
	expectN("/dev/sdp", "xvdp")
	expectErr("too many EBS volumes to attach")
}

func (*blockDeviceMappingSuite) TestGetBlockDeviceMappings(c *gc.C) {
	mapping, err := ec2.GetBlockDeviceMappings(constraints.Value{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mapping, gc.DeepEquals, []awsec2.BlockDeviceMapping{{
		VolumeSize: 8,
		DeviceName: "/dev/sda1",
	}, {
		VirtualName: "ephemeral0",
		DeviceName:  "/dev/sdb",
	}, {
		VirtualName: "ephemeral1",
		DeviceName:  "/dev/sdc",
	}, {
		VirtualName: "ephemeral2",
		DeviceName:  "/dev/sdd",
	}, {
		VirtualName: "ephemeral3",
		DeviceName:  "/dev/sde",
	}})
}
