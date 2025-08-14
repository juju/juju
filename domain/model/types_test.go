// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"testing"

	"github.com/juju/tc"

	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

// ptr returns a reference to a copied value of type T.
func ptr[T any](i T) *T {
	return &i
}

// TestModelCreationArgsValidation is aserting all the validation cases that the
// [GlobalModelCreationArgs.Validate] function checks for.
func (*typesSuite) TestModelCreationArgsValidation(c *tc.C) {
	adminUsers := []coreuser.UUID{usertesting.GenUserUUID(c)}

	tests := []struct {
		Args    GlobalModelCreationArgs
		Name    string
		ErrTest error
	}{
		{
			Name: "Test invalid name",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "",
				Qualifier:   "prod",
				AdminUsers:  adminUsers,
			},
			ErrTest: coreerrors.NotValid,
		},
		{
			Name: "Test invalid qualifier",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Qualifier:   "",
				AdminUsers:  adminUsers,
			},
			ErrTest: coreerrors.NotValid,
		},
		{
			Name: "Test invalid creator",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Qualifier:   "prod",
				AdminUsers:  []coreuser.UUID{""},
			},
			ErrTest: coreerrors.NotValid,
		},
		{
			Name: "Test invalid cloud",
			Args: GlobalModelCreationArgs{
				Cloud:       "",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Qualifier:   "prod",
				AdminUsers:  adminUsers,
			},
			ErrTest: coreerrors.NotValid,
		},
		{
			Name: "Test invalid cloud region",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "",
				Name:        "my-awesome-model",
				Qualifier:   "prod",
				AdminUsers:  adminUsers,
			},
			ErrTest: nil,
		},
		{
			Name: "Test invalid credential key",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: credential.Key{
					Owner: usertesting.GenNewName(c, "wallyworld"),
				},
				Name:       "my-awesome-model",
				Qualifier:  "prod",
				AdminUsers: adminUsers,
			},
			ErrTest: coreerrors.NotValid,
		},
		{
			Name: "Test happy path without credential key",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "my-awesome-model",
				Qualifier:   "prod",
				AdminUsers:  adminUsers,
			},
			ErrTest: nil,
		},
		{
			Name: "Test happy path with credential key",
			Args: GlobalModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Credential: credential.Key{
					Cloud: "cloud",
					Owner: usertesting.GenNewName(c, "wallyworld"),
					Name:  "mycred",
				},
				Name:       "my-awesome-model",
				Qualifier:  "prod",
				AdminUsers: adminUsers,
			},
			ErrTest: nil,
		},
	}

	for i, test := range tests {
		c.Logf("testing %q: %d %v", test.Name, i, test.Args)

		err := test.Args.Validate()
		if test.ErrTest == nil {
			c.Check(err, tc.ErrorIsNil, tc.Commentf("%s", test.Name))
		} else {
			c.Check(err, tc.ErrorIs, test.ErrTest, tc.Commentf("%s", test.Name))
		}
	}
}

// TestModelImportArgsValidation is aserting all the validation cases that the
// [ModelImportArgs.Validate] function checks for.
func (*typesSuite) TestModelImportArgsValidation(c *tc.C) {
	adminUsers := []coreuser.UUID{usertesting.GenUserUUID(c)}

	tests := []struct {
		Args    ModelImportArgs
		Name    string
		ErrTest error
	}{
		{
			Name: "Test happy path with valid model id",
			Args: ModelImportArgs{
				GlobalModelCreationArgs: GlobalModelCreationArgs{
					Cloud:       "my-cloud",
					CloudRegion: "my-region",
					Credential: credential.Key{
						Cloud: "cloud",
						Owner: usertesting.GenNewName(c, "wallyworld"),
						Name:  "mycred",
					},
					Name:       "my-awesome-model",
					Qualifier:  "prod",
					AdminUsers: adminUsers,
				},
				UUID: coremodel.GenUUID(c),
			},
		},
		{
			Name: "Test invalid model id",
			Args: ModelImportArgs{
				GlobalModelCreationArgs: GlobalModelCreationArgs{
					Cloud:       "my-cloud",
					CloudRegion: "my-region",
					Credential: credential.Key{
						Cloud: "cloud",
						Owner: usertesting.GenNewName(c, "wallyworld"),
						Name:  "mycred",
					},
					Name:       "my-awesome-model",
					Qualifier:  "prod",
					AdminUsers: adminUsers,
				},
				UUID: "not valid",
			},
			ErrTest: coreerrors.NotValid,
		},
	}

	for i, test := range tests {
		c.Logf("testing %q: %d %v", test.Name, i, test.Args)

		err := test.Args.Validate()
		if test.ErrTest == nil {
			c.Check(err, tc.ErrorIsNil, tc.Commentf("%s", test.Name))
		} else {
			c.Check(err, tc.ErrorIs, test.ErrTest, tc.Commentf("%s", test.Name))
		}
	}
}

// TestFromCoreConstraints is concerned with testing the mapping from a
// [coreconstraints.Value] to a [Constraints] object. Specifically the main thing we
// care about in this test is that spaces are either included or excluded
// correctly and that the rest of the values are set verbatim.
func (*typesSuite) TestFromCoreConstraints(c *tc.C) {
	tests := []struct {
		Comment string
		In      coreconstraints.Value
		Out     constraints.Constraints
	}{
		{
			Comment: "Test every value get's set as described",
			In: coreconstraints.Value{
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
			Out: constraints.Constraints{
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
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
					{SpaceName: "space2", Exclude: false},
					{SpaceName: "space3", Exclude: true},
				}),
			},
		},
		{
			Comment: "Test only excluded spaces",
			In: coreconstraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"^space3"}),
			},
			Out: constraints.Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space3", Exclude: true},
				}),
			},
		},
		{
			Comment: "Test only included spaces",
			In: coreconstraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"space3"}),
			},
			Out: constraints.Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space3", Exclude: false},
				}),
			},
		},
		{
			Comment: "Test no spaces",
			In: coreconstraints.Value{
				Arch: ptr("test"),
			},
			Out: constraints.Constraints{
				Arch: ptr("test"),
			},
		},
	}

	for _, test := range tests {
		rval := constraints.DecodeConstraints(test.In)
		c.Check(rval, tc.DeepEquals, test.Out, tc.Commentf(test.Comment))
	}
}

// TestToCoreConstraints is concerned with testing the mapping from a
// [Constraints] object to a [coreconstraints.Value]. Specifically the main thing we
// care about in this test is that spaces are either included or excluded
// correctly and that the rest of the values are set verbatim.
func (*typesSuite) TestToCoreConstraints(c *tc.C) {
	tests := []struct {
		Comment string
		Out     coreconstraints.Value
		In      constraints.Constraints
	}{
		{
			Comment: "Test every value get's set as described",
			In: constraints.Constraints{
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
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
					{SpaceName: "space2", Exclude: false},
					{SpaceName: "space3", Exclude: true},
				}),
			},
			Out: coreconstraints.Value{
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
			In: constraints.Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space3", Exclude: true},
				}),
			},
			Out: coreconstraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"^space3"}),
			},
		},
		{
			Comment: "Test only included spaces",
			In: constraints.Constraints{
				Arch: ptr("test"),
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space3", Exclude: false},
				}),
			},
			Out: coreconstraints.Value{
				Arch:   ptr("test"),
				Spaces: ptr([]string{"space3"}),
			},
		},
		{
			Comment: "Test no spaces",
			In: constraints.Constraints{
				Arch: ptr("test"),
			},
			Out: coreconstraints.Value{
				Arch: ptr("test"),
			},
		},
	}

	for _, test := range tests {
		rval := constraints.EncodeConstraints(test.In)
		c.Check(rval, tc.DeepEquals, test.Out, tc.Commentf(test.Comment))
	}
}
