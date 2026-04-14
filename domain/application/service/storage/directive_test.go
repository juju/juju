// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

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

		Expected []domainstorage.DirectiveArg
	}{
		{
			Name:             "no overrides, no charm meta storage, no default provisioners",
			CharmMetaStorage: map[string]internalcharm.Storage{},
			Overrides:        map[string]StorageDirectiveOverride{},
			Expected:         []domainstorage.DirectiveArg{},
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
			Expected: []domainstorage.DirectiveArg{
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

			Expected: []domainstorage.DirectiveArg{
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

			Expected: []domainstorage.DirectiveArg{
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

			Expected: []domainstorage.DirectiveArg{
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

	charmStorageDefs := map[string]internal.CharmStorageDefinitionForValidation{
		"st1": {
			CountMin:    0,
			CountMax:    -1, // -1 indicates no max limit
			Name:        "st1",
			MinimumSize: 1024,
			Type:        domainapplicationcharm.StorageBlock,
		},
	}

	overrides := map[string]StorageDirectiveOverride{
		"st1": {
			Count: new(uint32(5)),
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

	charmStorageDefs := map[string]internal.CharmStorageDefinitionForValidation{
		"st1": {
			CountMin:    0,
			CountMax:    2,
			Name:        "st1",
			MinimumSize: 1024,
			Type:        domainapplicationcharm.StorageBlock,
		},
	}

	overrides := map[string]StorageDirectiveOverride{
		"st1": {
			Count: new(uint32(3)),
		},
	}

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))
	err := svc.ValidateApplicationStorageDirectiveOverrides(
		c.Context(), charmStorageDefs, overrides,
	)

	errVal, is := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
	c.Check(is, tc.IsTrue)
	c.Check(errVal, tc.DeepEquals, applicationerrors.StorageCountLimitExceeded{
		Maximum:     new(2),
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
	createArgs := []domainstorage.DirectiveArg{
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

	expected := []internal.StorageDirective{
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
			MaxCount:          domainapplicationcharm.StorageNoMaxCount,
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

// TestReconcileStorageDirectiveAgainstCharmStorageNoChanges tests that when existing
// storage directives match new charm storage exactly, no changes are proposed.
func (s *directiveSuite) TestReconcileStorageDirectiveAgainstCharmStorageNoChanges(c *tc.C) {
	defer s.setupMocks(c.T).Finish()
	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	existingStorageDirectives := []internal.StorageDirective{
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

	modelStoragePools := makeModelStoragePools(c)
	s.state.EXPECT().GetModelStoragePools(gomock.Any()).Return(modelStoragePools, nil)

	toCreate, toUpdate, err := svc.ReconcileStorageDirectivesAgainstCharmStorage(
		c.Context(), existingStorageDirectives, newCharmStorages,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toCreate, tc.HasLen, 0)

	// There should be an update to refresh the charmID even no directive value has changed.
	c.Assert(toUpdate, tc.SameContents, []domainstorage.DirectiveArg{
		{
			Name:  "data",
			Size:  1024,
			Count: 1,
		},
	})
}

// TestReconcileStorageDirectiveAgainstCharmStorageIncreaseSize tests that when new
// charm storage minimum size is higher than existing, the size is increased.
func (s *directiveSuite) TestReconcileStorageDirectiveAgainstCharmStorageIncreaseSize(c *tc.C) {
	defer s.setupMocks(c.T).Finish()
	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	existingStorageDirectives := []internal.StorageDirective{
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

	modelStoragePools := makeModelStoragePools(c)
	s.state.EXPECT().GetModelStoragePools(gomock.Any()).Return(modelStoragePools, nil)

	toCreate, toUpdate, err := svc.ReconcileStorageDirectivesAgainstCharmStorage(
		c.Context(), existingStorageDirectives, newCharmStorages,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toCreate, tc.HasLen, 0)
	c.Assert(toUpdate, tc.SameContents, []domainstorage.DirectiveArg{
		{
			Name:  "data",
			Size:  2048,
			Count: 1,
		},
	})
}

// TestReconcileStorageDirectiveAgainstCharmStorageNoSizeChange tests that when existing
// storage size is already above new charm minimum, no size change occurs. However,
// there will still be an update to the charmID.
func (s *directiveSuite) TestReconcileStorageDirectiveAgainstCharmStorageNoSizeChange(c *tc.C) {
	defer s.setupMocks(c.T).Finish()
	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	existingStorageDirectives := []internal.StorageDirective{
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

	modelStoragePools := makeModelStoragePools(c)
	s.state.EXPECT().GetModelStoragePools(gomock.Any()).Return(modelStoragePools, nil)

	toCreate, toUpdate, err := svc.ReconcileStorageDirectivesAgainstCharmStorage(
		c.Context(), existingStorageDirectives, newCharmStorages,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toCreate, tc.HasLen, 0)

	// There should be an update to refresh the charmID even no directive value has changed.
	c.Assert(toUpdate, tc.SameContents, []domainstorage.DirectiveArg{
		{
			Name:  "data",
			Size:  4096,
			Count: 1,
		},
	})
}

// TestReconcileStorageDirectiveAgainstCharmStorageIncreaseCount tests that when new
// charm storage minimum count is higher than existing, the count is increased.
func (s *directiveSuite) TestReconcileStorageDirectiveAgainstCharmStorageIncreaseCount(c *tc.C) {
	defer s.setupMocks(c.T).Finish()
	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	existingStorageDirectives := []internal.StorageDirective{
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

	modelStoragePools := makeModelStoragePools(c)
	s.state.EXPECT().GetModelStoragePools(gomock.Any()).Return(modelStoragePools, nil)

	toCreate, toUpdate, err := svc.ReconcileStorageDirectivesAgainstCharmStorage(
		c.Context(), existingStorageDirectives, newCharmStorages,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toCreate, tc.HasLen, 0)
	c.Assert(toUpdate, tc.SameContents, []domainstorage.DirectiveArg{
		{
			Name:  "data",
			Size:  1024,
			Count: 3,
		},
	})
}

// TestReconcileStorageDirectiveAgainstCharmStorageDecreaseCount tests that when new
// charm storage maximum count is lower than existing, the count is decreased.
func (s *directiveSuite) TestReconcileStorageDirectiveAgainstCharmStorageDecreaseCount(c *tc.C) {
	defer s.setupMocks(c.T).Finish()
	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	existingStorageDirectives := []internal.StorageDirective{
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

	modelStoragePools := makeModelStoragePools(c)
	s.state.EXPECT().GetModelStoragePools(gomock.Any()).Return(modelStoragePools, nil)

	toCreate, toUpdate, err := svc.ReconcileStorageDirectivesAgainstCharmStorage(
		c.Context(), existingStorageDirectives, newCharmStorages,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toCreate, tc.HasLen, 0)
	c.Assert(toUpdate, tc.SameContents, []domainstorage.DirectiveArg{
		{
			Name:  "data",
			Size:  1024,
			Count: 4,
		},
	})
}

// TestReconcileStorageDirectiveAgainstCharmStorageAddNewStorage tests that storage
// present in new charm but not in existing directives is added.
func (s *directiveSuite) TestReconcileStorageDirectiveAgainstCharmStorageAddNewStorage(c *tc.C) {
	defer s.setupMocks(c.T).Finish()
	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	existingStorageDirectives := []internal.StorageDirective{
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

	modelStoragePools := makeModelStoragePools(c)
	s.state.EXPECT().GetModelStoragePools(gomock.Any()).Return(modelStoragePools, nil)

	toCreate, toUpdate, err := svc.ReconcileStorageDirectivesAgainstCharmStorage(
		c.Context(), existingStorageDirectives, newCharmStorages,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toCreate, tc.SameContents, []domainstorage.DirectiveArg{
		{
			Name:     domainstorage.Name("logs"),
			Count:    2,
			Size:     512,
			PoolUUID: *modelStoragePools.FilesystemPoolUUID,
		},
	})

	// Update of charmID even though no value changed.
	c.Assert(toUpdate, tc.HasLen, 1)
}

// TestReconcileStorageDirectiveAgainstCharmStorageComplexReconciliation tests a
// combination of operations: update storage count and size and add new storage.
func (s *directiveSuite) TestReconcileStorageDirectiveAgainstCharmStorageComplexReconciliation(c *tc.C) {
	defer s.setupMocks(c.T).Finish()
	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	existingStorageDirectives := []internal.StorageDirective{
		{
			CharmStorageType: domainapplicationcharm.StorageBlock,
			Name:             "data",
			Count:            1,
			Size:             512,
			PoolUUID:         poolUUID,
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

	modelStoragePools := makeModelStoragePools(c)
	s.state.EXPECT().GetModelStoragePools(gomock.Any()).Return(modelStoragePools, nil)

	toCreate, toUpdate, err := svc.ReconcileStorageDirectivesAgainstCharmStorage(
		c.Context(), existingStorageDirectives, newCharmStorages,
	)

	c.Assert(err, tc.IsNil)
	c.Assert(toCreate, tc.SameContents, []domainstorage.DirectiveArg{
		{
			Name:     domainstorage.Name("logs"),
			Count:    1,
			Size:     512,
			PoolUUID: *modelStoragePools.FilesystemPoolUUID,
		},
	})
	c.Assert(toUpdate, tc.SameContents, []domainstorage.DirectiveArg{
		{
			Name:     "data",
			Count:    2,
			Size:     1024,
			PoolUUID: poolUUID,
		},
	})
}
