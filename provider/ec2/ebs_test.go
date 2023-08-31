// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	stdcontext "context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/ec2"
	ec2test "github.com/juju/juju/provider/ec2/internal/testing"
	"github.com/juju/juju/testing"
)

type ebsSuite struct {
	testing.BaseSuite
	srv         localServer
	modelConfig *config.Config

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

	s.cloudCallCtx = context.NewEmptyCloudCallContext()
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
	clientFunc := func(ctx stdcontext.Context, spec environscloudspec.CloudSpec, options ...ec2.ClientOption) (ec2.Client, error) {
		c.Assert(spec.Region, gc.Equals, "test")
		return s.srv.ec2srv, nil
	}

	ctx := stdcontext.WithValue(s.cloudCallCtx, ec2.AWSClientContextKey, clientFunc)
	env, err := environs.Open(ctx, provider, environs.OpenParams{
		Cloud: environscloudspec.CloudSpec{
			Type:       "ec2",
			Name:       "ec2test",
			Region:     *s.srv.region.RegionName,
			Endpoint:   *s.srv.region.Endpoint,
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

func (s *ebsSuite) createVolumesParams(c *gc.C, instanceId string) []storage.VolumeParams {
	if instanceId == "" {
		inst, err := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Running, nil)
		c.Assert(err, jc.ErrorIsNil)
		instanceId = inst[0]
	}
	volume0 := names.NewVolumeTag("0")
	volume1 := names.NewVolumeTag("1")
	volume2 := names.NewVolumeTag("2")
	volume3 := names.NewVolumeTag("3")
	volume4 := names.NewVolumeTag("4")
	volume5 := names.NewVolumeTag("5")
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
	}, {
		Tag:      volume5,
		Size:     60 * 1024,
		Provider: ec2.EBS_ProviderType,
		ResourceTags: map[string]string{
			"volume-type": "gp3",
			"encrypted":   "true",
			"kms-key-id":  "123456789",
			"throughput":  "500M",
		},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				InstanceId: instance.Id(instanceId),
			},
		},
	}}
	return params
}

func (s *ebsSuite) createVolumes(c *gc.C, vs storage.VolumeSource, instanceId string) ([]storage.CreateVolumesResult, error) {
	return vs.CreateVolumes(s.cloudCallCtx, s.createVolumesParams(c, instanceId))
}

func (s *ebsSuite) assertCreateVolumes(c *gc.C, vs storage.VolumeSource, instanceId string) {
	results, err := s.createVolumes(c, vs, instanceId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 6)
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
	c.Assert(results[5].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("5"),
		storage.VolumeInfo{
			Size:       61440,
			VolumeId:   "vol-5",
			Persistent: true,
		},
	})
	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 6)
	sortBySize(ec2Vols.Volumes)
	c.Assert(aws.ToInt32(ec2Vols.Volumes[0].Size), gc.Equals, int32(10))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[1].Size), gc.Equals, int32(20))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[2].Size), gc.Equals, int32(30))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[3].Size), gc.Equals, int32(40))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[4].Size), gc.Equals, int32(50))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[5].Size), gc.Equals, int32(60))
}

type volumeSorter struct {
	vols []types.Volume
	less func(i, j types.Volume) bool
}

func sortBySize(vols []types.Volume) {
	sort.Sort(volumeSorter{vols, func(i, j types.Volume) bool {
		return aws.ToInt32(i.Size) < aws.ToInt32(j.Size)
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
	results, err := s.createVolumes(c, vs, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 6)
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
	c.Assert(results[5].Error, jc.ErrorIsNil)
	c.Assert(results[5].Volume, jc.DeepEquals, &storage.Volume{
		names.NewVolumeTag("5"),
		storage.VolumeInfo{
			Size:       61440,
			VolumeId:   "vol-5",
			Persistent: true,
		},
	})
	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 6)
	sortBySize(ec2Vols.Volumes)
	compareTags(c, ec2Vols.Volumes[0].Tags, []tagInfo{
		{"juju-model-uuid", "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
		{"Name", "juju-testmodel-volume-0"},
	})
	compareTags(c, ec2Vols.Volumes[1].Tags, []tagInfo{
		{"juju-model-uuid", "something-else"},
		{"Name", "juju-testmodel-volume-1"},
	})
	compareTags(c, ec2Vols.Volumes[2].Tags, []tagInfo{
		{"Name", "juju-testmodel-volume-2"},
		{"abc", "123"},
	})
	compareTags(c, ec2Vols.Volumes[3].Tags, []tagInfo{
		{"Name", "juju-testmodel-volume-3"},
		{"volume-type", "st1"},
	})
	compareTags(c, ec2Vols.Volumes[4].Tags, []tagInfo{
		{"Name", "juju-testmodel-volume-4"},
		{"volume-type", "sc1"},
	})
	compareTags(c, ec2Vols.Volumes[5].Tags, []tagInfo{
		{"Name", "juju-testmodel-volume-5"},
		{"volume-type", "gp3"},
		{"encrypted", "true"},
		{"kms-key-id", "123456789"},
		{"throughput", "500M"},
	})
}

func (s *ebsSuite) TestVolumeTypeAliases(c *gc.C) {
	inst, err := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Running, nil)
	c.Assert(err, jc.ErrorIsNil)
	instanceIdRunning := inst[0]
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
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, len(aliases))
	sort.Sort(volumeSorter{ec2Vols.Volumes, func(i, j types.Volume) bool {
		return aws.ToString(i.VolumeId) < aws.ToString(j.VolumeId)
	}})
	for i, alias := range aliases {
		c.Assert(string(ec2Vols.Volumes[i].VolumeType), gc.Equals, alias[1])
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

	s.srv.ec2srv.SetAPIError("DeleteVolume", &smithy.GenericAPIError{Code: "Blocked"})

	in := []string{"vol-0"}
	results, err := vs.DestroyVolumes(s.cloudCallCtx, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(in))
	for i, result := range results {
		c.Logf("checking volume deletion %d", i)
		c.Assert(errors.Is(result, common.ErrorCredentialNotValid), jc.IsTrue)
	}
}

func (s *ebsSuite) TestDestroyVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.setupAttachVolumesTest(c, vs, ec2test.Running)
	errs, err := vs.DestroyVolumes(s.cloudCallCtx, []string{"vol-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 5)
	sortBySize(ec2Vols.Volumes)
	c.Assert(aws.ToInt32(ec2Vols.Volumes[0].Size), gc.Equals, int32(20))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[1].Size), gc.Equals, int32(30))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[2].Size), gc.Equals, int32(40))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[3].Size), gc.Equals, int32(50))
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
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 5)
	sortBySize(ec2Vols.Volumes)
	c.Assert(aws.ToInt32(ec2Vols.Volumes[0].Size), gc.Equals, int32(20))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[2].Size), gc.Equals, int32(40))
	c.Assert(aws.ToInt32(ec2Vols.Volumes[3].Size), gc.Equals, int32(50))
}

func (s *ebsSuite) TestReleaseVolumes(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.setupAttachVolumesTest(c, vs, ec2test.Running)
	errs, err := vs.ReleaseVolumes(s.cloudCallCtx, []string{"vol-0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})

	ec2Client := ec2.StorageEC2(vs)
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, &awsec2.DescribeVolumesInput{
		VolumeIds: []string{"vol-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 1)
	compareTags(c, ec2Vols.Volumes[0].Tags, []tagInfo{
		{"juju-controller-uuid", ""},
		{"juju-model-uuid", ""},
		{"Name", "juju-testmodel-volume-0"},
	})
}

func (s *ebsSuite) TestReleaseVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	s.setupAttachVolumesTest(c, vs, ec2test.Running)

	s.srv.ec2srv.SetAPIError("DescribeVolumes", &smithy.GenericAPIError{Code: "Blocked"})
	in := []string{"vol-0"}
	results, err := vs.ReleaseVolumes(s.cloudCallCtx, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, len(in))
	for i, result := range results {
		c.Logf("checking volume release %d", i)
		c.Assert(errors.Is(result, common.ErrorCredentialNotValid), jc.IsTrue)
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
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, &awsec2.DescribeVolumesInput{
		VolumeIds: []string{"vol-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 1)
	compareTags(c, ec2Vols.Volumes[0].Tags, []tagInfo{
		{"juju-model-uuid", "deadbeef-0bad-400d-8000-4b1d0d06f00d"},
		{"Name", "juju-testmodel-volume-0"},
	})
}

func (s *ebsSuite) TestAttachVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.setupAttachVolumesTest(c, vs, ec2test.Running)

	s.srv.ec2srv.SetAPIError("AttachVolume", &smithy.GenericAPIError{Code: "Blocked"})

	results, err := vs.AttachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(errors.Is(results[0].Error, common.ErrorCredentialNotValid), jc.IsTrue)
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

	s.srv.ec2srv.SetAPIError("DescribeVolumes", &smithy.GenericAPIError{Code: "Blocked"})

	results, err := vs.DescribeVolumes(s.cloudCallCtx, []string{"vol-42"})
	c.Assert(errors.Is(err, common.ErrorCredentialNotValid), jc.IsTrue)
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

	s.srv.ec2srv.SetAPIError("DescribeVolumes", &smithy.GenericAPIError{Code: "Blocked"})

	results, err := vs.ListVolumes(s.cloudCallCtx)
	c.Assert(errors.Is(err, common.ErrorCredentialNotValid), jc.IsTrue)
	c.Assert(results, gc.IsNil)
}

func (s *ebsSuite) TestListVolumesIgnoresRootDisks(c *gc.C) {
	s.srv.ec2srv.SetCreateRootDisks(true)
	s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Pending, nil)

	// Tag the root disk with the model UUID.
	_, err := s.srv.ec2srv.CreateTags(s.cloudCallCtx, &awsec2.CreateTagsInput{
		Resources: []string{"vol-0"},
		Tags: []types.Tag{
			{Key: aws.String(tags.JujuModel), Value: aws.String(s.modelConfig.UUID())},
		},
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

	inst, err := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Pending, nil)
	c.Assert(err, jc.ErrorIsNil)
	instanceIdPending := inst[0]
	inst, err = s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Running, nil)
	instanceIdRunning := inst[0]
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
		err: `querying instance details: api error InvalidInstanceID.NotFound: instance "woat" not found`,
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
	}, {
		params: storage.VolumeParams{
			Tag:      volume0,
			Size:     10000,
			Provider: ec2.EBS_ProviderType,
			Attributes: map[string]interface{}{
				"volume-type": "gp2",
				"throughput":  "30",
			},
			Attachment: &attachmentParams,
		},
		err: `"throughput" cannot be specified when volume type is "gp2"`,
	}} {
		results, err := vs.CreateVolumes(s.cloudCallCtx, []storage.VolumeParams{test.params})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results, gc.HasLen, 1)
		c.Check(results[0].Error, gc.ErrorMatches, test.err)
	}
}

func (s *ebsSuite) TestCreateVolumesCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	params := s.createVolumesParams(c, "")

	s.srv.ec2srv.SetAPIError("CreateVolume", &smithy.GenericAPIError{Code: "Blocked"})

	results, err := vs.CreateVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	for i, result := range results {
		c.Logf("checking volume creation %d", i)
		c.Assert(result.Volume, gc.IsNil)
		c.Assert(result.VolumeAttachment, gc.IsNil)
		c.Assert(errors.Is(result.Error, common.ErrorCredentialNotValid), jc.IsTrue)
	}
}

var imageId = "ami-ccf405a5" // Ubuntu Maverick, i386, EBS store

func (s *ebsSuite) setupAttachVolumesTest(
	c *gc.C, vs storage.VolumeSource, state types.InstanceState,
) []storage.VolumeAttachmentParams {

	inst, err := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	instanceId := inst[0]
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
	inst, err := s.srv.ec2srv.NewInstances(1, "m1.medium", imageId, ec2test.Pending, nil)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.createVolumes(c, vs, inst[0])
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
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 6)
	sortBySize(ec2Vols.Volumes)
	c.Assert(ec2Vols.Volumes[0].Attachments, jc.DeepEquals, []types.VolumeAttachment{{
		VolumeId:   aws.String("vol-0"),
		InstanceId: aws.String("i-3"),
		Device:     aws.String("/dev/sdf"),
		State:      "attached",
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
	s.srv.ec2srv.SetAPIModifiers("DescribeVolumes", func(out interface{}) {
		out.(*awsec2.DescribeVolumesOutput).Volumes[0].State = "creating"
		calls++
	}, func(out interface{}) {
		out.(*awsec2.DescribeVolumesOutput).Volumes[0].State = "available"
		calls++
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
	s.srv.ec2srv.SetAPIModifiers("DescribeVolumes", func(out interface{}) {
		vols := out.(*awsec2.DescribeVolumesOutput).Volumes
		vols[0].State = "in-use"
		vols[0].Attachments = append(vols[0].Attachments, types.VolumeAttachment{
			InstanceId: aws.String("something else"),
		})
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
	ec2Vols, err := ec2Client.DescribeVolumes(s.cloudCallCtx, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec2Vols.Volumes, gc.HasLen, 6)
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

	s.srv.ec2srv.SetAPIError("DetachVolume", &smithy.GenericAPIError{Code: errorCode})

	errs, err := vs.DetachVolumes(s.cloudCallCtx, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, jc.DeepEquals, []error{nil})
}

func (s *ebsSuite) TestImportVolume(c *gc.C) {
	vs := s.volumeSource(c, nil)
	c.Assert(vs, gc.Implements, new(storage.VolumeImporter))

	resp, err := s.srv.ec2srv.CreateVolume(s.cloudCallCtx, &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("us-east-1a"),
	})
	c.Assert(err, jc.ErrorIsNil)

	volID := aws.ToString(resp.VolumeId)
	volInfo, err := vs.(storage.VolumeImporter).ImportVolume(s.cloudCallCtx, volID, map[string]string{
		"foo": "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volInfo, jc.DeepEquals, storage.VolumeInfo{
		VolumeId:   volID,
		Size:       1024,
		Persistent: true,
	})

	volumes, err := s.srv.ec2srv.DescribeVolumes(s.cloudCallCtx, &awsec2.DescribeVolumesInput{
		VolumeIds: []string{volID},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes.Volumes, gc.HasLen, 1)
	compareTags(c, volumes.Volumes[0].Tags, []tagInfo{
		{"foo", "bar"},
	})
}

func (s *ebsSuite) TestImportVolumeCredentialError(c *gc.C) {
	vs := s.volumeSource(c, nil)
	c.Assert(vs, gc.Implements, new(storage.VolumeImporter))
	resp, err := s.srv.ec2srv.CreateVolume(s.cloudCallCtx, &awsec2.CreateVolumeInput{
		Size:             aws.Int32(1),
		VolumeType:       "gp2",
		AvailabilityZone: aws.String("us-east-1a"),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.srv.ec2srv.SetAPIError("CreateTags", &smithy.GenericAPIError{Code: "Blocked"})

	_, err = vs.(storage.VolumeImporter).ImportVolume(s.cloudCallCtx, aws.ToString(resp.VolumeId), map[string]string{
		"foo": "bar",
	})
	c.Assert(errors.Is(err, common.ErrorCredentialNotValid), jc.IsTrue)
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
	expect("/dev/sdq", "xvdq")
	expect("/dev/sdr", "xvdr")
	expect("/dev/sds", "xvds")
	expect("/dev/sdt", "xvdt")
	expect("/dev/sdu", "xvdu")
	expect("/dev/sdv", "xvdv")
	expect("/dev/sdw", "xvdw")
	expect("/dev/sdx", "xvdx")
	expect("/dev/sdy", "xvdy")
	expect("/dev/sdz", "xvdz")
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
	expectN("/dev/sdq", "xvdq")
	expectN("/dev/sdr", "xvdr")
	expectN("/dev/sds", "xvds")
	expectN("/dev/sdt", "xvdt")
	expectN("/dev/sdu", "xvdu")
	expectN("/dev/sdv", "xvdv")
	expectN("/dev/sdw", "xvdw")
	expectN("/dev/sdx", "xvdx")
	expectN("/dev/sdy", "xvdy")
	expectN("/dev/sdz", "xvdz")
	expectErr("too many EBS volumes to attach")
}

func (*blockDeviceMappingSuite) TestGetBlockDeviceMappings(c *gc.C) {
	mapping, err := ec2.GetBlockDeviceMappings(constraints.Value{}, "jammy", false, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mapping, gc.DeepEquals, []types.BlockDeviceMapping{{
		Ebs:        &types.EbsBlockDevice{VolumeSize: aws.Int32(8)},
		DeviceName: aws.String("/dev/sda1"),
	}, {
		VirtualName: aws.String("ephemeral0"),
		DeviceName:  aws.String("/dev/sdb"),
	}, {
		VirtualName: aws.String("ephemeral1"),
		DeviceName:  aws.String("/dev/sdc"),
	}, {
		VirtualName: aws.String("ephemeral2"),
		DeviceName:  aws.String("/dev/sdd"),
	}, {
		VirtualName: aws.String("ephemeral3"),
		DeviceName:  aws.String("/dev/sde"),
	}})
}

func (*blockDeviceMappingSuite) TestGetBlockDeviceMappingsController(c *gc.C) {
	mapping, err := ec2.GetBlockDeviceMappings(constraints.Value{}, "jammy", true, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mapping, gc.DeepEquals, []types.BlockDeviceMapping{{
		Ebs:        &types.EbsBlockDevice{VolumeSize: aws.Int32(32)},
		DeviceName: aws.String("/dev/sda1"),
	}, {
		VirtualName: aws.String("ephemeral0"),
		DeviceName:  aws.String("/dev/sdb"),
	}, {
		VirtualName: aws.String("ephemeral1"),
		DeviceName:  aws.String("/dev/sdc"),
	}, {
		VirtualName: aws.String("ephemeral2"),
		DeviceName:  aws.String("/dev/sdd"),
	}, {
		VirtualName: aws.String("ephemeral3"),
		DeviceName:  aws.String("/dev/sde"),
	}})
}

type tagInfo struct {
	key   string
	value string
}

func compareTags(c *gc.C, obtained []types.Tag, expected []tagInfo) {
	got := make([]tagInfo, len(obtained))
	for i, t := range obtained {
		got[i] = tagInfo{
			key:   aws.ToString(t.Key),
			value: aws.ToString(t.Value),
		}
	}
	c.Assert(got, jc.SameContents, expected)
}
