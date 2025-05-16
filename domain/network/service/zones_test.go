// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type zonesSuite struct {
	testhelpers.IsolationSuite

	st                                *MockState
	providerWithNetworking            *MockProviderWithNetworking
	providerWithZones                 *MockProviderWithZones
	networkProviderGetter             func(context.Context) (ProviderWithNetworking, error)
	notSupportedNetworkProviderGetter func(context.Context) (ProviderWithNetworking, error)
	zoneProviderGetter                func(context.Context) (ProviderWithZones, error)
	notSupportedZoneProviderGetter    func(context.Context) (ProviderWithZones, error)
}

func TestZonesSuite(t *stdtesting.T) { tc.Run(t, &zonesSuite{}) }
func (s *zonesSuite) setupMocks(c *tc.C) *gomock.Controller {
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

func (s *zonesSuite) TestGetProviderAvailabilityZones(c *tc.C) {
	defer s.setupMocks(c).Finish()

	zones := network.AvailabilityZones{}
	s.providerWithZones.EXPECT().AvailabilityZones(gomock.Any()).Return(zones, nil)

	providerService := NewProviderService(s.st, s.notSupportedNetworkProviderGetter, s.zoneProviderGetter, loggertesting.WrapCheckLog(c))

	got, err := providerService.GetProviderAvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, zones)
}

func (s *zonesSuite) TestGetProviderAvailabilityZonesNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	providerService := NewProviderService(s.st, s.networkProviderGetter, s.notSupportedZoneProviderGetter, loggertesting.WrapCheckLog(c))

	zones, err := providerService.GetProviderAvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(zones, tc.HasLen, 0)
}
