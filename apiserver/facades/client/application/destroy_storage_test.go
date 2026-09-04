// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/application"
	coreunit "github.com/juju/juju/core/unit"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/removal"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/rpc/params"
)

// expectDestroyCharm resolves the charm of the "foo" application through the
// expectations required by [APIBase.DestroyUnit] and
// [APIBase.DestroyApplication].
func (s *applicationSuite) expectDestroyCharm(c *tc.C) {
	charmLocator := applicationcharm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "foo").Return(charmLocator, nil)
	s.applicationService.EXPECT().GetCharmMetadataName(gomock.Any(), charmLocator).Return("foo", nil)
}

func (s *applicationSuite) newStorageInstance(c *tc.C, id string, persistent bool) domainstorage.StorageInstanceClassification {
	return domainstorage.StorageInstanceClassification{
		ID:         id,
		Persistent: persistent,
		UUID:       tc.Must(c, domainstorage.NewStorageInstanceUUID),
	}
}

// TestDestroyUnitClassifiesStorage asserts that destroying an IAAS unit
// reports its non persistent storage as destroyed and its persistent storage
// as detached.
func (s *applicationSuite) TestDestroyUnitClassifiesStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	nonPersistent := s.newStorageInstance(c, "single-fs/0", false)
	persistent := s.newStorageInstance(c, "single-blk/0", true)

	s.applicationService.EXPECT().IsSubordinateApplicationByName(gomock.Any(), "foo").Return(false, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID, nil)
	s.storageService.EXPECT().GetStorageClassificationForUnits(gomock.Any(), []coreunit.UUID{unitUUID}).Return(
		map[coreunit.UUID][]domainstorage.StorageInstanceClassification{
			unitUUID: {nonPersistent, persistent},
		}, nil,
	)
	removalUUID := tc.Must(c, removal.NewUUID)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, false, false, time.Duration(0)).Return(removalUUID, nil)

	res, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag("foo/0").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Info, tc.DeepEquals, &params.DestroyUnitInfo{
		DestroyedStorage: []params.Entity{{Tag: "storage-single-fs-0"}},
		DetachedStorage:  []params.Entity{{Tag: "storage-single-blk-0"}},
	})
}

// TestDestroyUnitDestroyStorage asserts that destroying an IAAS unit with
// destroy storage reports all of its storage as destroyed.
func (s *applicationSuite) TestDestroyUnitDestroyStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	nonPersistent := s.newStorageInstance(c, "single-fs/0", false)
	persistent := s.newStorageInstance(c, "single-blk/0", true)

	s.applicationService.EXPECT().IsSubordinateApplicationByName(gomock.Any(), "foo").Return(false, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID, nil)
	s.storageService.EXPECT().GetStorageClassificationForUnits(gomock.Any(), []coreunit.UUID{unitUUID}).Return(
		map[coreunit.UUID][]domainstorage.StorageInstanceClassification{
			unitUUID: {persistent, nonPersistent},
		}, nil,
	)
	removalUUID := tc.Must(c, removal.NewUUID)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, true, false, time.Duration(0)).Return(removalUUID, nil)

	res, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag:        names.NewUnitTag("foo/0").String(),
			DestroyStorage: true,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Info, tc.DeepEquals, &params.DestroyUnitInfo{
		DestroyedStorage: []params.Entity{
			{Tag: "storage-single-blk-0"},
			{Tag: "storage-single-fs-0"},
		},
	})
}

// TestDestroyUnitDryRunClassifiesStorage asserts that a dry run of destroying
// an IAAS unit reports the storage classification without removing the unit.
func (s *applicationSuite) TestDestroyUnitDryRunClassifiesStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	persistent := s.newStorageInstance(c, "single-blk/0", true)

	s.applicationService.EXPECT().IsSubordinateApplicationByName(gomock.Any(), "foo").Return(false, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID, nil)
	s.storageService.EXPECT().GetStorageClassificationForUnits(gomock.Any(), []coreunit.UUID{unitUUID}).Return(
		map[coreunit.UUID][]domainstorage.StorageInstanceClassification{
			unitUUID: {persistent},
		}, nil,
	)

	res, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag("foo/0").String(),
			DryRun:  true,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Info, tc.DeepEquals, &params.DestroyUnitInfo{
		DetachedStorage: []params.Entity{{Tag: "storage-single-blk-0"}},
	})
}

// TestDestroyApplicationClassifiesStorage asserts that destroying an IAAS
// application reports the storage of all of its units, deduplicating storage
// that is shared between units.
func (s *applicationSuite) TestDestroyApplicationClassifiesStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	unitUUID1 := tc.Must(c, coreunit.NewUUID)
	unitUUID2 := tc.Must(c, coreunit.NewUUID)
	shared := s.newStorageInstance(c, "db-dir/0", true)
	nonPersistent := s.newStorageInstance(c, "single-fs/0", false)

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "foo").Return(
		[]coreunit.Name{"foo/0", "foo/1"}, nil,
	)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID1, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/1")).Return(unitUUID2, nil)
	s.storageService.EXPECT().GetStorageClassificationForUnits(
		gomock.Any(), []coreunit.UUID{unitUUID1, unitUUID2},
	).Return(
		map[coreunit.UUID][]domainstorage.StorageInstanceClassification{
			unitUUID1: {nonPersistent, shared},
			unitUUID2: {shared},
		}, nil,
	)

	appUUID := tc.Must(c, application.NewUUID)
	s.applicationService.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	removalUUID := tc.Must(c, removal.NewUUID)
	s.removalService.EXPECT().RemoveApplication(gomock.Any(), appUUID, false, false, time.Duration(0)).Return(removalUUID, nil)

	res, err := s.api.DestroyApplication(c.Context(), params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: names.NewApplicationTag("foo").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Info, tc.DeepEquals, &params.DestroyApplicationInfo{
		DestroyedUnits: []params.Entity{
			{Tag: "unit-foo-0"},
			{Tag: "unit-foo-1"},
		},
		DestroyedStorage: []params.Entity{{Tag: "storage-single-fs-0"}},
		DetachedStorage:  []params.Entity{{Tag: "storage-db-dir-0"}},
	})
}

// TestDestroyApplicationDryRunClassifiesStorage asserts that a dry run of
// destroying an IAAS application reports the storage classification without
// removing the application.
func (s *applicationSuite) TestDestroyApplicationDryRunClassifiesStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	nonPersistent := s.newStorageInstance(c, "single-fs/0", false)

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "foo").Return(
		[]coreunit.Name{"foo/0"}, nil,
	)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID, nil)
	s.storageService.EXPECT().GetStorageClassificationForUnits(gomock.Any(), []coreunit.UUID{unitUUID}).Return(
		map[coreunit.UUID][]domainstorage.StorageInstanceClassification{
			unitUUID: {nonPersistent},
		}, nil,
	)

	res, err := s.api.DestroyApplication(c.Context(), params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: names.NewApplicationTag("foo").String(),
			DryRun:         true,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Info, tc.DeepEquals, &params.DestroyApplicationInfo{
		DestroyedUnits:   []params.Entity{{Tag: "unit-foo-0"}},
		DestroyedStorage: []params.Entity{{Tag: "storage-single-fs-0"}},
	})
}

// TestDestroyApplicationCAASSkipsStorageClassification asserts that
// destroying an application on a container model never reports storage
// classification.
func (s *applicationSuite) TestDestroyApplicationCAASSkipsStorageClassification(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectAnyPermissions()
	s.blockChecker.EXPECT().RemoveAllowed(gomock.Any()).Return(nil)
	s.newCAASAPI(c)
	s.expectDestroyCharm(c)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "foo").Return(
		[]coreunit.Name{"foo/0"}, nil,
	)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID, nil)

	appUUID := tc.Must(c, application.NewUUID)
	s.applicationService.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	removalUUID := tc.Must(c, removal.NewUUID)
	s.removalService.EXPECT().RemoveApplication(gomock.Any(), appUUID, false, false, time.Duration(0)).Return(removalUUID, nil)

	res, err := s.api.DestroyApplication(c.Context(), params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: names.NewApplicationTag("foo").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Info, tc.DeepEquals, &params.DestroyApplicationInfo{
		DestroyedUnits: []params.Entity{{Tag: "unit-foo-0"}},
	})
}

// TestDestroyApplicationUnitNotFound asserts that when a unit of the
// application disappears mid-loop, DestroyApplication reports a NotFound error.
func (s *applicationSuite) TestDestroyApplicationUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	unitUUID0 := tc.Must(c, coreunit.NewUUID)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "foo").Return(
		[]coreunit.Name{"foo/0", "foo/1"}, nil,
	)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID0, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/1")).Return(
		coreunit.UUID(""), applicationerrors.UnitNotFound,
	)

	res, err := s.api.DestroyApplication(c.Context(), params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: names.NewApplicationTag("foo").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

// TestDestroyUnitDryRunUnitNotFound asserts that DestroyUnit on dry run
// returns NotFound when the target unit does not exist.
func (s *applicationSuite) TestDestroyUnitDryRunUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	s.applicationService.EXPECT().IsSubordinateApplicationByName(gomock.Any(), "foo").Return(false, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/9")).Return(
		coreunit.UUID(""), applicationerrors.UnitNotFound,
	)

	res, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag("foo/9").String(),
			DryRun:  true,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

// TestDestroyUnitStorageServiceError asserts that an error from the storage
// service during classification is reported in the unit result error and
// prevents removal.
func (s *applicationSuite) TestDestroyUnitStorageServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	unitUUID := tc.Must(c, coreunit.NewUUID)
	boom := errors.New("boom")

	s.applicationService.EXPECT().IsSubordinateApplicationByName(gomock.Any(), "foo").Return(false, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID, nil)
	s.storageService.EXPECT().GetStorageClassificationForUnits(gomock.Any(), []coreunit.UUID{unitUUID}).Return(
		nil, boom,
	)

	res, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag("foo/0").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.ErrorMatches, `.*getting storage classification: boom.*`)
}

// TestDestroyUnitNoStorage asserts that destroying a unit with no attached
// storage reports empty storage classification slices.
func (s *applicationSuite) TestDestroyUnitNoStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	unitUUID := tc.Must(c, coreunit.NewUUID)

	s.applicationService.EXPECT().IsSubordinateApplicationByName(gomock.Any(), "foo").Return(false, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("foo/0")).Return(unitUUID, nil)
	s.storageService.EXPECT().GetStorageClassificationForUnits(gomock.Any(), []coreunit.UUID{unitUUID}).Return(
		map[coreunit.UUID][]domainstorage.StorageInstanceClassification{}, nil,
	)
	removalUUID := tc.Must(c, removal.NewUUID)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, false, false, time.Duration(0)).Return(removalUUID, nil)

	res, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag("foo/0").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Info.DestroyedStorage, tc.HasLen, 0)
	c.Check(res.Results[0].Info.DetachedStorage, tc.HasLen, 0)
}

// TestDestroyApplicationNoUnits asserts that destroying an application with
// no units succeeds and returns empty destroyed units and storage.
func (s *applicationSuite) TestDestroyApplicationNoUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectDestroyCharm(c)

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "foo").Return(
		nil, nil,
	)
	appUUID := tc.Must(c, application.NewUUID)
	s.applicationService.EXPECT().GetApplicationUUIDByName(gomock.Any(), "foo").Return(appUUID, nil)
	removalUUID := tc.Must(c, removal.NewUUID)
	s.removalService.EXPECT().RemoveApplication(gomock.Any(), appUUID, false, false, time.Duration(0)).Return(removalUUID, nil)

	res, err := s.api.DestroyApplication(c.Context(), params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: names.NewApplicationTag("foo").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[0].Info, tc.DeepEquals, &params.DestroyApplicationInfo{})
}
