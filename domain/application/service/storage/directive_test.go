// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	domainapplication "github.com/juju/juju/domain/application"
	domainapplicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type directiveSuite struct {
	poolProvider *MockStoragePoolProvider
	state        *MockState
}

func makeModelStoragePools(c *tc.C) internal.ModelStoragePools {
	fakeFilesytemPoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	fakeBlockdevicePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	return internal.ModelStoragePools{
		FilesystemPoolUUID:  &fakeFilesytemPoolUUID,
		BlockDevicePoolUUID: &fakeBlockdevicePoolUUID,
	}
}

func TestDirectiveSuite(t *testing.T) {
	tc.Run(t, &directiveSuite{})
}

func (s *directiveSuite) setupMocks(t *testing.T) *gomock.Controller {
	ctrl := gomock.NewController(t)
	s.state = NewMockState(ctrl)
	s.poolProvider = NewMockStoragePoolProvider(ctrl)
	t.Cleanup(func() {
		s.state = nil
		s.poolProvider = nil
	})
	return ctrl
}

// TestMakeApplicationStorageDirectiveArgs tests the expected merges performed
// by [Service.MakeApplicationStorageDirectiveArgs].
func (s *directiveSuite) TestMakeApplicationStorageDirectiveArgs(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)
	tests := []struct {
		Name              string
		ModelStoragePools internal.ModelStoragePools
		CharmMetaStorage  map[string]internalcharm.Storage
		Overrides         map[string]StorageDirectiveOverride

		Expected []internal.CreateApplicationStorageDirectiveArg
	}{
		{
			Name:             "no overrides, no charm meta storage, no default provisioners",
			CharmMetaStorage: map[string]internalcharm.Storage{},
			Overrides:        map[string]StorageDirectiveOverride{},
			Expected:         []internal.CreateApplicationStorageDirectiveArg{},
		},
		{
			// Check to see that the correct provisioner is chosen (filesystem)
			// and pool is picked before provider type.
			Name: "sets default filesystem pool provisioner",
			CharmMetaStorage: map[string]internalcharm.Storage{
				"foo": {
					Name:        "foo",
					Type:        internalcharm.StorageFilesystem,
					CountMin:    2,
					CountMax:    10,
					MinimumSize: 256,
				},
			},
			ModelStoragePools: modelStoragePools,
			Expected: []internal.CreateApplicationStorageDirectiveArg{
				{
					Count:    2,
					Name:     domainstorage.Name("foo"),
					PoolUUID: *modelStoragePools.FilesystemPoolUUID,
					Size:     256,
				},
			},
		},
		{
			// Check to see that the correct provisioner is chosen (filesystem)
			// and provider type is used.
			Name: "sets default filesystem provider provisioner",
			CharmMetaStorage: map[string]internalcharm.Storage{
				"foo": {
					Name:        "foo",
					Type:        internalcharm.StorageFilesystem,
					CountMin:    2,
					CountMax:    10,
					MinimumSize: 256,
				},
			},
			ModelStoragePools: internal.ModelStoragePools{
				BlockDevicePoolUUID: modelStoragePools.BlockDevicePoolUUID,
			},

			Expected: []internal.CreateApplicationStorageDirectiveArg{
				{
					Count: 2,
					Name:  domainstorage.Name("foo"),
					Size:  256,
				},
			},
		},
		{
			// Check to see that the correct provisioner is chosen (blockdevice)
			// and pool is picked before provider type.
			Name: "sets default blockdevice pool provisioner",
			CharmMetaStorage: map[string]internalcharm.Storage{
				"foo": {
					Name:        "foo",
					Type:        internalcharm.StorageBlock,
					CountMin:    2,
					CountMax:    10,
					MinimumSize: 256,
				},
			},
			ModelStoragePools: internal.ModelStoragePools{
				FilesystemPoolUUID:  modelStoragePools.FilesystemPoolUUID,
				BlockDevicePoolUUID: modelStoragePools.BlockDevicePoolUUID,
			},

			Expected: []internal.CreateApplicationStorageDirectiveArg{
				{
					Count:    2,
					Name:     domainstorage.Name("foo"),
					PoolUUID: *modelStoragePools.BlockDevicePoolUUID,
					Size:     256,
				},
			},
		},
		{
			// Check to see that the correct provisioner is chosen (blockdevice)
			// and provider type is used.
			Name: "sets default blockdevice provider provisioner",
			CharmMetaStorage: map[string]internalcharm.Storage{
				"foo": {
					Name:        "foo",
					Type:        internalcharm.StorageBlock,
					CountMin:    2,
					CountMax:    10,
					MinimumSize: 256,
				},
			},
			ModelStoragePools: internal.ModelStoragePools{},

			Expected: []internal.CreateApplicationStorageDirectiveArg{
				{
					Count: 2,
					Name:  domainstorage.Name("foo"),
					Size:  256,
				},
			},
		},
	}

	for _, test := range tests {
		c.Run(test.Name, func(t *testing.T) {
			defer s.setupMocks(t).Finish()

			s.state.EXPECT().GetModelStoragePools(
				gomock.Any(),
			).Return(test.ModelStoragePools, nil).AnyTimes()

			svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))
			args, err := svc.MakeApplicationStorageDirectiveArgs(
				t.Context(),
				test.Overrides,
				test.CharmMetaStorage,
			)
			tc.Check(t, err, tc.ErrorIsNil)
			tc.Check(t, args, tc.DeepEquals, test.Expected)
		})
	}
}

// TestValidateApplicationStorageDirectiveOverridesNoMinLimit is a regression
// test to make sure that when the charm has not specified any limit on storage
// count the caller is free to provide a count to their liking.
func (s *directiveSuite) TestValidateApplicationStorageDirectiveOverridesNoMaxLimit(c *tc.C) {
	defer s.setupMocks(c.T).Finish()

	charmStorageDefs := map[string]internalcharm.Storage{
		"st1": {
			CountMin:    0,
			CountMax:    -1, // -1 indicates no max limit
			Description: "",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        internalcharm.StorageBlock,
		},
	}

	overrides := map[string]StorageDirectiveOverride{
		"st1": {
			Count: ptr(uint32(5)),
		},
	}

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))
	err := svc.ValidateApplicationStorageDirectiveOverrides(
		c.Context(), charmStorageDefs, overrides,
	)
	tc.Check(c, err, tc.ErrorIsNil)
}

func (s *directiveSuite) TestGetApplicationStorageDirectivesInfo(c *tc.C) {
	defer s.setupMocks(c.T).Finish()

	ctx := c.Context()
	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	// Test it rejects a bad UUID. test
	_, err := svc.GetApplicationStorageDirectivesInfo(ctx, "invalid-uuid")
	tc.Check(c, err, tc.ErrorIs, coreerrors.NotValid)

	// Test happy path.
	appUUID, err := coreapplication.NewUUID()
	tc.Check(c, err, tc.ErrorIsNil)

	expectedDirectivesInfo := make(map[string]domainapplication.ApplicationStorageInfo)
	expectedDirectivesInfo["data"] = domainapplication.ApplicationStorageInfo{
		Count:           2,
		SizeMiB:         10240,
		StoragePoolName: "my-storage-pool",
	}
	s.state.EXPECT().GetApplicationStorageDirectivesInfo(ctx, appUUID).Return(expectedDirectivesInfo, nil)

	directivesInfo, err := svc.GetApplicationStorageDirectivesInfo(ctx, appUUID)
	tc.Check(c, err, tc.ErrorIsNil)

	c.Check(directivesInfo, tc.DeepEquals, expectedDirectivesInfo)
}

// TestValidateApplicationStorageDirectiveOverridesExceedMax tests that when a
// a storage override requests more storage then the charm supports the caller
// gets back an error satisfying [applicationerrors.StorageCountLimitExceeded].
func (s *directiveSuite) TestValidateApplicationStorageDirectiveOverridesExceedMax(c *tc.C) {
	defer s.setupMocks(c.T).Finish()

	charmStorageDefs := map[string]internalcharm.Storage{
		"st1": {
			CountMin:    0,
			CountMax:    2,
			Description: "",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        internalcharm.StorageBlock,
		},
	}

	overrides := map[string]StorageDirectiveOverride{
		"st1": {
			Count: ptr(uint32(3)),
		},
	}

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))
	err := svc.ValidateApplicationStorageDirectiveOverrides(
		c.Context(), charmStorageDefs, overrides,
	)

	errVal, is := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
	c.Check(is, tc.IsTrue)
	c.Check(errVal, tc.DeepEquals, applicationerrors.StorageCountLimitExceeded{
		Maximum:     ptr(2),
		Minimum:     0,
		Requested:   3,
		StorageName: "st1",
	})
}

// TestMakeStorageDirectiveFromApplicationArg tests the happy path of
// transferring set of application create storage directive args to
// [domainapplication.StorageDirective] args.
func (s *directiveSuite) TestMakeStorageDirectiveFromApplicationArg(c *tc.C) {
	defer s.setupMocks(c.T).Finish()

	charmStorage := map[string]internalcharm.Storage{
		"data1": {
			CountMax: 20,
			Name:     "data1",
			Type:     internalcharm.StorageFilesystem,
		},
		"data2": {
			CountMax: -1,
			Name:     "data2",
			Type:     internalcharm.StorageBlock,
		},
	}

	poolUUID1 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	poolUUID2 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	createArgs := []internal.CreateApplicationStorageDirectiveArg{
		{
			Count:    3,
			Name:     domainstorage.Name("data1"),
			PoolUUID: poolUUID1,
			Size:     1022,
		},
		{
			Count:    3,
			Name:     domainstorage.Name("data2"),
			PoolUUID: poolUUID2,
			Size:     1011,
		},
	}

	expected := []domainapplication.StorageDirective{
		{
			CharmMetadataName: "kratos",
			CharmStorageType:  domainapplicationcharm.StorageFilesystem,
			Count:             3,
			MaxCount:          20,
			Name:              "data1",
			PoolUUID:          poolUUID1,
			Size:              1022,
		},
		{
			CharmMetadataName: "kratos",
			CharmStorageType:  domainapplicationcharm.StorageBlock,
			Count:             3,
			MaxCount:          domainapplication.StorageDirectiveNoMaxCount,
			Name:              "data2",
			PoolUUID:          poolUUID2,
			Size:              1011,
		},
	}

	gotDirectives := MakeStorageDirectiveFromApplicationArg(
		"kratos", charmStorage, createArgs,
	)
	c.Assert(gotDirectives, tc.SameContents, expected)
}

// TestReconcileUpdatedCharmStorageDirectiveNoChanges tests that when existing
// storage directives match new charm storage exactly, no changes are proposed.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveNoChanges(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)
	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageFilesystem,
			Name:             "data",
			Count:            1,
			Size:             1024,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toApply, tc.HasLen, 0)
	c.Assert(toDelete, tc.HasLen, 0)
}

// TestReconcileUpdatedCharmStorageDirectiveIncreaseSize tests that when new
// charm storage minimum size is higher than existing, the size is increased.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveIncreaseSize(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)
	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             1024,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 2048,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toApply, tc.HasLen, 1)
	c.Assert(toApply[0].Size, tc.Equals, uint64(2048))
	c.Assert(toApply[0].Name, tc.Equals, domainstorage.Name("data"))
	c.Assert(toDelete, tc.HasLen, 0)
}

// TestReconcileUpdatedCharmStorageDirectiveNoSizeChange tests that when existing
// storage size is already above new charm minimum, no size change occurs.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveNoSizeChange(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)
	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             4096,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageBlock,
			CountMin:    1,
			CountMax:    5,
			MinimumSize: 2048,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toApply, tc.HasLen, 0)
	c.Assert(toDelete, tc.HasLen, 0)
}

// TestReconcileUpdatedCharmStorageDirectiveIncreaseCount tests that when new
// charm storage minimum count is higher than existing, the count is increased.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveIncreaseCount(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)
	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             1024,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageBlock,
			CountMin:    3,
			CountMax:    5,
			MinimumSize: 1024,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toApply, tc.HasLen, 1)
	c.Assert(toApply[0].Count, tc.Equals, uint32(3))
	c.Assert(toApply[0].Name, tc.Equals, domainstorage.Name("data"))
	c.Assert(toDelete, tc.HasLen, 0)
}

// TestReconcileUpdatedCharmStorageDirectiveDecreaseCount tests that when new
// charm storage maximum count is lower than existing, the count is decreased.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveDecreaseCount(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)
	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "data",
			Count:            5,
			Size:             1024,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageBlock,
			CountMin:    1,
			CountMax:    4,
			MinimumSize: 1024,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toApply, tc.HasLen, 1)
	c.Assert(toApply[0].Count, tc.Equals, uint32(4))
	c.Assert(toApply[0].Name, tc.Equals, domainstorage.Name("data"))
	c.Assert(toDelete, tc.HasLen, 0)
}

// TestReconcileUpdatedCharmStorageDirectiveDeleteOldStorage tests that storage
// present in existing directives but not in new charm is marked for deletion.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveDeleteOldStorage(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)
	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             1024,
		},
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "logs",
			Count:            1,
			Size:             512,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toApply, tc.HasLen, 0)
	c.Assert(toDelete, tc.HasLen, 1)
	c.Assert(toDelete[0], tc.Equals, "logs")
}

// TestReconcileUpdatedCharmStorageDirectiveAddNewStorage tests that storage
// present in new charm but not in existing directives is added.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveAddNewStorage(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)

	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             1024,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageBlock,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
		"logs": {
			Type:        internalcharm.StorageFilesystem,
			CountMin:    2,
			CountMax:    5,
			MinimumSize: 512,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toApply, tc.HasLen, 1)
	c.Assert(string(toApply[0].Name), tc.Equals, "logs")
	c.Assert(toApply[0].Count, tc.Equals, uint32(2))
	c.Assert(toApply[0].Size, tc.Equals, uint64(512))
	c.Assert(toApply[0].PoolUUID, tc.Equals, *modelStoragePools.FilesystemPoolUUID)
	c.Assert(toDelete, tc.HasLen, 0)
}

// TestReconcileUpdatedCharmStorageDirectiveIncompatibleType tests that when
// storage type changes between existing and new charm, an error is returned.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveIncompatibleType(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)
	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			Name:             "data",
			Count:            1,
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Size:             1024,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    1,
			MinimumSize: 1024,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(toApply, tc.IsNil)
	c.Assert(toDelete, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `.*existing storage "data" type changed from "block" to "filesystem".*`)
	var incompatibleErr *applicationerrors.CharmStorageTypeChanged
	c.Assert(errors.As(err, &incompatibleErr), tc.Equals, true)
	c.Assert(incompatibleErr.StorageName, tc.Equals, "data")
	c.Assert(incompatibleErr.OldType, tc.Equals, "block")
	c.Assert(incompatibleErr.NewType, tc.Equals, "filesystem")
}

// TestReconcileUpdatedCharmStorageDirectiveComplexReconciliation tests a
// combination of operations: update storage count and size, delete old storage,
// and add new storage.
func (s *directiveSuite) TestReconcileUpdatedCharmStorageDirectiveComplexReconciliation(c *tc.C) {
	modelStoragePools := makeModelStoragePools(c)

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	existingStorageDirectives := []domainapplication.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         poolUUID,
		},
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "cache",
			Count:            1,
			Size:             256,
		},
	}

	newCharmStorages := map[string]internalcharm.Storage{
		"data": {
			Type:        internalcharm.StorageBlock,
			CountMin:    2,
			CountMax:    5,
			MinimumSize: 1024,
		},
		"logs": {
			Type:        internalcharm.StorageFilesystem,
			CountMin:    1,
			CountMax:    3,
			MinimumSize: 512,
		},
	}

	toApply, toDelete, err := ReconcileUpdatedCharmStorageDirective(
		newCharmStorages, existingStorageDirectives, modelStoragePools,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toApply, tc.HasLen, 2)
	c.Assert(toDelete, tc.HasLen, 1)
	c.Assert(toDelete[0], tc.Equals, "cache")

	// Check for updated "data" storage
	var dataUpdate, logsCreate *internal.ApplyApplicationStorageDirectiveArg
	for i := range toApply {
		if string(toApply[i].Name) == "data" {
			dataUpdate = &toApply[i]
		} else if string(toApply[i].Name) == "logs" {
			logsCreate = &toApply[i]
		}
	}

	c.Assert(dataUpdate, tc.Not(tc.IsNil))
	c.Assert(dataUpdate.Count, tc.Equals, uint32(2))
	c.Assert(dataUpdate.Size, tc.Equals, uint64(1024))
	c.Assert(dataUpdate.PoolUUID, tc.Equals, poolUUID)

	c.Assert(logsCreate, tc.Not(tc.IsNil))
	c.Assert(logsCreate.Count, tc.Equals, uint32(1))
	c.Assert(logsCreate.Size, tc.Equals, uint64(512))
	c.Assert(logsCreate.PoolUUID, tc.Equals, *modelStoragePools.FilesystemPoolUUID)
}
