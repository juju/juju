// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type providerSuite struct {
	testhelpers.IsolationSuite

	provider *MockProvider

	providerFactory *MockProviderFactory
}

type ephemeralProviderConfigGetter struct {
	EphemeralProviderConfig
}

func TestProviderSuite(t *testing.T) {
	tc.Run(t, &providerSuite{})
}

// GetEphemeralProviderConfig returns the ephemeral provider config set on this
// getter. This func implements the [EphemeralProviderConfigGetter] interface.
func (e *ephemeralProviderConfigGetter) GetEphemeralProviderConfig(
	_ context.Context,
) (EphemeralProviderConfig, error) {
	return e.EphemeralProviderConfig, nil
}

func (s *providerSuite) TestProviderRunner(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(s.provider, nil)

	runner := ProviderRunner[Provider](s.providerFactory, "foo")
	v, err := runner(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, s.provider)
}

func (s *providerSuite) TestProviderRunnerSubsetType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	provider := &fooProvider{Provider: s.provider}

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(provider, nil)

	runner := ProviderRunner[FooProvider](s.providerFactory, "foo")
	v, err := runner(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, provider)
}

func (s *providerSuite) TestProviderRunnerIsNotSubsetType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	provider := &fooProvider{Provider: s.provider}

	s.providerFactory.EXPECT().ProviderForModel(gomock.Any(), "foo").Return(provider, nil)

	runner := ProviderRunner[BarProvider](s.providerFactory, "foo")
	_, err := runner(c.Context())
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	configGetter := ephemeralProviderConfigGetter{
		EphemeralProviderConfig: EphemeralProviderConfig{
			ControllerUUID: uuid.MustNewUUID(),
		},
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), configGetter.EphemeralProviderConfig).Return(s.provider, nil)

	runner := EphemeralProviderRunnerFromConfig[Provider](s.providerFactory, &configGetter)

	var provider Provider
	err := runner(c.Context(), func(ctx context.Context, p Provider) error {
		provider = p
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(provider, tc.DeepEquals, s.provider)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfigSubsetType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	configGetter := ephemeralProviderConfigGetter{
		EphemeralProviderConfig: EphemeralProviderConfig{
			ControllerUUID: uuid.MustNewUUID(),
		},
	}

	fooProvider := &fooProvider{
		Provider: s.provider,
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), configGetter.EphemeralProviderConfig).Return(fooProvider, nil)

	runner := EphemeralProviderRunnerFromConfig[FooProvider](s.providerFactory, &configGetter)

	var provider FooProvider
	err := runner(c.Context(), func(ctx context.Context, p FooProvider) error {
		provider = p
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(provider, tc.DeepEquals, fooProvider)
}

func (s *providerSuite) TestEphemeralProviderRunnerFromConfigIsNotSubsetType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	configGetter := ephemeralProviderConfigGetter{
		EphemeralProviderConfig: EphemeralProviderConfig{
			ControllerUUID: uuid.MustNewUUID(),
		},
	}

	fooProvider := &fooProvider{
		Provider: s.provider,
	}

	s.providerFactory.EXPECT().EphemeralProviderFromConfig(gomock.Any(), configGetter.EphemeralProviderConfig).Return(fooProvider, nil)

	runner := EphemeralProviderRunnerFromConfig[BarProvider](s.providerFactory, &configGetter)
	err := runner(c.Context(), func(ctx context.Context, p BarProvider) error {
		c.Fail()
		return nil
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
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
