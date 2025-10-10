// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model/internal"
	domainstorage "github.com/juju/juju/domain/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
)

type storagePoolSuite struct {
	baseSuite
}

func TestStoragePoolSuite(t *testing.T) {
	tc.Run(t, &storagePoolSuite{})
}

func (s *storagePoolSuite) providerService(
	c *tc.C,
) *ProviderModelService {
	uuid := tc.Must(c, coremodel.NewUUID)
	return NewProviderModelService(
		uuid,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		func(context.Context) (ModelResourcesProvider, error) { return s.mockModelResourceProvider, nil },
		func(context.Context) (CloudInfoProvider, error) { return s.mockCloudInfoProvider, nil },
		func(context.Context) (RegionProvider, error) { return s.mockRegionProvider, nil },
		s.storageProviderRegistryGetter(),
		DefaultAgentBinaryFinder(),
		loggertesting.WrapCheckLog(c),
	)
}

func (s *storagePoolSuite) TestGetRecommendedStoragePoolsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(gomock.Any()).Return(
		nil,
	).AnyTimes()
	poolArgs, setArgs, err := getRecommendedStoragePools(s.mockProviderRegistry)
	c.Check(err, tc.ErrorIsNil)
	c.Check(poolArgs, tc.HasLen, 0)
	c.Check(setArgs, tc.HasLen, 0)
}

func (s *storagePoolSuite) TestGetRecommendedStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsCfg, err := internalstorage.NewConfig(
		"fsPool",
		internalstorage.ProviderType("testprovider"),
		internalstorage.Attrs{
			"key1": "val1",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem,
	).Return(fsCfg)

	bsCfg, err := internalstorage.NewConfig(
		"bsPool",
		internalstorage.ProviderType("testprovider"),
		internalstorage.Attrs{
			"key1": "val1",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock,
	).Return(bsCfg)

	poolArgs, setArgs, err := getRecommendedStoragePools(s.mockProviderRegistry)
	c.Check(err, tc.ErrorIsNil)

	expectedPoolArgs := []internal.CreateModelDefaultStoragePoolArg{
		{
			Attributes: map[string]string{
				"key1": "val1",
			},
			Name:   "fsPool",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   "testprovider",
		},
		{
			Attributes: map[string]string{
				"key1": "val1",
			},
			Name:   "bsPool",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   "testprovider",
		},
	}

	var expectedSetArgs []internal.SetModelStoragePoolArg
	for _, p := range poolArgs {
		switch p.Name {
		case "fsPool":
			expectedSetArgs = append(
				expectedSetArgs,
				internal.SetModelStoragePoolArg{
					StorageKind:     domainstorage.StorageKindFilesystem,
					StoragePoolUUID: p.UUID,
				},
			)
		case "bsPool":
			expectedSetArgs = append(
				expectedSetArgs,
				internal.SetModelStoragePoolArg{
					StorageKind:     domainstorage.StorageKindBlock,
					StoragePoolUUID: p.UUID,
				},
			)
		}
	}

	pChecker := tc.NewMultiChecker()
	pChecker.AddExpr("_[_].UUID", tc.IsNonZeroUUID)

	c.Check(poolArgs, pChecker, expectedPoolArgs)
	c.Check(setArgs, tc.SameContents, expectedSetArgs)
}

// TestGetRecommendedStoragePoolDedupe is testing that if the registry returns
// the same storage pool multiple times the results are deduplicated with only
// one storage pool being returned to the caller.
func (s *storagePoolSuite) TestGetRecommendedStoragePoolDedupe(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolCfg, err := internalstorage.NewConfig(
		"pool",
		internalstorage.ProviderType("testprovider"),
		internalstorage.Attrs{
			"key1": "val1",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(
		gomock.Any(),
	).Return(poolCfg).AnyTimes()

	poolArgs, setArgs, err := getRecommendedStoragePools(s.mockProviderRegistry)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(poolArgs, tc.HasLen, 1)

	expectedPoolArgs := []internal.CreateModelDefaultStoragePoolArg{
		{
			Attributes: map[string]string{
				"key1": "val1",
			},
			Name:   "pool",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   "testprovider",
		},
	}

	expectedSetArgs := []internal.SetModelStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: poolArgs[0].UUID,
		},
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: poolArgs[0].UUID,
		},
	}

	pChecker := tc.NewMultiChecker()
	pChecker.AddExpr("_[_].UUID", tc.IsNonZeroUUID)

	c.Check(poolArgs, pChecker, expectedPoolArgs)
	c.Check(setArgs, tc.SameContents, expectedSetArgs)
}

// TestGetRecommendedStoragePoolOnlyOne checks that if the registry only supplys
// one recommendation then the caller gets back exactly one association.
func (s *storagePoolSuite) TestGetRecommendedStoragePoolOnlyOne(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolCfg, err := internalstorage.NewConfig(
		"pool",
		internalstorage.ProviderType("testprovider"),
		internalstorage.Attrs{
			"key1": "val1",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem,
	).Return(poolCfg)
	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(
		gomock.Any(),
	).Return(nil).AnyTimes()

	poolArgs, setArgs, err := getRecommendedStoragePools(s.mockProviderRegistry)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(poolArgs, tc.HasLen, 1)

	expectedPoolArgs := []internal.CreateModelDefaultStoragePoolArg{
		{
			Attributes: map[string]string{
				"key1": "val1",
			},
			Name:   "pool",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   "testprovider",
		},
	}

	expectedSetArgs := []internal.SetModelStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: poolArgs[0].UUID,
		},
	}

	pChecker := tc.NewMultiChecker()
	pChecker.AddExpr("_[_].UUID", tc.IsNonZeroUUID)

	c.Check(poolArgs, pChecker, expectedPoolArgs)
	c.Check(setArgs, tc.SameContents, expectedSetArgs)
}

func (s *storagePoolSuite) TestSeedDefaultStoragePools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool1, _ := internalstorage.NewConfig(
		"tmpfs", "tmpfs", internalstorage.Attrs{
			"attr1": "val1",
		},
	)
	defaultPool2, _ := internalstorage.NewConfig(
		"rootfs", "rootfs", internalstorage.Attrs{
			"attr1": "val1",
		},
	)
	sp1 := NewMockStorageProvider(ctrl)
	sp2 := NewMockStorageProvider(ctrl)
	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem,
	).Return(defaultPool1)
	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(
		gomock.Any(),
	).Return(nil).AnyTimes()
	s.mockProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{
			"tmpfs",
			"rootfs",
		},
		nil,
	)
	s.mockProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("tmpfs"),
	).Return(sp1, nil)
	s.mockProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("rootfs"),
	).Return(sp2, nil)

	sp1.EXPECT().DefaultPools().Return([]*internalstorage.Config{defaultPool1})
	sp2.EXPECT().DefaultPools().Return([]*internalstorage.Config{defaultPool2})

	s.mockModelState.EXPECT().EnsureDefaultStoragePools(
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(
		_ context.Context, args []internal.CreateModelDefaultStoragePoolArg) error {
		c.Check(args, tc.SameContents, []internal.CreateModelDefaultStoragePoolArg{
			{
				Attributes: map[string]string{
					"attr1": "val1",
				},
				Name:   "tmpfs",
				Origin: domainstorage.StoragePoolOriginProviderDefault,
				Type:   "tmpfs",
				UUID:   "6a16b09c-8ca9-5952-a50a-9082ae7c32c1",
			},
			{
				Attributes: map[string]string{
					"attr1": "val1",
				},
				Name:   "rootfs",
				Origin: domainstorage.StoragePoolOriginProviderDefault,
				Type:   "rootfs",
				UUID:   "4d9a00e0-bf5f-5823-8ffa-db1a2ffb940c",
			},
		})
		return nil
	})
	s.mockModelState.EXPECT().SetModelStoragePools(
		gomock.Any(), []internal.SetModelStoragePoolArg{
			{
				StorageKind:     domainstorage.StorageKindFilesystem,
				StoragePoolUUID: "6a16b09c-8ca9-5952-a50a-9082ae7c32c1",
			},
		},
	)

	svc := s.providerService(c)
	err := svc.SeedDefaultStoragePools(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestSeedDefaultStoragePoolsNotKnown is a regression test. When a provider
// returns a default storage pool that the storage domain has not yet been made
// aware of we currently log that no fixed uuid exists. We should however not
// try to include this default pool in the model as it will be made with a uuid
// that we cannot predict and so model migration will be hampered in the future.
//
// The regression this is testing was still trying to add the pool with an empty
// uuid when the default pool was not known.
func (s *storagePoolSuite) TestSeedDefaultStoragePoolsNotKnown(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool1, _ := internalstorage.NewConfig(
		"unknown", "unknown", internalstorage.Attrs{},
	)
	sp1 := NewMockStorageProvider(ctrl)
	sp1.EXPECT().DefaultPools().Return([]*internalstorage.Config{defaultPool1})
	s.mockProviderRegistry.EXPECT().RecommendedPoolForKind(

		gomock.Any(),
	).Return(nil).AnyTimes()
	s.mockProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{"unknown"}, nil,
	)
	s.mockProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("unknown"),
	).Return(sp1, nil)

	s.mockModelState.EXPECT().EnsureDefaultStoragePools(
		gomock.Any(),
		// We don't want to see any pools being created.
		[]internal.CreateModelDefaultStoragePoolArg{},
	).Return(nil).Return(nil)
	s.mockModelState.EXPECT().SetModelStoragePools(
		gomock.Any(), []internal.SetModelStoragePoolArg{},
	).Return(nil).AnyTimes()

	svc := s.providerService(c)
	err := svc.SeedDefaultStoragePools(c.Context())
	c.Check(err, tc.ErrorIsNil)
}
