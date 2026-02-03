// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/schema"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/errors"
)

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

type serviceSuite struct {
	mockState               *MockState
	mockModelConfigProvider *MockModelConfigProvider
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (f ModelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}

func noopDefaultsProvider() ModelDefaultsProvider {
	return ModelDefaultsProviderFunc(func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{}, nil
	})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)
	s.mockModelConfigProvider = NewMockModelConfigProvider(ctrl)
	return ctrl
}

func (s *serviceSuite) modelConfigProviderFunc(context.Context, string) (ModelConfigProvider, error) {
	return s.mockModelConfigProvider, nil
}

// TestGetModelConfigContainsAgentInformation checks that the models agent
// version and stream gets injected into the model config.
func (s *serviceSuite) TestGetModelConfigContainsAgentInformation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must0(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey:        "foo",
			config.UUIDKey:        modelUUID.String(),
			config.TypeKey:        "testprovider",
			config.AgentStreamKey: coreagentbinary.AgentStreamReleased.String(),
		}, nil,
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(schema.Fields{})

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.modelConfigProviderFunc, s.mockState)
	cfg, err := svc.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.AgentStream(), tc.Equals, coreagentbinary.AgentStreamReleased.String())
}

// TestUpdateModelConfigAgentStream checks that the model agent stream value
// cannot be changed via model config and if it is we get back an errors that
// satisfies [config.ValidationError].
func (s *serviceSuite) TestUpdateModelConfigAgentStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name":         "wallyworld",
			"uuid":         "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type":         "testprovider",
			"agent-stream": "released",
		}, nil,
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(schema.Fields{})

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.modelConfigProviderFunc, s.mockState)
	err := svc.UpdateModelConfig(
		c.Context(),
		map[string]any{
			"agent-stream": "proposed",
		},
		nil,
	)

	val, is := errors.AsType[*config.ValidationError](err)
	c.Check(is, tc.IsTrue)
	c.Check(val.InvalidAttrs, tc.DeepEquals, []string{"agent-stream"})
}

// TestUpdateModelConfigNoAgentStreamChange checks that the model agent stream
// does not change in model config results in no error and the value is removed
// from model config before persiting.
func (s *serviceSuite) TestUpdateModelConfigNoAgentStreamChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			"name":         "wallyworld",
			"uuid":         "a677bdfd-3c96-46b2-912f-38e25faceaf7",
			"type":         "testprovider",
			"agent-stream": "released",
		}, nil,
	)
	s.mockState.EXPECT().UpdateModelConfig(
		gomock.Any(),
		map[string]string{},
		gomock.Any(),
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(schema.Fields{})

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.modelConfigProviderFunc, s.mockState)
	err := svc.UpdateModelConfig(
		c.Context(),
		map[string]any{
			"agent-stream": "released",
		},
		nil,
	)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetModelConfigSchema(c *tc.C) {
	defer s.setupMocks(c).Finish()

	schema := configschema.Fields{
		"foo": configschema.Attr{
			Description: "a string",
			Type:        configschema.Tstring,
			Group:       configschema.EnvironGroup,
		},
		"bar": configschema.Attr{
			Description: "an integer",
			Type:        configschema.Tint,
			Group:       configschema.EnvironGroup,
		},
	}

	s.mockModelConfigProvider.EXPECT().Schema().Return(schema)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.modelConfigProviderFunc, s.mockState)
	res, err := svc.GetModelConfigSchemaForCloudType(c.Context(), "anytype")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, schema)
}

func (s *serviceSuite) TestGetModelConfigSchemaProviderDoesntSuppport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	providerGetter := func(context.Context, string) (ModelConfigProvider, error) {
		return nil, coreerrors.NotSupported
	}

	defaultSchema, err := config.Schema(nil)
	c.Assert(err, tc.ErrorIsNil)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), providerGetter, s.mockState)
	res, err := svc.GetModelConfigSchemaForCloudType(c.Context(), "sometype")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, defaultSchema)
}

// TestModelConfigWithProviderSchemaCoercion checks that provider-specific
// config attributes are coerced from strings to their proper types based on
// the provider's schema.
func (s *serviceSuite) TestModelConfigWithProviderSchemaCoercion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey:   "wallyworld",
			config.UUIDKey:   modelUUID.String(),
			config.TypeKey:   "testprovider",
			"provider-bool":  "true",
			"provider-int":   "42",
			"regular-string": "value",
		}, nil,
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(
		schema.Fields{
			"provider-bool": schema.Bool(),
			"provider-int":  schema.Int(),
		},
	)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.modelConfigProviderFunc, s.mockState)
	cfg, err := svc.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	attrs := cfg.AllAttrs()
	c.Check(attrs["provider-bool"], tc.Equals, true)
	c.Check(attrs["provider-int"], tc.Equals, int64(42))
	c.Check(attrs["regular-string"], tc.Equals, "value")
}

// TestModelConfigWithoutProviderGetter checks that ModelConfig returns an error
// when no provider getter is supplied.
func (s *serviceSuite) TestModelConfigWithoutProviderGetter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey: "wallyworld",
			config.UUIDKey: modelUUID.String(),
			config.TypeKey: "sometype",
		}, nil,
	)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), nil, s.mockState)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, "coercing provider config attributes:.*no model config provider getter")
}

// TestModelConfigWithProviderNotFound checks that ModelConfig returns an error
// when the provider is not found.
func (s *serviceSuite) TestModelConfigWithProviderNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey: "wallyworld",
			config.UUIDKey: modelUUID.String(),
			config.TypeKey: "unknown",
		}, nil,
	)

	providerGetter := func(ctx context.Context, cloudType string) (ModelConfigProvider, error) {
		c.Check(cloudType, tc.Equals, "unknown")
		return nil, errors.Errorf("unknown cloud type %q", "unknown").Add(coreerrors.NotFound)
	}

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), providerGetter, s.mockState)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, `coercing provider config attributes:.*unknown cloud type "unknown"`)
}

// TestModelConfigWithProviderEmptySchema checks that ModelConfig works correctly
// when the provider has an empty schema.
func (s *serviceSuite) TestModelConfigWithProviderEmptySchema(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey: "wallyworld",
			config.UUIDKey: modelUUID.String(),
			config.TypeKey: "testprovider",
		}, nil,
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(schema.Fields{})

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.modelConfigProviderFunc, s.mockState)
	cfg, err := svc.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.Name(), tc.Equals, "wallyworld")
}

func (s *serviceSuite) TestSetModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Controller: "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"name": "wallyworld",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	}

	s.mockState.EXPECT().SetModelConfig(gomock.Any(), map[string]string{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "bar",
		"logging-config": "<root>=INFO",
	})

	svc := NewService(defaults, config.ModelValidator(), nil, s.mockState)
	err := svc.SetModelConfig(c.Context(), attrs)
	c.Assert(err, tc.ErrorIsNil)
}

// TestModelConfigWithEmptyCloudType checks that ModelConfig handles the case
// where cloud type is empty string by converting without coercion.
func (s *serviceSuite) TestModelConfigWithEmptyCloudType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey: "wallyworld",
			config.UUIDKey: modelUUID.String(),
			config.TypeKey: "",
		}, nil,
	)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.modelConfigProviderFunc, s.mockState)
	_, err := svc.ModelConfig(c.Context())
	// Even though type is empty string, config.New requires a valid type
	c.Check(err, tc.ErrorMatches, ".*empty type in model configuration.*")
}

// TestModelConfigWithProviderReturnsNotSupportedError checks that when provider getter
// returns a NotSupported error and provider is nil, we get an error.
func (s *serviceSuite) TestModelConfigWithProviderReturnsNotSupportedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey: "wallyworld",
			config.UUIDKey: modelUUID.String(),
			config.TypeKey: "unsupportedtype",
		}, nil,
	)

	providerGetter := func(context.Context, string) (ModelConfigProvider, error) {
		return nil, errors.Errorf("unsupported").Add(coreerrors.NotSupported)
	}

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), providerGetter, s.mockState)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, "coercing provider config attributes:.*provider not found or doesn't support config schema")
}

// TestModelConfigWithProviderReturnsOtherError checks that non-NotSupported errors
// are properly propagated.
func (s *serviceSuite) TestModelConfigWithProviderReturnsOtherError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey: "wallyworld",
			config.UUIDKey: modelUUID.String(),
			config.TypeKey: "sometype",
		}, nil,
	)

	providerGetter := func(context.Context, string) (ModelConfigProvider, error) {
		return nil, errors.Errorf("some other error")
	}

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), providerGetter, s.mockState)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, "coercing provider config attributes:.*some other error")
}

// TestModelConfigCoercionError checks that coercion errors are properly propagated.
func (s *serviceSuite) TestModelConfigCoercionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.mockState.EXPECT().ModelConfig(gomock.Any()).Return(
		map[string]string{
			config.NameKey:  "wallyworld",
			config.UUIDKey:  modelUUID.String(),
			config.TypeKey:  "testprovider",
			"provider-bool": "not-a-bool",
		}, nil,
	)

	s.mockModelConfigProvider.EXPECT().ConfigSchema().Return(
		schema.Fields{
			"provider-bool": schema.Bool(),
		},
	)

	svc := NewService(noopDefaultsProvider(), config.ModelValidator(), s.modelConfigProviderFunc, s.mockState)
	_, err := svc.ModelConfig(c.Context())
	c.Check(err, tc.ErrorMatches, `.*coercing provider config attributes:.*provider-bool.*`)
}
