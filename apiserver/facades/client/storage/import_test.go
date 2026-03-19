// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/rpc/params"
)

type importSuite struct {
	baseStorageSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	spUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageID := corestorage.MakeID("pgdata", 10)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool").Return(spUUID, nil)
	s.storageService.EXPECT().AdoptFilesystem(
		gomock.Any(),
		domainstorage.Name("pgdata"),
		spUUID, "a-provider-id", true,
	).Return(storageID, nil)

	apiArgs := params.BulkImportStorageParamsV2{
		Storage: []params.ImportStorageParamsV2{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
				Force:       true,
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *importSuite) TestImportPoolNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool",
	).Return("", domainstorageerrors.StoragePoolNotFound)

	apiArgs := params.BulkImportStorageParamsV2{
		Storage: []params.ImportStorageParamsV2{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
				Force:       true,
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
	c.Check(res.Results[0].Error.Message, tc.Equals, "storage pool not found")
}

func (s *importSuite) TestImportPoolNameInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool",
	).Return("", domainstorageerrors.StoragePoolNameInvalid)

	apiArgs := params.BulkImportStorageParamsV2{
		Storage: []params.ImportStorageParamsV2{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
				Force:       true,
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotValid)
	c.Check(res.Results[0].Error.Message, tc.Equals,
		"storage pool name is not valid")
}

func (s *importSuite) TestImportLatePoolNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	spUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool").Return(spUUID, nil)
	s.storageService.EXPECT().AdoptFilesystem(
		gomock.Any(),
		domainstorage.Name("pgdata"),
		spUUID, "a-provider-id", true,
	).Return("", domainstorageerrors.StoragePoolNotFound)

	apiArgs := params.BulkImportStorageParamsV2{
		Storage: []params.ImportStorageParamsV2{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
				Force:       true,
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
	c.Check(res.Results[0].Error.Message, tc.Equals, "storage pool not found")
}

func (s *importSuite) TestImportPooledStorageEntityNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIForIAASModel(c)

	spUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool").Return(spUUID, nil)
	s.storageService.EXPECT().AdoptFilesystem(
		gomock.Any(),
		domainstorage.Name("pgdata"),
		spUUID, "a-provider-id", true,
	).Return("", domainstorageerrors.StorageEntityNotFoundInPool)

	apiArgs := params.BulkImportStorageParamsV2{
		Storage: []params.ImportStorageParamsV2{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
				Force:       true,
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
	c.Check(res.Results[0].Error.Message, tc.Equals,
		"storage entity not found in pool")
}

type importV6Suite struct {
	baseStorageSuite
}

func TestImportV6Suite(t *testing.T) {
	tc.Run(t, &importV6Suite{})
}

func (s *importV6Suite) makeTestAPIV6ForIAASModel(c *tc.C) *StorageAPIv6 {
	return &StorageAPIv6{s.makeTestAPIForIAASModel(c)}
}

func (s *importV6Suite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIV6ForIAASModel(c)

	spUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageID := corestorage.MakeID("pgdata", 10)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool").Return(spUUID, nil)
	s.storageService.EXPECT().AdoptFilesystem(
		gomock.Any(),
		domainstorage.Name("pgdata"),
		spUUID, "a-provider-id", false,
	).Return(storageID, nil)

	apiArgs := params.BulkImportStorageParams{
		Storage: []params.ImportStorageParams{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *importV6Suite) TestImportPoolNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIV6ForIAASModel(c)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool",
	).Return("", domainstorageerrors.StoragePoolNotFound)

	apiArgs := params.BulkImportStorageParams{
		Storage: []params.ImportStorageParams{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
	c.Check(res.Results[0].Error.Message, tc.Equals, "storage pool not found")
}

func (s *importV6Suite) TestImportPoolNameInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIV6ForIAASModel(c)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool",
	).Return("", domainstorageerrors.StoragePoolNameInvalid)

	apiArgs := params.BulkImportStorageParams{
		Storage: []params.ImportStorageParams{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotValid)
	c.Check(res.Results[0].Error.Message, tc.Equals,
		"storage pool name is not valid")
}

func (s *importV6Suite) TestImportLatePoolNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIV6ForIAASModel(c)

	spUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool").Return(spUUID, nil)
	s.storageService.EXPECT().AdoptFilesystem(
		gomock.Any(),
		domainstorage.Name("pgdata"),
		spUUID, "a-provider-id", false,
	).Return("", domainstorageerrors.StoragePoolNotFound)

	apiArgs := params.BulkImportStorageParams{
		Storage: []params.ImportStorageParams{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
	c.Check(res.Results[0].Error.Message, tc.Equals, "storage pool not found")
}

func (s *importV6Suite) TestImportPooledStorageEntityNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.makeTestAPIV6ForIAASModel(c)

	spUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	s.storageService.EXPECT().GetStoragePoolUUID(
		gomock.Any(), "mypool").Return(spUUID, nil)
	s.storageService.EXPECT().AdoptFilesystem(
		gomock.Any(),
		domainstorage.Name("pgdata"),
		spUUID, "a-provider-id", false,
	).Return("", domainstorageerrors.StorageEntityNotFoundInPool)

	apiArgs := params.BulkImportStorageParams{
		Storage: []params.ImportStorageParams{
			{
				Kind:        params.StorageKindFilesystem,
				Pool:        "mypool",
				ProviderId:  "a-provider-id",
				StorageName: "pgdata",
			},
		},
	}

	res, err := api.Import(c.Context(), apiArgs)
	c.Check(err, tc.IsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
	c.Check(res.Results[0].Error.Message, tc.Equals,
		"storage entity not found in pool")
}
