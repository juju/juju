// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	domaintesting "github.com/juju/juju/domain/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/application/service State,DeleteSecretState,ResourceStoreGetter,WatcherFactory,AgentVersionGetter,Provider
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	modelID model.UUID

	state              *MockState
	charm              *MockCharm
	secret             *MockDeleteSecretState
	agentVersionGetter *MockAgentVersionGetter
	provider           *MockProvider

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	clock                 *testclock.Clock

	service *ProviderService
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	return s.setupMocksWithAtomic(c, func(ctx domain.AtomicContext) error {
		return errors.NotImplementedf("not implemented")
	})
}

func (s *baseSuite) setupMocksWithProvider(c *gc.C, fn func(ctx context.Context) (Provider, error)) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelID = modeltesting.GenModelUUID(c)

	s.agentVersionGetter = NewMockAgentVersionGetter(ctrl)
	s.provider = NewMockProvider(ctrl)

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.secret = NewMockDeleteSecretState(ctrl)

	s.storageRegistryGetter = corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewProviderService(
		s.state,
		s.secret,
		s.storageRegistryGetter,
		s.modelID,
		s.agentVersionGetter,
		fn,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	return ctrl
}

// Deprecated: atomic context is deprecated.
func (s *baseSuite) setupMocksWithAtomic(c *gc.C, fn func(domain.AtomicContext) error) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelID = modeltesting.GenModelUUID(c)

	s.agentVersionGetter = NewMockAgentVersionGetter(ctrl)
	s.provider = NewMockProvider(ctrl)

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.secret = NewMockDeleteSecretState(ctrl)

	s.storageRegistryGetter = corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewProviderService(
		s.state,
		s.secret,
		s.storageRegistryGetter,
		s.modelID,
		s.agentVersionGetter,
		func(ctx context.Context) (Provider, error) {
			return s.provider, nil
		},
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}
