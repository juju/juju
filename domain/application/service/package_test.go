// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go -source=./service.go
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination provider_mock_test.go -source=./provider.go
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/application/service CharmStore,StatusHistory
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination internal_charm_mock_test.go github.com/juju/juju/internal/charm Charm
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination constraints_mock_test.go github.com/juju/juju/core/constraints Validator
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination caas_mock_test.go github.com/juju/juju/caas Application

type baseSuite struct {
	testhelpers.IsolationSuite

	modelID model.UUID

	state              *MockState
	charm              *MockCharm
	charmStore         *MockCharmStore
	agentVersionGetter *MockAgentVersionGetter
	provider           *MockProvider
	caasProvider       *MockCAASProvider
	leadership         *MockEnsurer
	validator          *MockValidator

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	clock                 *testclock.Clock

	service *ProviderService
}

func noProviderError() error {
	return nil
}

func providerNotSupported() error {
	return coreerrors.NotSupported
}

func (s *baseSuite) setupMocksWithProvider(
	c *tc.C,
	providerGetterError func() error,
	caasProviderGetterError func() error,
) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelID = modeltesting.GenModelUUID(c)

	s.agentVersionGetter = NewMockAgentVersionGetter(ctrl)
	s.provider = NewMockProvider(ctrl)
	s.caasProvider = NewMockCAASProvider(ctrl)
	s.leadership = NewMockEnsurer(ctrl)

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.charmStore = NewMockCharmStore(ctrl)
	s.validator = NewMockValidator(ctrl)

	s.storageRegistryGetter = corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewProviderService(
		s.state,
		s.leadership,
		s.storageRegistryGetter,
		s.modelID,
		s.agentVersionGetter,
		func(ctx context.Context) (Provider, error) {
			if err := providerGetterError(); err != nil {
				return nil, err
			}
			return s.provider, nil
		},
		func(ctx context.Context) (CAASProvider, error) {
			if err := caasProviderGetterError(); err != nil {
				return nil, err
			}
			return s.caasProvider, nil
		},
		s.charmStore,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	return ctrl
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	return s.setupMocksWithStatusHistory(c, func(ctrl *gomock.Controller) StatusHistory {
		return domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock)
	})
}

func (s *baseSuite) setupMocksWithStatusHistory(c *tc.C, fn func(*gomock.Controller) StatusHistory) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelID = modeltesting.GenModelUUID(c)

	s.agentVersionGetter = NewMockAgentVersionGetter(ctrl)
	s.provider = NewMockProvider(ctrl)
	s.caasProvider = NewMockCAASProvider(ctrl)
	s.leadership = NewMockEnsurer(ctrl)

	s.state = NewMockState(ctrl)
	s.charm = NewMockCharm(ctrl)
	s.charmStore = NewMockCharmStore(ctrl)
	s.validator = NewMockValidator(ctrl)

	s.storageRegistryGetter = corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
		return storage.ChainedProviderRegistry{
			dummystorage.StorageProviders(),
			provider.CommonStorageProviders(),
		}
	})

	s.clock = testclock.NewClock(time.Time{})
	s.service = NewProviderService(
		s.state,
		s.leadership,
		s.storageRegistryGetter,
		s.modelID,
		s.agentVersionGetter,
		func(ctx context.Context) (Provider, error) {
			return s.provider, nil
		},
		func(ctx context.Context) (CAASProvider, error) {
			return s.caasProvider, nil
		},
		s.charmStore,
		fn(ctrl),
		s.clock,
		loggertesting.WrapCheckLog(c),
	)
	s.service.clock = s.clock

	return ctrl

}

func (s *baseSuite) minimalManifest() charm.Manifest {
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
