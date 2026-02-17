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

	provider := NewMockProviderWithNetworking(ctrl)
	svc := &ProviderService{
		providerWithNetworking: func(context.Context) (ProviderWithNetworking, error) {
			return provider, nil
		},
	}

	supported, err := svc.supportsNetworking(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(supported, tc.IsTrue)
}

func (s *providerServiceSuite) TestSupportsNetworkingCachesNotSupported(c *tc.C) {
	svc := &ProviderService{
		providerWithNetworking: func(context.Context) (ProviderWithNetworking, error) {
			return nil, errors.Errorf("provider %w", coreerrors.NotSupported)
		},
	}

	supported, err := svc.supportsNetworking(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(supported, tc.IsFalse)
}
