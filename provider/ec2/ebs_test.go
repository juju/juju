// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	awsec2 "gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/ec2/ec2test"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type ebsSuite struct {
	testing.BaseSuite
	srv         localServer
	modelConfig *config.Config
	instanceId  string

	cloudCallCtx context.ProviderCallContext
}

var _ = gc.Suite(&ebsSuite{})

func (s *ebsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&ec2.DestroyVolumeAttempt.Delay, time.Duration(0))

	modelConfig, err := config.New(config.NoDefaults, testing.FakeConfig().Merge(
		testing.Attrs{"type": "ec2"},
	))
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfig = modelConfig

	s.srv.startServer(c)
	s.AddCleanup(func(c *gc.C) { s.srv.stopServer(c) })

	restoreEC2Patching := patchEC2ForTesting(c, s.srv.region)
	s.AddCleanup(func(c *gc.C) { restoreEC2Patching() })

	s.cloudCallCtx = context.NewCloudCallContext()
}

func (s *ebsSuite) ebsProvider(c *gc.C) storage.Provider {
	provider, err := environs.Provider("ec2")
	c.Assert(err, jc.ErrorIsNil)

	credential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": "x",
			"secret-key": "x",
		},
	)
	env, err := environs.Open(provider, environs.OpenParams{
		Cloud: environs.CloudSpec{
			Type:       "ec2",
			Name:       "ec2test",
			Region:     s.srv.region.Name,
			Endpoint:   s.srv.region.EC2Endpoint,
			Credential: &credential,
		},
		Config: s.modelConfig,
	})
	c.Assert(err, jc.ErrorIsNil)

	p, err := env.StorageProvider(ec2.EBS_ProviderType)
	c.Assert(err, jc.ErrorIsNil)
	return p
}

func (s *ebsSuite) TestValidateConfigUnknownConfig(c *gc.C) {
	p := s.ebsProvider(c)
	cfg, err := storage.NewConfig("foo", ec2.EBS_ProviderType, map[string]interface{}{
		"unknown": "config",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, jc.ErrorIsNil) // unknown attrs ignored
}

func (s *ebsSuite) TestSupports(c *gc.C) {
	p := s.ebsProvider(c)
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

func (s *ebsSuite) volumeSource(c *gc.C, cfg *storage.Config) storage.VolumeSource {
	p := s.ebsProvider(c)
	vs, err := p.VolumeSource(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return vs
}

func (s *ebsSuite) createVolumesParams(instanceId string) []storage.VolumeParams {
	if instanceId == "" {
		instanceId = s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Running, nil)[0]
	}
	volume0 := names.NewVolumeTag("0")
	volume1 := names.NewVolumeTag("1")
	volume2 := names.NewVolumeTag("2")
	volume3 := names.NewVolumeTag("3")
	volume4 := names.NewVolumeTag("4")
	params := []storage.VolumeParams{{
		Tag:      volume0,
		Size:     10 * 1000,
		Provider: ec2.EBS_ProviderType,
		Attributes: map[string]interface{}{
			"volume-type": "io1",
			"iops":        30,
		},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: instance.Id(instanceId),
			},
		},
		ResourceTags: map[string]string{
			tags.JujuModel: s.modelConfig.UUID(),
		},
	}, {
		Tag:      volume1,
		Size:     20 * 1000,
		Provider: ec2.EBS_ProviderType,
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: instance.Id(instanceId),
			},
		},
		ResourceTags: map[string]string{
			tags.JujuModel: "something-else",
		},
	}, {
		Tag:      volume2,
		Size:     30 * 1000,
		Provider: ec2.EBS_ProviderType,
		ResourceTags: map[string]string{
			"abc": "123",
		},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: instance.Id(instanceId),
			},
		},
	}, {
		Tag:      volume3,
		Size:     40 * 1000,
		Provider: ec2.EBS_ProviderType,
		ResourceTags: map[string]string{
			"volume-type": "st1",
		},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: instance.Id(instanceId),
			},
		},
	}, {
		Tag:      volume4,
		Size:     50 * 1024,
		Provider: ec2.EBS_ProviderType,
		ResourceTags: map[string]string{
			"volume-type": "sc1",
		},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: instance.Id(instanceId),
			},
		},
	}}
	return params
}

func (s *ebsSuite) createVolumes(vs storage.VolumeSource, instanceId string) ([]storage.CreateVolumesResult, error) {
	return vs.CreateVolumes(s.cloudCallCtx, s.createVolumesParams(instanceId))
}

func (s *ebsSuite) assertCreateVolumes(c *gc.C, vs storage.VolumeSource, instanceId string) {
	results, err := s.createVolumes(vs, instanceId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 5)
	c.Assert(results[0].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("0"),
		storage.VolumeInfo{
			Size:       10240,
			VolumeId:   "vol-0",
			Persistent: true,
		},
	})
	c.Assert(results[1].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("1"),
		storage.VolumeInfo{
			Size:       20480,
			VolumeId:   "vol-1",
			Persistent: true,
		},
	})
	c.Assert(results[2].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("2"),
		storage.VolumeInfo{
			Size:       30720,
			VolumeId:   "vol-2",
			Persistent: true,
		},
	})
	c.Assert(results[3].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("3"),
		storage.VolumeInfo{
			Size:       40960,
			VolumeId:   "vol-3",
			Persistent: true,
		},
	})
	c.Assert(results[4].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("4"),
		storage.VolumeInfo{
			Size:       51200,
			VolumeId:   "vol-4",
			Persistent: true,
		},
	})
	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 5)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Size, gc.Equals, 10)
	c.Assert(ec2Vols.Volumes[1].Size, gc.Equals, 20)
	c.Assert(ec2Vols.Volumes[2].Size, gc.Equals, 30)
	c.Assert(ec2Vols.Volumes[3].Size, gc.Equals, 40)
	c.Assert(ec2Vols.Volumes[4].Size, gc.Equals, 50)
}

var deleteSecurityGroupForTestFunc = func(inst ec2.SecurityGroupCleaner, ctx context.ProviderCallContext, group awsec2.SecurityGroup, _ clock.Clock) error {
	// With an exponential retry for deleting security groups,
	// we never return from local live tests.
	// No need to re-try in tests anyway - just call delete.
	_, err := inst.DeleteSecurityGroup(group)
	return err
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

func (s *ebsSuite) TestCreateVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.assertCreateVolumes(c, vs, "")
}

func (s *ebsSuite) TestVolumeTags(c *gc.C) {
	vs := s.volumeSource(c, nil)
	results, err := s.createVolumes(vs, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 5)
	c.Assert(results[0].Error, jc.ErrorIsNil)
	c.Assert(results[0].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("0"),
		storage.VolumeInfo{
			Size:       10240,
			VolumeId:   "vol-0",
			Persistent: true,
		},
	})
	c.Assert(results[1].Error, jc.ErrorIsNil)
	c.Assert(results[1].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("1"),
		storage.VolumeInfo{
			Size:       20480,
			VolumeId:   "vol-1",
			Persistent: true,
		},
	})
	c.Assert(results[2].Error, jc.ErrorIsNil)
	c.Assert(results[2].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("2"),
		storage.VolumeInfo{
			Size:       30720,
			VolumeId:   "vol-2",
			Persistent: true,
		},
	})
	c.Assert(results[3].Error, jc.ErrorIsNil)
	c.Assert(results[3].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("3"),
		storage.VolumeInfo{
			Size:       40960,
			VolumeId:   "vol-3",
			Persistent: true,
		},
	})
	c.Assert(results[4].Error, jc.ErrorIsNil)
	c.Assert(results[4].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("4"),
		storage.VolumeInfo{
			Size:       51200,
			VolumeId:   "vol-4",
			Persistent: true,
		},
	})
	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 5)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Tags, jc.SameContents, []awsec2.Tag{
		{"juju-model-uuid", "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
		{"Name", "juju-testmodel-volume-0"},
	})
	c.Assert(ec2Vols.Volumes[1].Tags, jc.SameContents, []awsec2.Tag{
		{"juju-model-uuid", "something-else"},
		{"Name", "juju-testmodel-volume-1"},
	})
	c.Assert(ec2Vols.Volumes[2].Tags, jc.SameContents, []awsec2.Tag{
		{"Name", "juju-testmodel-volume-2"},
		{"abc", "123"},
	})
	c.Assert(ec2Vols.Volumes[3].Tags, jc.SameContents, []awsec2.Tag{
		{"Name", "juju-testmodel-volume-3"},
		{"volume-type", "st1"},
	})
	c.Assert(ec2Vols.Volumes[4].Tags, jc.SameContents, []awsec2.Tag{
		{"Name", "juju-testmodel-volume-4"},
		{"volume-type", "sc1"},
	})
}

func (s *ebsSuite) TestVolumeTypeAliases(c *gc.C) {
	instanceIdRunning := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Running, nil)[0]
	vs := s.volumeSource(c, nil)
	ec2Client := ec2.StorageEC2(vs)
	aliases := [][2]string{
		{"magnetic", "standard"},
		{"cold-storage", "sc1"},
		{"optimized-hdd", "st1"},
		{"ssd", "gp2"},
		{"provisioned-iops", "io1"},
	}
	for i, alias := range aliases {
		params := []storage.VolumeParams{{
			Tag:      names.NewVolumeTag("0"),
			Size:     500 * 1024,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": alias[0],
			},
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					InstanceId: instance.Id(instanceIdRunning),
				},
			},
		}}
		if alias[1] == "io1" {
			params[0].Attributes["iops"] = 30
		}
		results, err := vs.CreateVolumes(s.cloudCallCtx, params)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results, gc.HasLen, 1)
		c.Assert(results[0].Error, jc.ErrorIsNil)
		c.Assert(results[0].Volume.VolumeId, gc.Equals, fmt.Sprintf("vol-%d", i))
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

func (s *ebsSuite) TestDestroyVolumesNotFoundReturnsNil(c *gc.C) {
	vs := s.volumeSource(c, nil)
	results, err := vs.DestroyVolumes(s.cloudCallCtx, []string{"vol-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0], jc.ErrorIsNil)
}

func (s *ebsSuite) TestDestroyVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.setupAttachVolumesTest(c, vs, ec2test.Running)

	s.srv.proxy.ModifyResponse = func(resp *http.Response) error {
		resp.StatusCode = http.StatusBadRequest
		return replaceResponseBody(resp, ec2Errors{[]awsec2.Error{{
			Code: "Blocked",
		}}})
	}
	in := []string{"vol-0"}
	results, err := vs.DestroyVolumes(s.cloudCallCtx, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(in))
	for i, result := range results {
		c.Logf("checking volume deletion %d", i)
		c.Assert(result, jc.Satisfies, common.IsCredentialNotValid)
	}
}

func (s *ebsSuite) TestDestroyVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.setupAttachVolumesTest(c, vs, ec2test.Running)
	errs, err := vs.DestroyVolumes(s.cloudCallCtx, []string{"vol-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 4)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Size, gc.Equals, 20)
	c.Assert(ec2Vols.Volumes[1].Size, gc.Equals, 30)
	c.Assert(ec2Vols.Volumes[2].Size, gc.Equals, 40)
	c.Assert(ec2Vols.Volumes[3].Size, gc.Equals, 50)
}

func (s *ebsSuite) TestDestroyVolumesStillAttached(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)
	_, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	errs, err := vs.DestroyVolumes(s.cloudCallCtx, []string{"vol-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 4)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Size, gc.Equals, 20)
	c.Assert(ec2Vols.Volumes[2].Size, gc.Equals, 40)
	c.Assert(ec2Vols.Volumes[3].Size, gc.Equals, 50)
}

func (s *ebsSuite) TestReleaseVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.setupAttachVolumesTest(c, vs, ec2test.Running)
	errs, err := vs.ReleaseVolumes(s.cloudCallCtx, []string{"vol-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes([]string{"vol-0"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 1)
	c.Assert(ec2Vols.Volumes[0].Tags, jc.SameContents, []awsec2.Tag{
		{"juju-controller-uuid", ""},
		{"juju-model-uuid", ""},
		{"Name", "juju-testmodel-volume-0"},
	})
}

func (s *ebsSuite) TestReleaseVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.setupAttachVolumesTest(c, vs, ec2test.Running)

	s.srv.proxy.ModifyResponse = func(resp *http.Response) error {
		resp.StatusCode = http.StatusBadRequest
		return replaceResponseBody(resp, ec2Errors{[]awsec2.Error{{
			Code: "Blocked",
		}}})
	}
	in := []string{"vol-0"}
	results, err := vs.ReleaseVolumes(s.cloudCallCtx, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(in))
	for i, result := range results {
		c.Logf("checking volume release %d", i)
		c.Assert(result, jc.Satisfies, common.IsCredentialNotValid)
	}
}

func (s *ebsSuite) TestReleaseVolumesStillAttached(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)
	_, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	errs, err := vs.ReleaseVolumes(s.cloudCallCtx, []string{"vol-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], gc.ErrorMatches, `cannot release volume "vol-0": attachments still active`)

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes([]string{"vol-0"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 1)
	c.Assert(ec2Vols.Volumes[0].Tags, jc.SameContents, []awsec2.Tag{
		{"juju-model-uuid", "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
		{"Name", "juju-testmodel-volume-0"},
	})
}

func (s *ebsSuite) TestAttachVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)

	s.srv.proxy.ModifyResponse = func(resp *http.Response) error {
		resp.StatusCode = http.StatusBadRequest
		return replaceResponseBody(resp, ec2Errors{[]awsec2.Error{{
			Code: "Blocked",
		}}})
	}
	results, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Assert(results, gc.IsNil)
}

func (s *ebsSuite) TestReleaseVolumesNotFound(c *gc.C) {
	vs := s.volumeSource(c, nil)
	errs, err := vs.ReleaseVolumes(s.cloudCallCtx, []string{"vol-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], gc.ErrorMatches, `cannot release volume "vol-42": vol-42 not found`)
}

func (s *ebsSuite) TestDescribeVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.assertCreateVolumes(c, vs, "")

	vols, err := vs.DescribeVolumes(s.cloudCallCtx, []string{"vol-0", "vol-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, jc.DeepEquals, []storage.DescribeVolumesResult{{
		VolumeInfo: &storage.VolumeInfo{
			Size:       10240,
			VolumeId:   "vol-0",
			Persistent: true,
		},
	}, {
		VolumeInfo: &storage.VolumeInfo{
			Size:       20480,
			VolumeId:   "vol-1",
			Persistent: true,
		},
	}})
}

func (s *ebsSuite) TestDescribeVolumesNotFound(c *gc.C) {
	vs := s.volumeSource(c, nil)
	vols, err := vs.DescribeVolumes(s.cloudCallCtx, []string{"vol-42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vols, gc.HasLen, 1)
	c.Assert(vols[0].Error, gc.ErrorMatches, "vol-42 not found")
}

func (s *ebsSuite) TestDescribeVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.srv.proxy.ModifyResponse = func(resp *http.Response) error {
		resp.StatusCode = http.StatusBadRequest
		return replaceResponseBody(resp, ec2Errors{[]awsec2.Error{{
			Code: "Blocked",
		}}})
	}
	results, err := vs.DescribeVolumes(s.cloudCallCtx, []string{"vol-42"})
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Assert(results, gc.IsNil)
}

func (s *ebsSuite) TestListVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.assertCreateVolumes(c, vs, "")

	// Only one volume created by assertCreateVolumes has
	// the model-uuid tag with the expected value.
	volIds, err := vs.ListVolumes(s.cloudCallCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volIds, jc.SameContents, []string{"vol-0"})
}

func (s *ebsSuite) TestListVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.srv.proxy.ModifyResponse = func(resp *http.Response) error {
		resp.StatusCode = http.StatusBadRequest
		return replaceResponseBody(resp, ec2Errors{[]awsec2.Error{{
			Code: "Blocked",
		}}})
	}
	results, err := vs.ListVolumes(s.cloudCallCtx)
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
	c.Assert(results, gc.IsNil)
}

func (s *ebsSuite) TestListVolumesIgnoresRootDisks(c *gc.C) {
	s.srv.ec2srv.SetCreateRootDisks(true)
	s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Pending, nil)

	// Tag the root disk with the model UUID.
	_, err := s.srv.client.CreateTags([]string{"vol-0"}, []awsec2.Tag{
		{tags.JujuModel, s.modelConfig.UUID()},
	})
	c.Assert(err, jc.ErrorIsNil)

	vs := s.volumeSource(c, nil)
	volIds, err := vs.ListVolumes(s.cloudCallCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volIds, gc.HasLen, 0)
}

func (s *ebsSuite) TestCreateVolumesErrors(c *gc.C) {
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
			Size:     1024,
			Provider: ec2.EBS_ProviderType,
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					InstanceId: "woat",
				},
			},
		},
		err: `querying instance details: instance "woat" not found \(InvalidInstanceID.NotFound\)`,
	}, {
		params: storage.VolumeParams{
			Size:     1024,
			Provider: ec2.EBS_ProviderType,
			Attachment: &storage.VolumeAttachmentParams{
				AttachmentParams: storage.AttachmentParams{
					InstanceId: instance.Id(instanceIdPending),
				},
			},
		},
		err: "cannot attach to non-running instance i-3",
	}, {
		params: storage.VolumeParams{
			Size:       100000000,
			Provider:   ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{},
			Attachment: &attachmentParams,
		},
		err: "volume size 97657 GiB exceeds the maximum of 16384 GiB",
	}, {
		params: storage.VolumeParams{
			Size:     100000000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "gp2",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size 97657 GiB exceeds the maximum of 16384 GiB",
	}, {
		params: storage.VolumeParams{
			Size:     100000000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "io1",
				"iops":        "30",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size 97657 GiB exceeds the maximum of 16384 GiB",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     1000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "io1",
				"iops":        "30",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size is 1 GiB, must be at least 4 GiB",
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
		err: "specified IOPS ratio is 1234/GiB, maximum is 30/GiB",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     10000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "standard",
				"iops":        "30",
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
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     400 * 1024,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "st1",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size is 400 GiB, must be at least 500 GiB",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     17 * 1024 * 1024,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "st1",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size 17408 GiB exceeds the maximum of 16384 GiB",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     10000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "st1",
				"iops":        "30",
			},
			Attachment: &attachmentParams,
		},
		err: `IOPS specified, but volume type is "st1"`,
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     300 * 1024,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "sc1",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size is 300 GiB, must be at least 500 GiB",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     18 * 1024 * 1024,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "sc1",
			},
			Attachment: &attachmentParams,
		},
		err: "volume size 18432 GiB exceeds the maximum of 16384 GiB",
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     10000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "sc1",
				"iops":        "30",
			},
			Attachment: &attachmentParams,
		},
		err: `IOPS specified, but volume type is "sc1"`,
	}} {
		results, err := vs.CreateVolumes(s.cloudCallCtx, []storage.VolumeParams{test.params})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results, gc.HasLen, 1)
		c.Check(results[0].Error, gc.ErrorMatches, test.err)
	}
}

func (s *ebsSuite) TestCreateVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.createVolumesParams("")
	s.srv.proxy.ModifyResponse = func(resp *http.Response) error {
		resp.StatusCode = http.StatusBadRequest
		return replaceResponseBody(resp, ec2Errors{[]awsec2.Error{{
			Code: "Blocked",
		}}})
	}
	results, err := vs.CreateVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	for i, result := range results {
		c.Logf("checking volume creation %d", i)
		c.Assert(result.Volume, gc.IsNil)
		c.Assert(result.VolumeAttachment, gc.IsNil)
		c.Assert(result.Error, jc.Satisfies, common.IsCredentialNotValid)
	}
}

var imageId = "ami-ccf405a5" // Ubuntu Maverick, i386, EBS store

func (s *ebsSuite) setupAttachVolumesTest(
	c *gc.C, vs storage.VolumeSource, state awsec2.InstanceState,
) []storage.VolumeAttachmentParams {

	instanceId := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, state, nil)[0]
	s.assertCreateVolumes(c, vs, instanceId)

	return []storage.VolumeAttachmentParams{{
		Volume:   names.NewVolumeTag("0"),
		VolumeId: "vol-0",
		AttachmentParams: storage.AttachmentParams{
			Machine:    names.NewMachineTag("1"),
			InstanceId: instance.Id(instanceId),
		},
	}}
}

func (s *ebsSuite) TestAttachVolumesNotRunning(c *gc.C) {
	vs := s.volumeSource(c, nil)
	instanceId := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Pending, nil)[0]
	results, err := s.createVolumes(vs, instanceId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.Not(gc.HasLen), 0)
	for _, result := range results {
		c.Check(errors.Cause(result.Error), gc.ErrorMatches, "cannot attach to non-running instance i-3")
	}
}

func (s *ebsSuite) TestAttachVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)
	result, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error, jc.ErrorIsNil)
	c.Assert(result[0].VolumeAttachment, jc.DeepEquals, &storage.VolumeAttachment{
		names.NewVolumeTag("0"),
		names.NewMachineTag("1"),
		storage.VolumeAttachmentInfo{
			DeviceName: "xvdf",
			DeviceLink: "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0",
			ReadOnly:   false,
		},
	})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 5)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Attachments, jc.DeepEquals, []awsec2.VolumeAttachment{{
		VolumeId:   "vol-0",
		InstanceId: "i-3",
		Device:     "/dev/sdf",
		Status:     "attached",
	}})

	// Test idempotency.
	result, err = vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error, jc.ErrorIsNil)
	c.Assert(result[0].VolumeAttachment, jc.DeepEquals, &storage.VolumeAttachment{
		names.NewVolumeTag("0"),
		names.NewMachineTag("1"),
		storage.VolumeAttachmentInfo{
			DeviceName: "xvdf",
			DeviceLink: "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0",
			ReadOnly:   false,
		},
	})
}

func (s *ebsSuite) TestAttachVolumesCreating(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)
	var calls int
	s.srv.proxy.ModifyResponse = makeDescribeVolumesResponseModifier(func(resp *awsec2.VolumesResp) error {
		if len(resp.Volumes) != 1 {
			return errors.New("expected one volume")
		}
		calls++
		if calls == 1 {
			resp.Volumes[0].Status = "creating"
		} else {
			resp.Volumes[0].Status = "available"
		}
		return nil
	})
	result, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error, jc.ErrorIsNil)
	c.Assert(calls, gc.Equals, 2)
}

func (s *ebsSuite) TestAttachVolumesDetaching(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)
	s.srv.proxy.ModifyResponse = makeDescribeVolumesResponseModifier(func(resp *awsec2.VolumesResp) error {
		if len(resp.Volumes) != 1 {
			return errors.New("expected one volume")
		}
		resp.Volumes[0].Status = "in-use"
		resp.Volumes[0].Attachments = append(resp.Volumes[0].Attachments, awsec2.VolumeAttachment{
			InstanceId: "something else",
		})
		return nil
	})
	result, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error, gc.ErrorMatches, "volume vol-0 is attached to something else")
}

func (s *ebsSuite) TestDetachVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)
	_, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	errs, err := vs.DetachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.Volumes(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 5)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Attachments, gc.HasLen, 0)

	// Test idempotent
	errs, err = vs.DetachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
}

func (s *ebsSuite) TestDetachVolumesIncorrectState(c *gc.C) {
	s.testDetachVolumesDetachedState(c, "IncorrectState")
}

func (s *ebsSuite) TestDetachVolumesAttachmentNotFound(c *gc.C) {
	s.testDetachVolumesDetachedState(c, "InvalidAttachment.NotFound")
}

func (s *ebsSuite) testDetachVolumesDetachedState(c *gc.C, errorCode string) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)
	_, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)

	s.srv.proxy.ModifyResponse = func(resp *http.Response) error {
		resp.StatusCode = http.StatusBadRequest
		return replaceResponseBody(resp, ec2Errors{[]awsec2.Error{{
			Code: errorCode,
		}}})
	}
	errs, err := vs.DetachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
}

func (s *ebsSuite) TestImportVolume(c *gc.C) {
	vs := s.volumeSource(c, nil)
	c.Assert(vs, gc.Implements, new(storage.VolumeImporter))

	resp, err := s.srv.client.CreateVolume(awsec2.CreateVolume{
		VolumeSize: 1,
		VolumeType: "gp2",
		AvailZone:  "us-east-1a",
	})
	c.Assert(err, jc.ErrorIsNil)

	volInfo, err := vs.(storage.VolumeImporter).ImportVolume(s.cloudCallCtx, resp.Id, map[string]string{
		"foo": "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volInfo, jc.DeepEquals, storage.VolumeInfo{
		VolumeId:   resp.Id,
		Size:       1024,
		Persistent: true,
	})

	volumes, err := s.srv.client.Volumes([]string{resp.Id}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Volumes, gc.HasLen, 1)
	c.Assert(volumes.Volumes[0].Tags, jc.DeepEquals, []awsec2.Tag{
		{"foo", "bar"},
	})
}

func (s *ebsSuite) TestImportVolumeCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	c.Assert(vs, gc.Implements, new(storage.VolumeImporter))
	resp, err := s.srv.client.CreateVolume(awsec2.CreateVolume{
		VolumeSize: 1,
		VolumeType: "gp2",
		AvailZone:  "us-east-1a",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.srv.proxy.ModifyResponse = func(resp *http.Response) error {
		resp.StatusCode = http.StatusBadRequest
		return replaceResponseBody(resp, ec2Errors{[]awsec2.Error{{
			Code: "Blocked",
		}}})
	}
	_, err = vs.(storage.VolumeImporter).ImportVolume(s.cloudCallCtx, resp.Id, map[string]string{
		"foo": "bar",
	})
	c.Assert(err, jc.Satisfies, common.IsCredentialNotValid)
}

func (s *ebsSuite) TestImportVolumeInUse(c *gc.C) {
	vs := s.volumeSource(c, nil)
	c.Assert(vs, gc.Implements, new(storage.VolumeImporter))

	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)
	_, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)

	volId := params[0].VolumeId
	_, err = vs.(storage.VolumeImporter).ImportVolume(s.cloudCallCtx, volId, map[string]string{})
	c.Assert(err, gc.ErrorMatches, `cannot import volume with status "in-use"`)
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
	nextName = ec2.BlockDeviceNamer(false)
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
	nextName = ec2.BlockDeviceNamer(true)
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
	mapping := ec2.GetBlockDeviceMappings(constraints.Value{}, "trusty", false)
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

func (*blockDeviceMappingSuite) TestGetBlockDeviceMappingsController(c *gc.C) {
	mapping := ec2.GetBlockDeviceMappings(constraints.Value{}, "trusty", true)
	c.Assert(mapping, gc.DeepEquals, []awsec2.BlockDeviceMapping{{
		VolumeSize: 32,
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

func makeDescribeVolumesResponseModifier(modify func(*awsec2.VolumesResp) error) func(*http.Response) error {
	return func(resp *http.Response) error {
		if resp.Request.URL.Query().Get("Action") != "DescribeVolumes" {
			return nil
		}
		var respDecoded struct {
			XMLName xml.Name
			awsec2.VolumesResp
		}
		if err := xml.NewDecoder(resp.Body).Decode(&respDecoded); err != nil {
			return err
		}
		resp.Body.Close()

		if err := modify(&respDecoded.VolumesResp); err != nil {
			return err
		}
		return replaceResponseBody(resp, &respDecoded)
	}
}

func replaceResponseBody(resp *http.Response, value interface{}) error {
	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(value); err != nil {
		return err
	}
	resp.Body = ioutil.NopCloser(&buf)
	return nil
}

type ec2Errors struct {
	Errors []awsec2.Error `xml:"Errors>Error"`
}
