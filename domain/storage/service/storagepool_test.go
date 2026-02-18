// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

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
