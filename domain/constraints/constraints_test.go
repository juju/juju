// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
)

type constraintsSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&constraintsSuite{})

// TestFromCoreConstraints is concerned with testing the mapping from a
// [constraints.Value] to a [Constraints] object. Specifically the main thing we
// care about in this test is that spaces are either included or excluded
// correctly and that the rest of the values are set verbatim.
func (*constraintsSuite) TestFromCoreConstraints(c *tc.C) {
	tests := []struct {
		Comment string
		In      constraints.Value
		Out     Constraints
	}{
		{
			Comment: "Test every value get's set as described",
			In: constraints.Value{
				Arch:             ptr("test"),
				Container:        ptr(instance.LXD),
				CpuCores:         ptr(uint64(1)),
				CpuPower:         ptr(uint64(1)),
				Mem:              ptr(uint64(1024)),
				RootDisk:         ptr(uint64(100)),
				RootDiskSource:   ptr("source"),
				Tags:             ptr([]string{"tag1", "tag2"}),
				InstanceRole:     ptr("instance-role"),
				InstanceType:     ptr("instance-type"),
				VirtType:         ptr("kvm"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
				ImageID:          ptr("image-123"),
				Spaces:           ptr([]string{"space1", "space2", "^space3"}),
			},
			Out: Constraints{
				Arch:             ptr("test"),
				Container:        ptr(instance.LXD),
				CpuCores:         ptr(uint64(1)),
				CpuPower:         ptr(uint64(1)),
				Mem:              ptr(uint64(1024)),
				RootDisk:         ptr(uint64(100)),
				RootDiskSource:   ptr("source"),
				Tags:             ptr([]string{"tag1", "tag2"}),
				InstanceRole:     ptr("instance-role"),
				InstanceType:     ptr("instance-type"),
				VirtType:         ptr("kvm"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
				ImageID:          ptr("image-123"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
					{SpaceName: "space2", Exclude: false},
					{SpaceName: "space3", Exclude: true},
				}),
			},
		},
		{
			Comment: "Test only excluded spaces",
			In: constraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"^space3"}),
			},
			Out: Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space3", Exclude: true},
				}),
			},
		},
		{
			Comment: "Test only included spaces",
			In: constraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"space3"}),
			},
			Out: Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space3", Exclude: false},
				}),
			},
		},
		{
			Comment: "Test no spaces",
			In: constraints.Value{
				Arch: ptr("test"),
			},
			Out: Constraints{
				Arch: ptr("test"),
			},
		},
	}

	for _, test := range tests {
		rval := DecodeConstraints(test.In)
		c.Check(rval, jc.DeepEquals, test.Out, tc.Commentf(test.Comment))
	}
}

// TestToCoreConstraints is concerned with testing the mapping from a
// [Constraints] object to a [constraints.Value]. Specifically the main thing we
// care about in this test is that spaces are either included or excluded
// correctly and that the rest of the values are set verbatim.
func (*constraintsSuite) TestToCoreConstraints(c *tc.C) {
	tests := []struct {
		Comment string
		Out     constraints.Value
		In      Constraints
	}{
		{
			Comment: "Test every value get's set as described",
			In: Constraints{
				Arch:             ptr("test"),
				Container:        ptr(instance.LXD),
				CpuCores:         ptr(uint64(1)),
				CpuPower:         ptr(uint64(1)),
				Mem:              ptr(uint64(1024)),
				RootDisk:         ptr(uint64(100)),
				RootDiskSource:   ptr("source"),
				Tags:             ptr([]string{"tag1", "tag2"}),
				InstanceRole:     ptr("instance-role"),
				InstanceType:     ptr("instance-type"),
				VirtType:         ptr("kvm"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
				ImageID:          ptr("image-123"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
					{SpaceName: "space2", Exclude: false},
					{SpaceName: "space3", Exclude: true},
				}),
			},
			Out: constraints.Value{
				Arch:             ptr("test"),
				Container:        ptr(instance.LXD),
				CpuCores:         ptr(uint64(1)),
				CpuPower:         ptr(uint64(1)),
				Mem:              ptr(uint64(1024)),
				RootDisk:         ptr(uint64(100)),
				RootDiskSource:   ptr("source"),
				Tags:             ptr([]string{"tag1", "tag2"}),
				InstanceRole:     ptr("instance-role"),
				InstanceType:     ptr("instance-type"),
				VirtType:         ptr("kvm"),
				Zones:            ptr([]string{"zone1", "zone2"}),
				AllocatePublicIP: ptr(true),
				ImageID:          ptr("image-123"),
				Spaces:           ptr([]string{"space1", "space2", "^space3"}),
			},
		},
		{
			Comment: "Test only excluded spaces",
			In: Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space3", Exclude: true},
				}),
			},
			Out: constraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"^space3"}),
			},
		},
		{
			Comment: "Test only included spaces",
			In: Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]SpaceConstraint{
					{SpaceName: "space3", Exclude: false},
				}),
			},
			Out: constraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"space3"}),
			},
		},
		{
			Comment: "Test no spaces",
			In: Constraints{
				Arch: ptr("test"),
			},
			Out: constraints.Value{
				Arch: ptr("test"),
			},
		},
	}

	for _, test := range tests {
		rval := EncodeConstraints(test.In)
		c.Check(rval, jc.DeepEquals, test.Out, tc.Commentf(test.Comment))
	}
}

// ptr returns a reference to a copied value of type T.
func ptr[T any](i T) *T {
	return &i
}
