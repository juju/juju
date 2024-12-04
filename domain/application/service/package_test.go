// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/application/service State,DeleteSecretState,ResourceStoreGetter,WatcherFactory,AgentVersionGetter,Provider,CharmStore
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	modelID model.UUID

	state               *MockState
	charm               *MockCharm
	secret              *MockDeleteSecretState
	charmStore          *MockCharmStore
	agentVersionGetter  *MockAgentVersionGetter
	provider            *MockProvider
	resourceStoreGetter *MockResourceStoreGetter

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	clock                 *testclock.Clock

	service *ProviderService
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	return s.setupMocksWithAtomic(c, func(ctx domain.AtomicContext) error {
		return errors.Errorf("not implemented %w", coreerrors.NotImplemented)
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
	s.charmStore = NewMockCharmStore(ctrl)
	s.resourceStoreGetter = NewMockResourceStoreGetter(ctrl)

	s.storageRegistryGetter = corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.resourceStoreGetter.EXPECT().AddStore(resource.TypeContainerImage, gomock.Any())

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewProviderService(
		s.state,
		s.secret,
		s.storageRegistryGetter,
		s.resourceStoreGetter,
		s.modelID,
		s.agentVersionGetter,
		fn,
		s.charmStore,
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
	s.charmStore = NewMockCharmStore(ctrl)
	s.resourceStoreGetter = NewMockResourceStoreGetter(ctrl)

	s.storageRegistryGetter = corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.resourceStoreGetter.EXPECT().AddStore(resource.TypeContainerImage, gomock.Any())

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewProviderService(
		s.state,
		s.secret,
		s.storageRegistryGetter,
		s.resourceStoreGetter,
		s.modelID,
		s.agentVersionGetter,
		func(ctx context.Context) (Provider, error) {
			return s.provider, nil
		},
		s.charmStore,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

	return ctrl
}

func (s *baseSuite) minimalManifest(c *gc.C) charm.Manifest {
	return charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}

func ptr[T any](v T) *T {
	return &v
}

type changeEvent struct {
	typ       changestream.ChangeType
	namespace string
	changed   string
}

var _ changestream.ChangeEvent = (*changeEvent)(nil)

func (c *changeEvent) Type() changestream.ChangeType {
	return c.typ
}

func (c *changeEvent) Namespace() string {
	return c.namespace
}

func (c *changeEvent) Changed() string {
	return c.changed
}
