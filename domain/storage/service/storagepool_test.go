// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/description/v11"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// registryGetter provides a testing implementation of
// [corestorage.ModelStorageRegistryGetter] based off of a
// [internalstorage.ProviderRegistry].
type registryGetter struct {
	internalstorage.ProviderRegistry
}

// storagePoolServiceSuite is a set of tests to verify the functionality of the
// [StoragePoolService] interface and contracts.
type storagePoolServiceSuite struct {
	registry                internalstorage.StaticProviderRegistry
	state                   *MockStoragePoolState
	storageProviderRegistry *MockProviderRegistry
	storageRegistryGetter   *MockModelStorageRegistryGetter
	storageProvider         *MockProvider
}

// TestStoragePoolServiceSuite runs all of the tests contained within
// [storagePoolServiceSuite].
func TestStoragePoolServiceSuite(t *testing.T) {
	tc.Run(t, &storagePoolServiceSuite{})
}

// GetStorageRegistry return the [internalstorage.ProviderRegistry] captrued
// within this type.
//
// Implements the [corestorage.ModelStorageRegistryGetter] interface.
func (r registryGetter) GetStorageRegistry(
	context.Context,
) (internalstorage.ProviderRegistry, error) {
	return r.ProviderRegistry, nil
}

func (s *storagePoolServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.registry = internalstorage.StaticProviderRegistry{
		Providers: map[internalstorage.ProviderType]internalstorage.Provider{},
	}
	s.state = NewMockStoragePoolState(ctrl)
	s.storageProviderRegistry = NewMockProviderRegistry(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.storageProvider = NewMockProvider(ctrl)

	c.Cleanup(func() {
		s.registry = internalstorage.StaticProviderRegistry{}
		s.state = nil
		s.storageProviderRegistry = nil
		s.storageRegistryGetter = nil
		s.storageProvider = nil
	})

	return ctrl
}

// TestCreateStoragePool is a happy bath test for
// [StoragePoolService.CreateStoragePool].
func (s *storagePoolServiceSuite) TestCreateStoragePool(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	provider := NewMockProvider(ctrl)
	providerEXP := provider.EXPECT()
	providerEXP.ValidateConfig(gomock.Any())

	s.registry.Providers["storageprovider1"] = provider

	createArgs := domainstorageinternal.CreateStoragePool{
		Attrs: map[string]string{
			"key": "val",
		},
		Name:         "my-pool",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("storageprovider1"),
	}

	createArgsMC := tc.NewMultiChecker()
	createArgsMC.AddExpr("_.UUID", tc.IsUUID)
	s.state.EXPECT().CreateStoragePool(
		gomock.Any(), tc.Bind(createArgsMC, createArgs),
	).Return(nil)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	uuid, err := svc.CreateStoragePool(
		c.Context(),
		"my-pool",
		"storageprovider1",
		map[string]any{"key": "val"},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid, tc.IsUUID)
}

// TestCreateStoragePoolWithInvalidProviderType tests that supplying an invalid
// provider type value when creating a new storage pool results in the caller
// getting back an error that satisfies
// [domainstorageerrors.ProviderTypeInvalid].
func (s *storagePoolServiceSuite) TestCreateStoragePoolWithInvalidProviderType(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// It is on purpose that no mock expects are established in this test. This
	// error SHOULD always happen before do expensive operations on
	// dependencies.

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	_, err := svc.CreateStoragePool(
		c.Context(), "my-testpool", "-invalid-provider-type", nil,
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.ProviderTypeInvalid)
}

// TestCreateStoragePoolProviderTypeNotFound tests that supplying a provider
// type that does not exist in the storage registry returns to the caller a
// [domainstorageerrors.ProviderTypeNotFound] error.
func (s *storagePoolServiceSuite) TestCreateStoragePoolProviderTypeNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	// It is on purpose that no mock expects are established in this test. This
	// error SHOULD always happen before do expensive operations on
	// dependencies.

	registry := NewMockProviderRegistry(ctrl)
	registryEXP := registry.EXPECT()
	// NotFound is returned by registry implementations when no provider exists
	// for a given type.
	registryEXP.StorageProvider(
		internalstorage.ProviderType("storagep1"),
	).Return(nil, coreerrors.NotFound)

	svc := StoragePoolService{
		registryGetter: registryGetter{registry},
		st:             s.state,
	}

	_, err := svc.CreateStoragePool(
		c.Context(), "my-testpool", "storagep1", nil,
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.ProviderTypeNotFound)
}

// TestCreateStoragePoolWithInvalidName tests that supplying an invalid name
// for a new storage pool results in the caller getting back an error that
// satisfies [domainstorageerrors.StoragePoolNameInvalid].
func (s *storagePoolServiceSuite) TestCreateStoragePoolWithInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	_, err := svc.CreateStoragePool(
		c.Context(),
		"66invalid",
		"ebs",
		nil,
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNameInvalid)
}

// TestCreateStoragePoolAlreadyExists tests the case where a caller attempts to
// add a new storage pool to the model using a name that already exists. We
// expect the caller in this case to get back an error that satisfies
// [domainstorageerrors.StoragePoolAlreadyExists].
func (s *storagePoolServiceSuite) TestCreateStoragePoolAlreadyExists(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	provider := NewMockProvider(ctrl)
	providerEXP := provider.EXPECT()
	providerEXP.ValidateConfig(gomock.Any())

	s.registry.Providers["storageprovider1"] = provider

	createArgs := domainstorageinternal.CreateStoragePool{
		Attrs: map[string]string{
			"key": "val",
		},
		Name:         "my-pool",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("storageprovider1"),
	}

	createArgsMC := tc.NewMultiChecker()
	createArgsMC.AddExpr("_.UUID", tc.IsUUID)
	s.state.EXPECT().CreateStoragePool(
		gomock.Any(), tc.Bind(createArgsMC, createArgs),
	).Return(domainstorageerrors.StoragePoolAlreadyExists)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	_, err := svc.CreateStoragePool(
		c.Context(),
		"my-pool",
		"storageprovider1",
		map[string]any{"key": "val"},
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolAlreadyExists)
}

// TestCreateStoragePoolUsesSameUUID is a sanity check to make sure that the
// storage pool UUID created by the service is the same one passed to the state
// layer that is returned to the caller.
func (s *storagePoolServiceSuite) TestCreateStoragePoolUsesSameUUID(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	provider := NewMockProvider(ctrl)
	providerEXP := provider.EXPECT()
	providerEXP.ValidateConfig(gomock.Any())

	s.registry.Providers["storageprovider1"] = provider

	var capturedUUID domainstorage.StoragePoolUUID
	s.state.EXPECT().CreateStoragePool(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(
		func(_ context.Context, args domainstorageinternal.CreateStoragePool) error {
			capturedUUID = args.UUID
			return nil
		},
	)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	gotUUID, err := svc.CreateStoragePool(
		c.Context(),
		"my-pool",
		"storageprovider1",
		map[string]any{"key": "val"},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, capturedUUID)
}

// TestCreateStoragePoolProviderValidationFail tests that when validation of
// a storage pools configuration fails the error returned by the provider is
// maintained up the stack to the caller.
//
// Long term this is not the contract we wish to maintain with a provider and it
// would ideally be done so that the provider returns an error describing the
// exact attribute key that failed and a reason. By having a more robust error
// type contract with providers it would allow the service to correctly signal
// to the caller what the problem is.
func (s *storagePoolServiceSuite) TestCreateStoragePoolProviderValidationFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	intPoolConfig, err := internalstorage.NewConfig(
		"my-pool",
		"storageprovider1",
		internalstorage.Attrs{
			"key": "val",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	validationErr := errors.New("pool validation failed")
	provider := NewMockProvider(ctrl)
	providerEXP := provider.EXPECT()
	providerEXP.ValidateConfig(tc.Bind(tc.DeepEquals, intPoolConfig)).Return(
		validationErr,
	)

	s.registry.Providers["storageprovider1"] = provider

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	_, err = svc.CreateStoragePool(
		c.Context(),
		"my-pool",
		"storageprovider1",
		map[string]any{"key": "val"},
	)
	c.Check(err, tc.ErrorIs, validationErr)
}

func (s *storagePoolServiceSuite) TestListStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePools(gomock.Any()).Return([]domainstorage.StoragePool{sp}, nil)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	got, err := svc.ListStoragePools(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesAndProviders(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePoolsByNamesAndProviders(gomock.Any(), domainstorage.Names{"ebs-fast"}, domainstorage.Providers{"ebs"}).
		Return([]domainstorage.StoragePool{sp}, nil)

	provider := NewMockProvider(ctrl)
	s.registry.Providers["ebs"] = provider

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	got, err := svc.ListStoragePoolsByNamesAndProviders(c.Context(), domainstorage.Names{"ebs-fast"}, domainstorage.Providers{"ebs"})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesAndProvidersEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	got, err := svc.ListStoragePoolsByNamesAndProviders(c.Context(), domainstorage.Names{}, domainstorage.Providers{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 0)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesAndProvidersInvalidNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	_, err := svc.ListStoragePoolsByNamesAndProviders(c.Context(), domainstorage.Names{"666invalid"}, domainstorage.Providers{"ebs"})
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNameInvalid)
	c.Assert(err, tc.ErrorMatches, `pool name "666invalid" not valid`)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesAndProvidersInvalidProviders(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	_, err := svc.ListStoragePoolsByNamesAndProviders(c.Context(), domainstorage.Names{"loop"}, domainstorage.Providers{"invalid"})
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePoolsByNames(gomock.Any(), domainstorage.Names{"ebs-fast"}).
		Return([]domainstorage.StoragePool{sp}, nil)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	got, err := svc.ListStoragePoolsByNames(c.Context(), domainstorage.Names{"ebs-fast"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	got, err := svc.ListStoragePoolsByNames(c.Context(), domainstorage.Names{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 0)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByNamesInvalidNames(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	_, err := svc.ListStoragePoolsByNames(c.Context(), domainstorage.Names{"666invalid"})
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNameInvalid)
	c.Assert(err, tc.ErrorMatches, `pool name "666invalid" not valid`)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByProviders(c *tc.C) {
	ctrl := s.setupMocks(c)
	ctrl.Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	s.state.EXPECT().ListStoragePoolsByProviders(gomock.Any(), domainstorage.Providers{"ebs"}).
		Return([]domainstorage.StoragePool{sp}, nil)

	provider := NewMockProvider(ctrl)
	s.registry.Providers["ebs"] = provider

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	got, err := svc.ListStoragePoolsByProviders(c.Context(), domainstorage.Providers{"ebs"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.SameContents, []domainstorage.StoragePool{sp})
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByProvidersEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	got, err := svc.ListStoragePoolsByProviders(c.Context(), domainstorage.Providers{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 0)
}

func (s *storagePoolServiceSuite) TestListStoragePoolsByProvidersInvalidProviders(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	_, err := svc.ListStoragePoolsByProviders(c.Context(), domainstorage.Providers{"invalid"})
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `storage provider "invalid" not found`)
}

func (s *storagePoolServiceSuite) TestGetStoragePoolByName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
		},
	}
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	s.state.EXPECT().GetStoragePoolUUID(gomock.Any(), "ebs-fast").Return(poolUUID, nil)
	s.state.EXPECT().GetStoragePool(gomock.Any(), poolUUID).Return(sp, nil)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	got, err := svc.GetStoragePoolByName(c.Context(), "ebs-fast")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, sp)
}

func (s *storagePoolServiceSuite) TestGetStoragePoolByNamePoolNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetStoragePoolUUID(gomock.Any(), "ebs-fast").Return("", domainstorageerrors.StoragePoolNotFound)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	_, err := svc.GetStoragePoolByName(c.Context(), "ebs-fast")
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *storagePoolServiceSuite) TestGetStoragePoolByNameInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}
	_, err := svc.GetStoragePoolByName(c.Context(), "666invalid")
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNameInvalid)
}

// TestImportStoragePools tests the happy path where a single storage pool
// is validated and created successfully.
func (s *storagePoolServiceSuite) TestImportStoragePools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	provider := NewMockProvider(ctrl)
	provider.EXPECT().ValidateConfig(gomock.Any())

	s.registry.Providers["storageprovider1"] = provider

	uuid := domainstorage.StoragePoolUUID("123e4567-e89b-12d3-a456-426614174000")

	createArg := domainstorageinternal.CreateStoragePool{
		UUID:         uuid,
		Name:         "my-pool",
		ProviderType: domainstorage.ProviderType("storageprovider1"),
		Origin:       domainstorage.StoragePoolOriginProviderDefault,
		Attrs: map[string]string{
			"key": "val",
		},
	}

	s.state.EXPECT().
		CreateStoragePool(gomock.Any(), createArg).
		Return(nil)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	err := svc.ImportStoragePools(
		c.Context(),
		[]domainstorage.ImportStoragePoolParams{
			{
				UUID:   uuid,
				Name:   "my-pool",
				Type:   "storageprovider1",
				Origin: domainstorage.StoragePoolOriginProviderDefault,
				Attrs:  map[string]any{"key": "val"},
			},
		},
	)

	c.Check(err, tc.ErrorIsNil)
}

// TestImportStoragePoolsMultipleSuccess tests that multiple storage pools
// are validated and created successfully when no errors occur.
func (s *storagePoolServiceSuite) TestImportStoragePoolsMultipleSuccess(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	provider := NewMockProvider(ctrl)
	// ValidateConfig should be called once per pool.
	provider.EXPECT().ValidateConfig(gomock.Any()).Times(2)

	s.registry.Providers["storageprovider1"] = provider

	pool1UUID := domainstorage.StoragePoolUUID("111e4567-e89b-12d3-a456-426614174000")
	pool2UUID := domainstorage.StoragePoolUUID("222e4567-e89b-12d3-a456-426614174000")

	gomock.InOrder(
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), domainstorageinternal.CreateStoragePool{
				UUID:         pool1UUID,
				Name:         "pool-one",
				ProviderType: domainstorage.ProviderType("storageprovider1"),
				Origin:       domainstorage.StoragePoolOriginUser,
				Attrs: map[string]string{
					"a": "1",
				},
			}),
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), domainstorageinternal.CreateStoragePool{
				UUID:         pool2UUID,
				Name:         "pool-two",
				ProviderType: domainstorage.ProviderType("storageprovider1"),
				Origin:       domainstorage.StoragePoolOriginProviderDefault,
				Attrs: map[string]string{
					"b": "true",
				},
			}),
	)

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	err := svc.ImportStoragePools(
		c.Context(),
		[]domainstorage.ImportStoragePoolParams{
			{
				UUID:   pool1UUID,
				Name:   "pool-one",
				Type:   "storageprovider1",
				Origin: domainstorage.StoragePoolOriginUser,
				Attrs:  map[string]any{"a": 1},
			},
			{
				UUID:   pool2UUID,
				Name:   "pool-two",
				Type:   "storageprovider1",
				Origin: domainstorage.StoragePoolOriginProviderDefault,
				Attrs:  map[string]any{"b": true},
			},
		},
	)

	c.Check(err, tc.ErrorIsNil)
}

// TestImportStoragePoolsInvalidProviderType tests that an invalid provider type
// returns [domainstorageerrors.ProviderTypeInvalid].
func (s *storagePoolServiceSuite) TestImportStoragePoolsInvalidProviderType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	err := svc.ImportStoragePools(
		c.Context(),
		[]domainstorage.ImportStoragePoolParams{
			{
				Name:   "my-pool",
				Type:   "-invalid-provider-",
				Origin: domainstorage.StoragePoolOriginUser,
			},
		},
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.ProviderTypeInvalid)
}

// TestImportStoragePoolsProviderTypeNotFound tests that importing a storage
// pool for a provider not present in the registry returns
// [domainstorageerrors.ProviderTypeNotFound].
func (s *storagePoolServiceSuite) TestImportStoragePoolsProviderTypeNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	registry := NewMockProviderRegistry(ctrl)
	registry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storagep1")).
		Return(nil, coreerrors.NotFound)

	svc := StoragePoolService{
		registryGetter: registryGetter{registry},
		st:             s.state,
	}

	err := svc.ImportStoragePools(
		c.Context(),
		[]domainstorage.ImportStoragePoolParams{
			{
				Name:   "my-pool",
				Type:   "storagep1",
				Origin: domainstorage.StoragePoolOriginUser,
			},
		},
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.ProviderTypeNotFound)
}

// TestImportStoragePoolsProviderRegistryError tests that unexpected
// errors returned by the provider registry are propagated.
func (s *storagePoolServiceSuite) TestImportStoragePoolsProviderRegistryError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	registryErr := errors.New("registry failure")

	registry := NewMockProviderRegistry(ctrl)
	registry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storageprovider1")).
		Return(nil, registryErr)

	svc := StoragePoolService{
		registryGetter: registryGetter{registry},
		st:             s.state,
	}

	err := svc.ImportStoragePools(
		c.Context(),
		[]domainstorage.ImportStoragePoolParams{
			{
				Name:   "my-pool",
				Type:   "storageprovider1",
				Origin: domainstorage.StoragePoolOriginUser,
			},
		},
	)

	c.Check(err, tc.ErrorIs, registryErr)
}

// TestImportStoragePoolsInvalidName tests that an invalid legacy storage
// pool name returns [domainstorageerrors.StoragePoolNameInvalid].
func (s *storagePoolServiceSuite) TestImportStoragePoolsInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		st:             s.state,
	}

	err := svc.ImportStoragePools(
		c.Context(),
		[]domainstorage.ImportStoragePoolParams{
			{
				// Must start with a letter.
				Name:   "66invalid",
				Type:   "ebs",
				Origin: domainstorage.StoragePoolOriginUser,
			},
		},
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNameInvalid)
}

// TestSetRecommendedStoragePools tests that the service correctly converts
// recommended storage pool parameters into model arguments and delegates
// persistence to the state layer without error.
func (s *storagePoolServiceSuite) TestSetRecommendedStoragePools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uuid1 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	uuid2 := tc.Must(c, domainstorage.NewStoragePoolUUID)

	input := []domainstorage.RecommendedStoragePoolParams{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: uuid2,
		},
	}

	svc := StoragePoolService{
		st: s.state,
	}

	s.state.EXPECT().SetModelStoragePools(gomock.Any(), []domainstorage.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: uuid2,
		},
	})

	err := svc.SetRecommendedStoragePools(c.Context(), input)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetRecommendedStoragePoolsPoolNotFound tests that the service propagates
// a [domainstorageerrors.StoragePoolNotFound] error returned by the state layer
// when a referenced storage pool does not exist.
func (s *storagePoolServiceSuite) TestSetRecommendedStoragePoolsPropagatesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedErr := domainstorageerrors.StoragePoolNotFound

	svc := &StoragePoolService{
		st: s.state,
	}

	s.state.EXPECT().SetModelStoragePools(gomock.Any(), gomock.Any()).Return(
		domainstorageerrors.StoragePoolNotFound)

	err := svc.SetRecommendedStoragePools(c.Context(), []domainstorage.RecommendedStoragePoolParams{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: tc.Must(c, domainstorage.NewStoragePoolUUID),
		},
	})

	c.Check(err, tc.ErrorIs, expectedErr)
}

// TestImport tests that both user-defined storage pools and provider default
// pools are returned and the recommended storage pools are returned accordingly.
func (s *storagePoolServiceSuite) TestImport(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := internalstorage.NewConfig(
		"lxd",
		"lxd",
		internalstorage.Attrs{"foo": "bar"},
	)
	c.Assert(err, tc.ErrorIsNil)

	provider := NewMockProvider(ctrl)
	provider.EXPECT().DefaultPools().Return([]*internalstorage.Config{cfg})

	s.registry.Providers["lxd"] = provider
	model := description.NewModel(description.ModelArgs{})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "custom-pool",
		Provider:   "lxd",
		Attributes: nil,
	})
	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		logger:         loggertesting.WrapCheckLog(c),
	}

	pools, recommended, err := svc.GetStoragePoolsToImport(
		c.Context(),
		model.StoragePools(),
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.HasLen, 0)
	c.Assert(pools, tc.HasLen, 2)
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:  "custom-pool",
		Type:  "lxd",
		Attrs: nil,
	}))
	c.Assert(pools[1], tc.DeepEquals, domainstorage.ImportStoragePoolParams{
		UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
		Name:   "lxd",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Attrs:  map[string]any{"foo": "bar"},
	})
}

// TestGetStoragePoolsToImportUserPoolsOnly verifies that when only user-defined
// storage pools are present and that there are no provider default pools, the
// service returns them unchanged, generates UUIDs, and does not include provider default or
// recommended pools.
func (s *storagePoolServiceSuite) TestGetStoragePoolsToImportUserPoolsOnly(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "user-pool",
		Provider:   "lxd",
		Attributes: map[string]any{"foo": "bar"},
	})

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		logger:         loggertesting.WrapCheckLog(c),
	}

	pools, recommended, err := svc.GetStoragePoolsToImport(
		c.Context(),
		model.StoragePools(),
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.HasLen, 0)
	c.Assert(pools, tc.HasLen, 1)
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "user-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
}

// TestImportPickUserDefinedOnDuplicate ensures that when a user-defined storage
// pool conflicts by name and provider with a provider default pool, the user-defined
// pool is preferred and the conflicting default pool is skipped.
func (s *storagePoolServiceSuite) TestImportPickUserDefinedOnDuplicate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg1, err := internalstorage.NewConfig(
		"lxd-btrfs",
		"lxd",
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	cfg2, err := internalstorage.NewConfig(
		"lxd-zfs",
		"lxd",
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	provider := NewMockProvider(ctrl)
	provider.EXPECT().DefaultPools().Return([]*internalstorage.Config{cfg1, cfg2})

	s.registry.Providers["lxd"] = provider

	model := description.NewModel(description.ModelArgs{})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "lxd-btrfs",
		Provider:   "lxd",
		Attributes: nil,
	})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "custom-pool",
		Provider:   "lxd",
		Attributes: nil,
	})

	svc := StoragePoolService{
		registryGetter: registryGetter{s.registry},
		logger:         loggertesting.WrapCheckLog(c),
	}

	pools, _, err := svc.GetStoragePoolsToImport(
		c.Context(),
		model.StoragePools(),
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 3)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	// User pool: lxd-btrfs.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "lxd-btrfs",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  nil,
	}))

	// User pool: custom-pool.
	c.Assert(pools[1], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "custom-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  nil,
	}))

	// Provider default (non-conflicting): lxd-zfs.
	c.Assert(pools[2], tc.DeepEquals, domainstorage.ImportStoragePoolParams{
		// Use the hardcoded UUID for this pool.
		UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
		Name:   "lxd-zfs",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Attrs:  nil,
	})
}

// TestGetStoragePoolsToImportReturnsRecommendedPools verifies that provider default
// pools are added and recommended storage pools are returned when the registry
// supplies recommendations for specific storage kinds.
func (s *storagePoolServiceSuite) TestGetStoragePoolsToImportReturnsRecommendedPools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool, _ := internalstorage.NewConfig("lxd", "lxd", internalstorage.Attrs{})
	zfsPool, _ := internalstorage.NewConfig("lxd-zfs", "lxd", internalstorage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := internalstorage.NewConfig("lxd-btrfs", "lxd", internalstorage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	recommendedPoolForBlock, _ := internalstorage.NewConfig("loop", "loop",
		internalstorage.Attrs{})
	lxdDefaultPools := []*internalstorage.Config{defaultPool, zfsPool, btrfsPool}

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{"lxd"}, nil,
	)
	s.storageProviderRegistry.EXPECT().StorageProvider(internalstorage.ProviderType("lxd")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindFilesystem).
		Return(defaultPool)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindBlock).
		Return(recommendedPoolForBlock)

	model := description.NewModel(description.ModelArgs{})
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "custom-pool",
		Provider:   "lxd",
		Attributes: map[string]any{"foo": "bar"},
	})
	svc := StoragePoolService{
		registryGetter: s.storageRegistryGetter,
		logger:         loggertesting.WrapCheckLog(c),
	}
	pools, recommended, err := svc.GetStoragePoolsToImport(
		c.Context(),
		model.StoragePools(),
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.SameContents, []domainstorage.RecommendedStoragePoolParams{
		// This is a pool name: lxd and provider: lxd.
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
		{
			StoragePoolUUID: "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad",
			StorageKind:     domainstorage.StorageKindBlock,
		},
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(pools, tc.HasLen, 5)
	// User pool. The UUID is generated by the service so we don't know the exact
	// value, but we assert that it is a UUID.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "custom-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
	// The rest are provider default pools.
	c.Assert(pools[1:], tc.SameContents, []domainstorage.ImportStoragePoolParams{
		{
			UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			Name:   "lxd",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
		{
			UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
			Name:   "lxd-zfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":        "zfs",
				"lxd-pool":      "juju-zfs",
				"zfs.pool_name": "juju-lxd",
			},
		},
		{
			UUID:   "e1acb8b8-c978-5d53-bc22-2a0e7fd58734",
			Name:   "lxd-btrfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			},
		},
		{
			UUID:   "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad",
			Name:   "loop",
			Type:   "loop",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
	})
}

// TestGetStoragePoolsToImportExcludeConflictingUserPool tests that if there is a recommended provider default
// pool with conflicting name with a user-defined pool, then that pool will not be included
// in the recommended pools because we cannot guarantee they refer to the same pool.
func (s *storagePoolServiceSuite) TestGetStoragePoolsToImportExcludeConflictingUserPool(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool, _ := internalstorage.NewConfig("lxd", "lxd", internalstorage.Attrs{})
	zfsPool, _ := internalstorage.NewConfig("lxd-zfs", "lxd", internalstorage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := internalstorage.NewConfig("lxd-btrfs", "lxd", internalstorage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	// This is the conflicting pool.
	recommendedPoolForBlock, _ := internalstorage.NewConfig("loop", "loop",
		internalstorage.Attrs{})
	lxdDefaultPools := []*internalstorage.Config{defaultPool, zfsPool, btrfsPool}

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{"lxd"}, nil,
	)
	s.storageProviderRegistry.EXPECT().StorageProvider(internalstorage.ProviderType("lxd")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindFilesystem).
		Return(defaultPool)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindBlock).
		Return(recommendedPoolForBlock)

	model := description.NewModel(description.ModelArgs{})
	// This has the same name as the provider loop pool but we cannot guarantee
	// they are refer to the same instance.
	model.AddStoragePool(description.StoragePoolArgs{
		Name:       "loop",
		Provider:   "loop",
		Attributes: map[string]any{"foo": "bar"},
	})
	svc := StoragePoolService{
		registryGetter: s.storageRegistryGetter,
		logger:         loggertesting.WrapCheckLog(c),
	}
	pools, recommended, err := svc.GetStoragePoolsToImport(
		c.Context(),
		model.StoragePools(),
	)

	c.Assert(err, tc.ErrorIsNil)
	// We assert that only the lxd pool is recommended.
	c.Assert(recommended, tc.DeepEquals, []domainstorage.RecommendedStoragePoolParams{
		// This is a pool name: lxd and provider: lxd.
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(pools, tc.HasLen, 4)
	// User pool. The UUID is generated by the service so we don't know the exact
	// value, but we assert that it is a UUID.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "loop",
		Type:   "loop",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
	// The rest are provider default pools.
	c.Assert(pools[1:], tc.SameContents, []domainstorage.ImportStoragePoolParams{
		{
			UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			Name:   "lxd",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
		{
			UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
			Name:   "lxd-zfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":        "zfs",
				"lxd-pool":      "juju-zfs",
				"zfs.pool_name": "juju-lxd",
			},
		},
		{
			UUID:   "e1acb8b8-c978-5d53-bc22-2a0e7fd58734",
			Name:   "lxd-btrfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			},
		},
	})
}

// TestGetStoragePoolsToImportRegistryGetterError asserts that an error propagated
// correctly when the storage provider registry returns an error.
func (s *storagePoolServiceSuite) TestGetStoragePoolsToImportRegistryGetterError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedErr := errors.New("registry down")

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(nil, expectedErr)

	svc := StoragePoolService{
		registryGetter: s.storageRegistryGetter,
		logger:         loggertesting.WrapCheckLog(c),
	}

	_, _, err := svc.GetStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider registry for model: registry down`)
}

// TestGetStoragePoolsToImportProviderTypesError asserts that an error is propagated
// correctly when fetching storage provider types returns an error.
func (s *storagePoolServiceSuite) TestGetStoragePoolsToImportProviderTypesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProviderTypes().
		Return(nil, errors.New("types boom"))

	svc := StoragePoolService{
		registryGetter: s.storageRegistryGetter,
		logger:         loggertesting.WrapCheckLog(c),
	}

	_, _, err := svc.GetStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider types for model storage registry: types boom`)
}

// TestGetStoragePoolsToImportStorageProviderError asserts that an error is propagated
// correctly when fetching a specific storage provider returns an error.
func (s *storagePoolServiceSuite) TestGetStoragePoolsToImportStorageProviderError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("lxd")).
		Return(nil, errors.New("provider boom"))

	svc := StoragePoolService{
		registryGetter: s.storageRegistryGetter,
		logger:         loggertesting.WrapCheckLog(c),
	}

	_, _, err := svc.GetStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider "lxd" from registry: provider boom`)
}
