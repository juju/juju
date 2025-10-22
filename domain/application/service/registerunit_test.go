// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	caas "github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/internal"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

type registerCAASUnitSuite struct {
	baseSuite
}

func TestRegisterCAASUnitSuite(t *testing.T) {
	tc.Run(t, &registerCAASUnitSuite{})
}

func (s *registerCAASUnitSuite) makeStorageArg(
	c *tc.C,
) internal.RegisterUnitStorageArg {
	fsUUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	rval := internal.RegisterUnitStorageArg{
		CreateUnitStorageArg: internal.CreateUnitStorageArg{
			StorageDirectives: []internal.CreateUnitStorageDirectiveArg{
				{
					Count:    1,
					Name:     "st1",
					PoolUUID: storagePoolUUID,
					Size:     1024,
				},
			},
			StorageInstances: []internal.CreateUnitStorageInstanceArg{
				{
					CharmName: "foo",
					Filesystem: &internal.CreateUnitStorageFilesystemArg{
						UUID:           fsUUID,
						ProvisionScope: domainstorageprov.ProvisionScopeModel,
					},
					Kind:            domainstorage.StorageKindFilesystem,
					RequestSizeMiB:  1024,
					StoragePoolUUID: storagePoolUUID,
					UUID:            storageInstUUID,
				},
			},
			StorageToAttach: []internal.CreateUnitStorageAttachmentArg{
				{
					FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
						FilesystemUUID: fsUUID,
						ProvisionScope: domainstorageprov.ProvisionScopeModel,
						UUID:           tc.Must(c, domainstorageprov.NewFilesystemAttachmentUUID),
					},
					UUID:                tc.Must(c, domainstorageprov.NewStorageAttachmentUUID),
					StorageInstanceUUID: storageInstUUID,
				},
			},
			StorageToOwn: []domainstorage.StorageInstanceUUID{storageInstUUID},
		},
		FilesystemProviderIDs: map[domainstorageprov.FilesystemUUID]string{
			fsUUID: "fs-providerid-1",
		},
	}

	return rval
}

func (*registerCAASUnitSuite) storageChecker() *tc.MultiChecker {
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.CreateUnitStorageArg.StorageToAttach[_].FilesystemAttachment.NetNodeUUID`, tc.Ignore)
	return mc
}

// TestRegisterNewCAASUnit tests the happy path of registering a new CAAS unit
// into the model.
func (s *registerCAASUnitSuite) TestRegisterNewCAASUnit(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	storageArg := s.makeStorageArg(c)

	app := NewMockApplication(ctrl)
	app.EXPECT().Units().Return([]caas.Unit{{
		Id:      "foo-666",
		Address: "10.6.6.6",
		Ports:   []string{"8080"},
		FilesystemInfo: []caas.FilesystemInfo{{
			FilesystemId: "fs-providerid-1",
			StorageName:  "st1",
		}},
	}}, nil)
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	s.state.EXPECT().GetCAASUnitRegistered(gomock.Any(), gomock.Any()).Return(
		false, "", "", nil,
	)
	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").
		Return(appUUID, nil)
	s.storageService.EXPECT().MakeRegisterNewCAASUnitStorageArg(
		gomock.Any(), appUUID, gomock.Any(), gomock.Any(),
	).Return(storageArg, nil).AnyTimes()

	arg := application.RegisterCAASUnitArg{
		UnitName:               "foo/666",
		PasswordHash:           "secret",
		ProviderID:             "foo-666",
		Address:                ptr("10.6.6.6"),
		Ports:                  ptr([]string{"8080"}),
		OrderedScale:           true,
		OrderedId:              666,
		RegisterUnitStorageArg: storageArg,
	}

	var gotRCA application.RegisterCAASUnitArg
	s.state.EXPECT().RegisterCAASUnit(
		gomock.Any(), "foo", gomock.Any(),
	).DoAndReturn(func(
		_ context.Context, _ string, rca application.RegisterCAASUnitArg,
	) error {
		gotRCA = rca
		return nil
	})

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
		ProviderID:      "foo-666",
	}
	unitName, password, err := s.service.RegisterCAASUnit(c.Context(), p)
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.PasswordHash`, tc.Ignore)
	mc.AddExpr(`_.NetNodeUUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_.RegisterUnitStorageArg`, s.storageChecker(), tc.ExpectedValue)
	c.Assert(gotRCA, mc, arg)
	c.Assert(unitName.String(), tc.Equals, "foo/666")
	c.Assert(password, tc.Not(tc.Equals), "")
}

// TestRegisterExistingCAASUnit tests the happy path of registering an existing
// CAAS unit into the model. This would be the case where a unit has an un
// expected restart and wishes to drive itself back into the model idempotently.
//
// It is also concievable that a container is changed outside of Juju's preview.
//
// Key observabilities in this test:
// - We want to see that the existing net node uuid for the caas unit is re-used.
func (s *registerCAASUnitSuite) TestRegisterExistingCAASUnit(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	unitNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	storageArg := s.makeStorageArg(c)

	app := NewMockApplication(ctrl)
	app.EXPECT().Units().Return([]caas.Unit{{
		Id:      "foo-666",
		Address: "10.6.6.6",
		Ports:   []string{"8080"},
		FilesystemInfo: []caas.FilesystemInfo{{
			FilesystemId: "fs-providerid-1",
			StorageName:  "st1",
		}},
	}}, nil)
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)
	s.state.EXPECT().GetCAASUnitRegistered(gomock.Any(), gomock.Any()).Return(
		true, unitUUID, unitNetNodeUUID, nil,
	)
	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").
		Return(appUUID, nil)
	s.storageService.EXPECT().MakeRegisterExistingCAASUnitStorageArg(
		gomock.Any(), unitUUID, gomock.Any(), gomock.Any(),
	).Return(storageArg, nil).AnyTimes()

	expectedArg := application.RegisterCAASUnitArg{
		Address:                ptr("10.6.6.6"),
		NetNodeUUID:            unitNetNodeUUID,
		OrderedId:              666,
		OrderedScale:           true,
		PasswordHash:           "secret",
		Ports:                  ptr([]string{"8080"}),
		ProviderID:             "foo-666",
		RegisterUnitStorageArg: storageArg,
		UnitName:               "foo/666",
	}

	var gotRCA application.RegisterCAASUnitArg
	s.state.EXPECT().RegisterCAASUnit(
		gomock.Any(), "foo", gomock.Any(),
	).DoAndReturn(func(
		_ context.Context, _ string, rca application.RegisterCAASUnitArg,
	) error {
		gotRCA = rca
		return nil
	})

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
		ProviderID:      "foo-666",
	}
	unitName, password, err := s.service.RegisterCAASUnit(c.Context(), p)
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.PasswordHash`, tc.Ignore)
	mc.AddExpr(`_.RegisterUnitStorageArg`, s.storageChecker(), tc.ExpectedValue)
	c.Assert(gotRCA, mc, expectedArg)
	c.Assert(unitName.String(), tc.Equals, "foo/666")
	c.Assert(password, tc.Not(tc.Equals), "")
}

// TestRegisterCAASUnitMissingProviderID tests the case where a caas unit is
// attempting registration but no provider id has been supplied.
//
// NOTE(tlm): It is unclear if this test has any value. The API should be
// stopping this at the front door.
func (s *registerCAASUnitSuite) TestRegisterCAASUnitMissingProviderID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
	}
	_, _, err := s.service.RegisterCAASUnit(c.Context(), p)
	c.Assert(err, tc.ErrorMatches, "provider id not valid")
}

// TestRegisterCAASUnitApplicationNoPods tests the case where a caas unit is
// attempting registration but the provider informs us that the container does
// not exist anymore.
//
// NOTE(tlm): It is unclear if this test has any value.
func (s *registerCAASUnitSuite) TestRegisterCAASUnitApplicationNoPods(c *tc.C) {
	ctrl := s.setupMocksWithProvider(c, noProviderError, noProviderError)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)

	app := NewMockApplication(ctrl)
	app.EXPECT().Units().Return([]caas.Unit{}, nil)
	s.caasProvider.EXPECT().Application("foo", caas.DeploymentStateful).Return(app)

	s.state.EXPECT().GetCAASUnitRegistered(gomock.Any(), gomock.Any()).Return(
		false, "", "", nil,
	).AnyTimes()
	s.state.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(
		appUUID, nil,
	).AnyTimes()

	p := application.RegisterCAASUnitParams{
		ApplicationName: "foo",
		ProviderID:      "foo-666",
	}
	_, _, err := s.service.RegisterCAASUnit(c.Context(), p)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}
