// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

type providerServiceSuite struct{}

func TestProviderServiceSuite(t *testing.T) {
	tc.Run(t, &providerServiceSuite{})
}

func (s *providerServiceSuite) TestSupportsNetworkingCachesSuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	calls := 0
	provider := NewMockProviderWithNetworking(ctrl)
	svc := &ProviderService{
		providerWithNetworking: func(context.Context) (ProviderWithNetworking, error) {
			calls++
			return provider, nil
		},
	}

	supported, err := svc.supportsNetworking(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(supported, tc.IsTrue)

	supported, err = svc.supportsNetworking(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(supported, tc.IsTrue)
	c.Check(calls, tc.Equals, 1)
}

func (s *providerServiceSuite) TestSupportsNetworkingCachesNotSupported(c *tc.C) {
	calls := 0
	svc := &ProviderService{
		providerWithNetworking: func(context.Context) (ProviderWithNetworking, error) {
			calls++
			return nil, errors.Errorf("provider %w", coreerrors.NotSupported)
		},
	}

	supported, err := svc.supportsNetworking(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(supported, tc.IsFalse)

	supported, err = svc.supportsNetworking(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(supported, tc.IsFalse)
	c.Check(calls, tc.Equals, 1)
}

func (s *providerServiceSuite) TestSupportsNetworkingDoesNotCacheError(c *tc.C) {
	calls := 0
	svc := &ProviderService{
		providerWithNetworking: func(context.Context) (ProviderWithNetworking, error) {
			calls++
			return nil, errors.New("boom")
		},
	}

	supported, err := svc.supportsNetworking(c.Context())
	c.Assert(supported, tc.IsFalse)
	c.Assert(err, tc.ErrorMatches, "boom")

	supported, err = svc.supportsNetworking(c.Context())
	c.Assert(supported, tc.IsFalse)
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Check(calls, tc.Equals, 2)
}
