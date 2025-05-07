// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/uuid"
)

type providerSuite struct {
	testing.IsolationSuite

	provider *MockProvider

	providerFactory *MockProviderFactory
}

var _ = tc.Suite(&providerSuite{})

func (s *providerSuite) TestProviderRunner(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(s.provider, nil)

	runner := ProviderRunner[Provider](s.providerFactory, "foo")
	v, err := runner(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, s.provider)
}

func (s *providerSuite) TestProviderRunnerSubsetType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	provider := &fooProvider{Provider: s.provider}

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(provider, nil)

	runner := ProviderRunner[FooProvider](s.providerFactory, "foo")
	v, err := runner(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, provider)
}

func (s *providerSuite) TestProviderRunnerIsNotSubsetType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	provider := &fooProvider{Provider: s.provider}

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(provider, nil)

	runner := ProviderRunner[BarProvider](s.providerFactory, "foo")
	_, err := runner(context.Background())
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := EphemeralProviderConfig{
		ControllerUUID: uuid.MustNewUUID(),
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), config).Return(s.provider, nil)

	runner := EphemeralProviderRunnerFromConfig[Provider](s.providerFactory, config)

	var provider Provider
	err := runner(context.Background(), func(ctx context.Context, p Provider) error {
		provider = p
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(provider, tc.DeepEquals, s.provider)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfigSubsetType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := EphemeralProviderConfig{
		ControllerUUID: uuid.MustNewUUID(),
	}

	fooProvider := &fooProvider{
		Provider: s.provider,
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), config).Return(fooProvider, nil)

	runner := EphemeralProviderRunnerFromConfig[FooProvider](s.providerFactory, config)

	var provider FooProvider
	err := runner(context.Background(), func(ctx context.Context, p FooProvider) error {
		provider = p
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(provider, tc.DeepEquals, fooProvider)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfigIsNotSubsetType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := EphemeralProviderConfig{
		ControllerUUID: uuid.MustNewUUID(),
	}

	fooProvider := &fooProvider{
		Provider: s.provider,
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), config).Return(fooProvider, nil)

	runner := EphemeralProviderRunnerFromConfig[BarProvider](s.providerFactory, config)
	err := runner(context.Background(), func(ctx context.Context, p BarProvider) error {
		c.Fail()
		return nil
	})
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.provider = NewMockProvider(ctrl)

	s.providerFactory = NewMockProviderFactory(ctrl)

	return ctrl
}

type fooProvider struct {
	Provider
}

func (fooProvider) Hello() string {
	return "Hello"
}

type FooProvider interface {
	Hello() string
}

type BarProvider interface {
	World() string
}
