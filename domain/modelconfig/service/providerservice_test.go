// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/schema"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

type providerServiceSuite struct {
	mockState               *MockProviderState
	mockModelConfigProvider *MockModelConfigProvider
}

func TestProviderServiceSuite(t *testing.T) {
	tc.Run(t, &providerServiceSuite{})
}

func (s *providerServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockProviderState(ctrl)
	s.mockModelConfigProvider = NewMockModelConfigProvider(ctrl)
	return ctrl
}

func (s *providerServiceSuite) modelConfigProviderFunc(cloudType string) ModelConfigProviderFunc {
	return func(context.Context, string) (ModelConfigProvider, error) {
		// In tests, we don't need to fetch the cloud type from state,
		// we just return the mock provider for the expected cloud type.
		return s.mockModelConfigProvider, nil
	}
}

func (s *providerServiceSuite) TestModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		},
		nil,
	)

	svc := NewProviderService(s.mockState, nil)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, "coercing provider config attributes:.*no model config provider getter")
}

// TestModelConfigWithProviderSchemaCoercion checks that provider-specific
// config attributes are coerced from strings to their proper types based on
// the provider's schema.
func (s *providerServiceSuite) TestModelConfigWithProviderSchemaCoercion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name":           "wallyworld",
			"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type":           "testprovider",
			"provider-bool":  "true",
			"provider-int":   "42",
			"regular-string": "value",
		},
		nil,
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(
		schema.Fields{
			"provider-bool": schema.Bool(),
			"provider-int":  schema.Int(),
		},
	)

	providerGetter := s.modelConfigProviderFunc("testprovider")

	svc := NewProviderService(s.mockState, providerGetter)
	cfg, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorIsNil)

	attrs := cfg.AllAttrs()
	c.Check(attrs["name"], tc.Equals, "wallyworld")
	c.Check(attrs["uuid"], tc.Equals, "a677bdfd-3c96-46b2-912f-38e25faceaf7")
	c.Check(attrs["type"], tc.Equals, "testprovider")
	c.Check(attrs["provider-bool"], tc.Equals, true)
	c.Check(attrs["provider-int"], tc.Equals, int64(42))
	c.Check(attrs["regular-string"], tc.Equals, "value")
}

// TestModelConfigWithoutProviderGetter checks that ModelConfig returns an error
// when no provider getter is supplied.
func (s *providerServiceSuite) TestModelConfigWithoutProviderGetter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "sometype",
		},
		nil,
	)

	svc := NewProviderService(s.mockState, nil)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, "coercing provider config attributes:.*no model config provider getter")
}

// TestModelConfigWithProviderNotFound checks that ModelConfig returns an error
// when the provider is not found.
func (s *providerServiceSuite) TestModelConfigWithProviderNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "unknown",
		},
		nil,
	)

	providerGetter := func(ctx context.Context, cloudType string) (ModelConfigProvider, error) {
		c.Check(cloudType, tc.Equals, "unknown")
		return nil, errors.Errorf("unknown cloud type %q", "unknown").Add(coreerrors.NotFound)
	}

	svc := NewProviderService(s.mockState, providerGetter)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, `coercing provider config attributes:.*unknown cloud type "unknown"`)
}

// TestModelConfigWithProviderEmptySchema checks that ModelConfig works correctly
// when the provider has an empty schema.
func (s *providerServiceSuite) TestModelConfigWithProviderEmptySchema(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "testprovider",
		},
		nil,
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(schema.Fields{})

	providerGetter := s.modelConfigProviderFunc("testprovider")

	svc := NewProviderService(s.mockState, providerGetter)
	cfg, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(cfg.Name(), tc.Equals, "wallyworld")
}

// TestModelConfigStateError checks that errors from the state layer are
// properly propagated.
func (s *providerServiceSuite) TestModelConfigStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		nil,
		errors.Errorf("database error"),
	)

	svc := NewProviderService(s.mockState, nil)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, "getting model config from state:.*database error")
}

// TestModelConfigWithEmptyCloudType checks that ModelConfig handles the case
// where cloud type is empty string by converting without coercion.
func (s *providerServiceSuite) TestModelConfigWithEmptyCloudType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "",
		},
		nil,
	)

	providerGetter := s.modelConfigProviderFunc("testprovider")

	svc := NewProviderService(s.mockState, providerGetter)
	_, err := svc.ModelConfig(c.Context())
	// Even though type is empty string, config.New requires a valid type
	c.Check(err, tc.ErrorMatches, ".*empty type in model configuration.*")
}

// TestModelConfigWithProviderReturnsNotSupportedError checks that when provider getter
// returns a NotSupported error and provider is nil, we get an error.
func (s *providerServiceSuite) TestModelConfigWithProviderReturnsNotSupportedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "unsupportedtype",
		},
		nil,
	)

	providerGetter := func(context.Context, string) (ModelConfigProvider, error) {
		return nil, errors.Errorf("unsupported").Add(coreerrors.NotSupported)
	}

	svc := NewProviderService(s.mockState, providerGetter)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, "coercing provider config attributes:.*provider not found or doesn't support config schema")
}

// TestModelConfigWithProviderReturnsOtherError checks that non-NotSupported errors
// are properly propagated.
func (s *providerServiceSuite) TestModelConfigWithProviderReturnsOtherError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name": "wallyworld",
			"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type": "sometype",
		},
		nil,
	)

	providerGetter := func(context.Context, string) (ModelConfigProvider, error) {
		return nil, errors.Errorf("some other error")
	}

	svc := NewProviderService(s.mockState, providerGetter)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, "coercing provider config attributes:.*some other error")
}

// TestModelConfigCoercionError checks that coercion errors are properly propagated.
func (s *providerServiceSuite) TestModelConfigCoercionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name":          "wallyworld",
			"uuid":          "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type":          "testprovider",
			"provider-bool": "not-a-bool",
		},
		nil,
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(
		schema.Fields{
			"provider-bool": schema.Bool(),
		},
	)

	providerGetter := s.modelConfigProviderFunc("testprovider")

	svc := NewProviderService(s.mockState, providerGetter)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, `.*coercing provider config key "provider-bool".*`)
}
