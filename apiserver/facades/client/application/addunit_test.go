// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	coreunit "github.com/juju/juju/core/unit"
	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	domainapplicationservice "github.com/juju/juju/domain/application/service"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/rpc/params"
)

type addUnitSuite struct {
	baseSuite
}

func TestAddUnit(t *testing.T) {
	tc.Run(t, &addUnitSuite{})
}

func (s *addUnitSuite) TestMakeAddUnitArgsWithoutPlacementOrStorage(c *tc.C) {
	result := makeAddUnitArgs(3, nil, nil)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddUnitArg{
		{}, {}, {},
	})
}

func (s *addUnitSuite) TestMakeAddUnitArgsWithPlacementAndStorage(c *tc.C) {
	placements := []*instance.Placement{
		{Scope: "zone", Directive: "az1"},
		{Scope: "zone", Directive: "az2"},
	}
	storageInstances := [][]domainstorage.StorageInstanceUUID{
		{"storage-uuid-1"},
		{"storage-uuid-2", "storage-uuid-3"},
	}

	result := makeAddUnitArgs(3, placements, storageInstances)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddUnitArg{{
		Placement: &instance.Placement{
			Scope: "zone", Directive: "az1",
		},
		StorageInstancesToAttach: []domainstorage.StorageInstanceUUID{
			"storage-uuid-1",
		},
	}, {
		Placement: &instance.Placement{
			Scope: "zone", Directive: "az2",
		},
		StorageInstancesToAttach: []domainstorage.StorageInstanceUUID{
			"storage-uuid-2", "storage-uuid-3",
		},
	}, {
		Placement:                nil,
		StorageInstancesToAttach: nil,
	}})
}

func (s *addUnitSuite) TestMakeIAASAddUnitArgsWithoutPlacementOrStorage(c *tc.C) {
	result := makeIAASAddUnitArgs(2, nil, nil)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddIAASUnitArg{
		{AddUnitArg: domainapplicationservice.AddUnitArg{}},
		{AddUnitArg: domainapplicationservice.AddUnitArg{}},
	})
}

func (s *addUnitSuite) TestMakeIAASAddUnitArgsWithPlacementAndStorage(c *tc.C) {
	placements := []*instance.Placement{
		{Scope: "zone", Directive: "az1"},
	}
	storageInstances := [][]domainstorage.StorageInstanceUUID{
		{"storage-uuid-1"},
	}

	result := makeIAASAddUnitArgs(2, placements, storageInstances)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddIAASUnitArg{{
		AddUnitArg: domainapplicationservice.AddUnitArg{
			Placement: &instance.Placement{
				Scope: "zone", Directive: "az1",
			},
			StorageInstancesToAttach: []domainstorage.StorageInstanceUUID{
				"storage-uuid-1",
			},
		},
	}, {
		AddUnitArg: domainapplicationservice.AddUnitArg{
			Placement:                nil,
			StorageInstancesToAttach: nil,
		},
	}})
}

func (s *addUnitSuite) TestMakeCAASAddUnitArgsWithoutPlacementOrStorage(c *tc.C) {
	result := makeCAASAddUnitArgs(2, nil, nil)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddUnitArg{
		{}, {},
	})
}

func (s *addUnitSuite) TestMakeCAASAddUnitArgsWithPlacementAndStorage(c *tc.C) {
	placements := []*instance.Placement{
		{Scope: "zone", Directive: "az1"},
	}
	storageInstances := [][]domainstorage.StorageInstanceUUID{
		{"storage-uuid-1"},
	}

	result := makeCAASAddUnitArgs(2, placements, storageInstances)
	c.Assert(result, tc.DeepEquals, []domainapplicationservice.AddUnitArg{{
		Placement: &instance.Placement{
			Scope: "zone", Directive: "az1",
		},
		StorageInstancesToAttach: []domainstorage.StorageInstanceUUID{
			"storage-uuid-1",
		},
	}, {
		Placement:                nil,
		StorageInstancesToAttach: nil,
	}})
}

// TestAddUnitsCAASNotSupported verifies that AddUnits rejects CAAS models.
func (s *addUnitSuite) TestAddUnitsCAASNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newCAASAPI(c)

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestAddUnitsZeroNumUnits verifies that requesting zero units is rejected.
func (s *addUnitSuite) TestAddUnitsZeroNumUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        0,
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestAddUnitsSuccess verifies the happy path returns the added unit names.
func (s *addUnitSuite) TestAddUnitsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	s.storageService.EXPECT().GetStorageInstanceUUIDsByIDs(
		gomock.Any(), gomock.Any(),
	).Return(map[string]domainstorage.StorageInstanceUUID{}, nil)
	s.applicationService.EXPECT().AddIAASUnits(
		gomock.Any(), "foo", gomock.Any(),
	).Return([]coreunit.Name{"foo/0", "foo/1"}, nil, nil)

	result, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        2,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Units, tc.DeepEquals, []string{"foo/0", "foo/1"})
}

// TestAddUnitsWithStorageAttach verifies storage tags are resolved and passed
// through to the application service when adding a single unit.
func (s *addUnitSuite) TestAddUnitsWithStorageAttach(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	s.storageService.EXPECT().GetStorageInstanceUUIDsByIDs(
		gomock.Any(), []string{"data/0"},
	).Return(map[string]domainstorage.StorageInstanceUUID{
		"data/0": "storage-uuid-1",
	}, nil)
	s.applicationService.EXPECT().AddIAASUnits(
		gomock.Any(), "foo", gomock.Any(),
	).Return([]coreunit.Name{"foo/0"}, nil, nil)

	result, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
		AttachStorage:   []string{"storage-data-0"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Units, tc.DeepEquals, []string{"foo/0"})
}

// TestAddUnitsStorageAttachWithMultipleUnits verifies that attaching storage
// to more than one unit at a time is rejected.
func (s *addUnitSuite) TestAddUnitsStorageAttachWithMultipleUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        2,
		AttachStorage:   []string{"storage-data-0"},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestAddUnitsAttachStorageNotFound verifies that add-unit fails when any
// requested storage instance cannot be resolved.
func (s *addUnitSuite) TestAddUnitsAttachStorageNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	s.storageService.EXPECT().GetStorageInstanceUUIDsByIDs(
		gomock.Any(), []string{"data/0", "cache/0"},
	).Return(map[string]domainstorage.StorageInstanceUUID{
		"data/0": "storage-uuid-1",
	}, nil)

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
		AttachStorage:   []string{"storage-data-0", "storage-cache-0"},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(err, tc.ErrorMatches, `storage instance "cache/0" does not exist`)
}

// TestAddUnitsStorageAttachWithMultipleUnitsMissingStorage verifies that the
// single-unit restriction is enforced from the request, even if resolution
// would otherwise return no storage instances.
func (s *addUnitSuite) TestAddUnitsStorageAttachWithMultipleUnitsMissingStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        2,
		AttachStorage:   []string{"storage-missing-0"},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestAddUnitsInvalidStorageTag verifies that a malformed storage tag string
// is rejected before any service calls are made.
func (s *addUnitSuite) TestAddUnitsInvalidStorageTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
		AttachStorage:   []string{"not-a-storage-tag"},
	})
	c.Assert(err, tc.ErrorMatches, `.*not-a-storage-tag.*`)
}

// TestAddUnitsApplicationNotFound verifies that a missing application is
// surfaced as a NotFound error.
func (s *addUnitSuite) TestAddUnitsApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	s.storageService.EXPECT().GetStorageInstanceUUIDsByIDs(
		gomock.Any(), gomock.Any(),
	).Return(map[string]domainstorage.StorageInstanceUUID{}, nil)
	s.applicationService.EXPECT().AddIAASUnits(
		gomock.Any(), "nonexistent", gomock.Any(),
	).Return(nil, nil, domainapplicationerrors.ApplicationNotFound)

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "nonexistent",
		NumUnits:        1,
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestAddUnitsStorageServiceError verifies that a failure from the storage
// service is propagated as an error.
func (s *addUnitSuite) TestAddUnitsStorageServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.newIAASAPI(c)

	s.storageService.EXPECT().GetStorageInstanceUUIDsByIDs(
		gomock.Any(), gomock.Any(),
	).Return(nil, fmt.Errorf("storage service failed"))

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
	})
	c.Assert(
		err, tc.ErrorMatches,
		"getting storage instance UUIDs: storage service failed",
	)
}
