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
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type directiveSuite struct {
	poolProvider *MockStoragePoolProvider
	state        *MockState
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
	// Set of fake values to reference in the sub tests.
	fakeFilesytemPoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	fakeBlockdevicePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

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
			ModelStoragePools: internal.ModelStoragePools{
				FilesystemPoolUUID:  &fakeFilesytemPoolUUID,
				BlockDevicePoolUUID: &fakeBlockdevicePoolUUID,
			},
			Expected: []internal.CreateApplicationStorageDirectiveArg{
				{
					Count:    2,
					Name:     domainstorage.Name("foo"),
					PoolUUID: fakeFilesytemPoolUUID,
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
				BlockDevicePoolUUID: &fakeBlockdevicePoolUUID,
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
				FilesystemPoolUUID:  &fakeFilesytemPoolUUID,
				BlockDevicePoolUUID: &fakeBlockdevicePoolUUID,
			},

			Expected: []internal.CreateApplicationStorageDirectiveArg{
				{
					Count:    2,
					Name:     domainstorage.Name("foo"),
					PoolUUID: fakeBlockdevicePoolUUID,
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
