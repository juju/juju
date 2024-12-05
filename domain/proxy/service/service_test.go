// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	proxyerrors "github.com/juju/juju/domain/proxy/errors"
)

type serviceSuite struct {
	testing.IsolationSuite

	provider *MockProvider
	proxier  *MockProxier
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.provider = NewMockProvider(ctrl)
	s.proxier = NewMockProxier(ctrl)

	return ctrl
}

func (s *serviceSuite) TestGetConnectionProxyInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ConnectionProxyInfo(gomock.Any()).Return(s.proxier, nil)

	service := NewService(func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	})
	proxier, err := service.GetConnectionProxyInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(proxier, gc.Equals, s.proxier)
}

func (s *serviceSuite) TestGetConnectionProxyInfoNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	service := NewService(func(ctx context.Context) (Provider, error) {
		return s.provider, coreerrors.NotSupported
	})
	_, err := service.GetConnectionProxyInfo(context.Background())
	c.Assert(err, jc.ErrorIs, proxyerrors.ProxyInfoNotSupported)
}

func (s *serviceSuite) TestGetConnectionProxyInfoNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ConnectionProxyInfo(gomock.Any()).Return(s.proxier, coreerrors.NotFound)

	service := NewService(func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	})
	_, err := service.GetConnectionProxyInfo(context.Background())
	c.Assert(err, jc.ErrorIs, proxyerrors.ProxyInfoNotFound)
}

func (s *serviceSuite) TestGetProxyToApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ProxyToApplication(gomock.Any(), "foo", "8080").Return(s.proxier, nil)

	service := NewService(func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	})
	proxier, err := service.GetProxyToApplication(context.Background(), "foo", "8080")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(proxier, gc.Equals, s.proxier)
}

func (s *serviceSuite) TestGetProxyToApplicationNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	service := NewService(func(ctx context.Context) (Provider, error) {
		return s.provider, coreerrors.NotSupported
	})
	_, err := service.GetProxyToApplication(context.Background(), "foo", "8080")
	c.Assert(err, jc.ErrorIs, proxyerrors.ProxyNotSupported)
}
