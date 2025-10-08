package service

import (
	"context"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
)

type baseSuite struct {
	testhelpers.IsolationSuite

	mockControllerState        *MockControllerState
	mockCloudInfoProvider      *MockCloudInfoProvider
	mockEnvironVersionProvider *MockEnvironVersionProvider
	mockModelState             *MockModelState
	mockModelResourceProvider  *MockModelResourcesProvider
	mockProviderRegistry       *MockProviderRegistry
	mockRegionProvider         *MockRegionProvider
}

// storageProviderRegistryGetterFunc provides a func type that implements
// [service.StorageProviderRegistryGetter].
type storageProviderRegistryGetterFunc func(
	context.Context,
) (internalstorage.ProviderRegistry, error)

// environVersionProviderGetter provides a test implementation of
// [EnvironVersionProviderFunc] that uses the mocked [EnvironVersionProvider] on
// this suite.
func (s *baseSuite) environVersionProviderGetter() EnvironVersionProviderFunc {
	return func(string) (EnvironVersionProvider, error) {
		return s.mockEnvironVersionProvider, nil
	}
}

// GetStorageRegistry return the result of calling
// [storageProviderRegistryGetterFunc].
func (s storageProviderRegistryGetterFunc) GetStorageRegistry(
	c context.Context,
) (internalstorage.ProviderRegistry, error) {
	return s(c)
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockControllerState = NewMockControllerState(ctrl)
	s.mockCloudInfoProvider = NewMockCloudInfoProvider(ctrl)
	s.mockEnvironVersionProvider = NewMockEnvironVersionProvider(ctrl)
	s.mockModelState = NewMockModelState(ctrl)
	s.mockModelResourceProvider = NewMockModelResourcesProvider(ctrl)
	s.mockProviderRegistry = NewMockProviderRegistry(ctrl)
	s.mockRegionProvider = NewMockRegionProvider(ctrl)

	c.Cleanup(func() {
		s.mockControllerState = nil
		s.mockCloudInfoProvider = nil
		s.mockEnvironVersionProvider = nil
		s.mockEnvironVersionProvider = nil
		s.mockModelState = nil
		s.mockModelResourceProvider = nil
		s.mockProviderRegistry = nil
		s.mockRegionProvider = nil
	})
	return ctrl
}

func (s *baseSuite) storageProviderRegistryGetter() storageProviderRegistryGetterFunc {
	return func(_ context.Context) (internalstorage.ProviderRegistry, error) {
		return s.mockProviderRegistry, nil
	}
}
