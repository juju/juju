// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"testing"

	"github.com/juju/gomaasapi/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/storage"
)

type volumeSuite struct {
	maasSuite
}

func TestVolumeSuite(t *testing.T) {
	tc.Run(t, &volumeSuite{})
}

func (s *volumeSuite) TestBuildMAASVolumeParametersNoVolumes(c *tc.C) {
	vInfo, err := buildMAASVolumeParameters(nil, constraints.Value{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vInfo, tc.HasLen, 0)
}

func (s *volumeSuite) TestBuildMAASVolumeParametersJustRootDisk(c *tc.C) {
	var cons constraints.Value
	rootSize := uint64(20000)
	cons.RootDisk = &rootSize
	vInfo, err := buildMAASVolumeParameters(nil, cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vInfo, tc.DeepEquals, []volumeInfo{
		{"root", 20, nil},
	})
}

func (s *volumeSuite) TestBuildMAASVolumeParametersNoTags(c *tc.C) {
	vInfo, err := buildMAASVolumeParameters(
		[]storage.VolumeParams{
			{
				Provider: storage.ProviderType("maas"),
				Tag:      names.NewVolumeTag("1"),
				Size:     2000000,
			},
		},
		constraints.Value{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vInfo, tc.DeepEquals, []volumeInfo{
		{"root", 0, nil}, //root disk must be first
		{"1", 1954, nil},
	})
}

// TestBuildMAASVolumeParametersWithRootDisk checks that
// [buildMAASVolumeParameters] correctly constructs the right volume parameters
// for the supplied [storage.VolumeParams] and includes the root disk for the
// node that is being acquired.
//
// This test also expects to see that the root disk is the first element of the
// returned slice as required by the MAAS API.
func (s *volumeSuite) TestBuildMAASVolumeParametersWithRootDisk(c *tc.C) {
	var cons constraints.Value
	rootSize := uint64(20000)
	cons.RootDisk = &rootSize
	vInfo, err := buildMAASVolumeParameters(
		[]storage.VolumeParams{
			{
				Provider: storage.ProviderType("maas"),
				Tag:      names.NewVolumeTag("1"),
				Size:     2000000,
			},
		},
		cons,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vInfo, tc.DeepEquals, []volumeInfo{
		{"root", 20, nil}, //root disk must be first
		{"1", 1954, nil},
	})
}

// TestBuildMAASVolumeParametersWithTags checks that [buildMAASVolumeParameters]
// correctly constructs the right volume parameters for the supplied
// [storage.VolumeParams] including tags and includes the root disk for the node
// that is being acquired.
//
// This test also expects to see that the root disk is the first element of the
// returned slice as required by the MAAS API.
func (s *volumeSuite) TestBuildMAASVolumeParametersWithTags(c *tc.C) {
	vInfo, err := buildMAASVolumeParameters(
		[]storage.VolumeParams{
			{
				Provider:   storage.ProviderType("maas"),
				Tag:        names.NewVolumeTag("1"),
				Size:       2000000,
				Attributes: map[string]any{"tags": "tag1,tag2"},
			},
		},
		constraints.Value{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vInfo, tc.DeepEquals, []volumeInfo{
		{"root", 0, nil}, //root disk must be first
		{"1", 1954, []string{"tag1", "tag2"}},
	})
}

// TestBuildMAASVolumeParametersWithUnsupportedProvider tests that calling
// [buildMAASVolumeParameters] with a volume that is using another provider
// other then [maasStorageProviderType] returns a [coreerrors.NotSupported]
// error to the caller.
func (s *volumeSuite) TestBuildMAASVolumeParametersWithUnsupportedProvider(c *tc.C) {
	_, err := buildMAASVolumeParameters(
		[]storage.VolumeParams{
			{
				Provider:   storage.ProviderType("anotherprovider"),
				Tag:        names.NewVolumeTag("1"),
				Size:       2000000,
				Attributes: map[string]any{"tags": "tag1,tag2"},
			},
		},
		constraints.Value{},
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *volumeSuite) TestInstanceVolumesMAAS2(c *tc.C) {
	instance := maasInstance{
		machine: &fakeMachine{},
		constraintMatches: gomaasapi.ConstraintMatches{
			Storage: map[string][]gomaasapi.StorageDevice{
				"root": {&fakeBlockDevice{name: "sda", idPath: "/dev/disk/by-dname/sda", size: 250059350016}},
				"1":    {&fakeBlockDevice{name: "sdb", idPath: "/dev/sdb", size: 500059350016}},
				"2":    {&fakeBlockDevice{name: "sdc", idPath: "/dev/disk/by-id/foo", size: 250362438230}},
				"3": {
					&fakeBlockDevice{name: "sdd", idPath: "/dev/disk/by-dname/sdd", size: 250362438230},
					&fakeBlockDevice{name: "sde", idPath: "/dev/disk/by-dname/sde", size: 250362438230},
				},
				"4": {
					&fakeBlockDevice{name: "sdf", idPath: "/dev/disk/by-id/wwn-drbr", size: 280362438231},
				},
				"5": {
					&fakePartition{name: "sde-part1", path: "/dev/disk/by-dname/sde-part1", size: 280362438231},
				},
				"6": {
					&fakeBlockDevice{name: "sdg", idPath: "/dev/disk/by-dname/sdg", size: 280362438231},
				},
			},
		},
	}
	mTag := names.NewMachineTag("1")
	volumes, attachments, err := instance.volumes(
		c.Context(),
		mTag, []names.VolumeTag{
			names.NewVolumeTag("1"),
			names.NewVolumeTag("2"),
			names.NewVolumeTag("3"),
			names.NewVolumeTag("4"),
			names.NewVolumeTag("5"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	// Expect 4 volumes - root volume is ignored, as are volumes
	// with tags we did not request.
	c.Assert(volumes, tc.HasLen, 5)
	c.Assert(attachments, tc.HasLen, 5)
	c.Check(volumes, tc.SameContents, []storage.Volume{{
		Tag: names.NewVolumeTag("1"),
		VolumeInfo: storage.VolumeInfo{
			VolumeId: "volume-1",
			Size:     476893,
		},
	}, {
		Tag: names.NewVolumeTag("2"),
		VolumeInfo: storage.VolumeInfo{
			VolumeId:   "volume-2",
			Size:       238764,
			HardwareId: "foo",
		},
	}, {
		Tag: names.NewVolumeTag("3"),
		VolumeInfo: storage.VolumeInfo{
			VolumeId: "volume-3",
			Size:     238764,
		},
	}, {
		Tag: names.NewVolumeTag("4"),
		VolumeInfo: storage.VolumeInfo{
			VolumeId: "volume-4",
			Size:     267374,
			WWN:      "drbr",
		},
	}, {
		Tag: names.NewVolumeTag("5"),
		VolumeInfo: storage.VolumeInfo{
			VolumeId: "volume-5",
			Size:     267374,
		},
	}})
	c.Assert(attachments, tc.SameContents, []storage.VolumeAttachment{{
		Volume:  names.NewVolumeTag("1"),
		Machine: mTag,
		VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
			DeviceName: "sdb",
		},
	}, {
		Volume:               names.NewVolumeTag("2"),
		Machine:              mTag,
		VolumeAttachmentInfo: storage.VolumeAttachmentInfo{},
	}, {
		Volume:  names.NewVolumeTag("3"),
		Machine: mTag,
		VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
			DeviceLink: "/dev/disk/by-dname/sdd",
		},
	}, {
		Volume:               names.NewVolumeTag("4"),
		Machine:              mTag,
		VolumeAttachmentInfo: storage.VolumeAttachmentInfo{},
	}, {
		Volume:  names.NewVolumeTag("5"),
		Machine: mTag,
		VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
			DeviceLink: "/dev/disk/by-dname/sde-part1",
		},
	}})
}
