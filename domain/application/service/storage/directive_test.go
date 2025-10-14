// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/internal"
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
)

type directiveSuite struct {
	poolProvider *MockStoragePoolProvider
	state        *MockState
}

func TestDirectiveSuite(t *testing.T) {
}

func (s *directiveSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.poolProvider = NewMockStoragePoolProvider(ctrl)
	c.Cleanup(func() {
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

		Expected []application.CreateApplicationStorageDirectiveArg
	}{
		{
			Name:             "no overrides, no charm meta storage, no default provisioners",
			CharmMetaStorage: map[string]internalcharm.Storage{},
			Overrides:        map[string]StorageDirectiveOverride{},
			Expected:         []application.CreateApplicationStorageDirectiveArg{},
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
			Expected: []application.CreateApplicationStorageDirectiveArg{
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

			Expected: []application.CreateApplicationStorageDirectiveArg{
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

			Expected: []application.CreateApplicationStorageDirectiveArg{
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

			Expected: []application.CreateApplicationStorageDirectiveArg{
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
			s.state.EXPECT().GetModelStoragePools(
				gomock.Any(),
			).Return(test.ModelStoragePools, nil).AnyTimes()

			svc := NewService(s.state, s.poolProvider)
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
