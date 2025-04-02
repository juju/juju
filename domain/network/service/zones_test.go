// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type zonesSuite struct {
	testing.IsolationSuite

	st                                *MockState
	providerWithNetworking            *MockProviderWithNetworking
	providerWithZones                 *MockProviderWithZones
	networkProviderGetter             func(context.Context) (ProviderWithNetworking, error)
	notSupportedNetworkProviderGetter func(context.Context) (ProviderWithNetworking, error)
	zoneProviderGetter                func(context.Context) (ProviderWithZones, error)
	notSupportedZoneProviderGetter    func(context.Context) (ProviderWithZones, error)
}

var _ = gc.Suite(&zonesSuite{})

func (s *zonesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.providerWithNetworking = NewMockProviderWithNetworking(ctrl)
	s.networkProviderGetter = func(ctx context.Context) (ProviderWithNetworking, error) {
		return s.providerWithNetworking, nil
	}
	s.notSupportedNetworkProviderGetter = func(ctx context.Context) (ProviderWithNetworking, error) {
		return nil, errors.Errorf("provider %w", coreerrors.NotSupported)
	}

	s.providerWithZones = NewMockProviderWithZones(ctrl)
	s.zoneProviderGetter = func(ctx context.Context) (ProviderWithZones, error) {
		return s.providerWithZones, nil
	}
	s.notSupportedZoneProviderGetter = func(ctx context.Context) (ProviderWithZones, error) {
		return nil, errors.Errorf("provider %w", coreerrors.NotSupported)
	}

	return ctrl
}

func (s *zonesSuite) TestGetProviderAvailabilityZones(c *gc.C) {
	defer s.setupMocks(c).Finish()

	zones := network.AvailabilityZones{}
	s.providerWithZones.EXPECT().AvailabilityZones(gomock.Any()).Return(zones, nil)

	providerService := NewProviderService(s.st, s.notSupportedNetworkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))

	got, err := providerService.GetProviderAvailabilityZones(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, jc.DeepEquals, zones)
}

func (s *zonesSuite) TestGetProviderAvailabilityZonesNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.notSupportedZoneProviderGetter, loggertesting.WrapCheckLog(c))

	zones, err := providerService.GetProviderAvailabilityZones(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(zones, gc.HasLen, 0)
}
