// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	schema "github.com/juju/schema"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type serviceSuite struct {
	modelConfigProvider *MockModelConfigProvider
	state               *MockState
	modelUUID           coremodel.UUID
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) modelConfigProviderFunc(c *gc.C) ModelConfigProviderFunc {
	return func(_ string) (environs.ModelConfigProvider, error) {
		return s.modelConfigProvider, nil
	}
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.modelConfigProvider = NewMockModelConfigProvider(ctrl)
	return ctrl
}

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
}

// TestModelDefault is asserting the happy path of model defaults by showing
// that if everything is working we get a curated list of what we expect at the
// end. This includes some cases of demonstrating value override and also
// provider defaults.
func (s *serviceSuite) TestModelDefaults(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelConfigProvider.EXPECT().ModelConfigDefaults(gomock.Any()).Return(
		map[string]any{
			"provider-default": "val",
			"override":         "val1",
		},
		nil,
	)

	s.modelConfigProvider.EXPECT().ConfigSchema().Return(
		schema.Fields{
			"provider-config-field":            schema.Any(),
			"provider-config-field-no-default": schema.Any(),
		},
	)

	s.modelConfigProvider.EXPECT().ConfigDefaults().Return(
		schema.Defaults{
			"provider-config-field": "val",
		},
	)

	s.state.EXPECT().ModelCloudType(gomock.Any(), s.modelUUID).Return(
		"dummy", nil,
	)

	s.state.EXPECT().ConfigDefaults(gomock.Any()).Return(
		map[string]any{
			"juju-default": "val",
		},
	)

	s.state.EXPECT().ModelCloudDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"cloud-default": "val",
		},
		nil,
	)

	s.state.EXPECT().ModelCloudRegionDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"cloud-region-default": "val",
			"override":             "val2",
		},
		nil,
	)

	s.state.EXPECT().ModelMetadataDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"uuid": s.modelUUID.String(),
		},
		nil,
	)

	svc := NewService(s.modelConfigProviderFunc(c), s.state)
	defaults, err := svc.ModelDefaults(context.Background(), s.modelUUID)
	c.Check(err, jc.ErrorIsNil)

	c.Check(defaults["provider-default"].Value, gc.Equals, "val")
	c.Check(defaults["provider-default"].Source, gc.Equals, config.JujuDefaultSource)

	c.Check(defaults["provider-config-field"].Value, gc.Equals, "val")
	c.Check(defaults["provider-config-field"].Source, gc.Equals, config.JujuDefaultSource)

	// This provider field doesn't have a default so it shouldn't be set
	c.Check(defaults["provider-config-field-no-default"].Value, gc.Equals, nil)

	c.Check(defaults["juju-default"].Value, gc.Equals, "val")
	c.Check(defaults["juju-default"].Source, gc.Equals, config.JujuDefaultSource)

	c.Check(defaults["cloud-default"].Value, gc.Equals, "val")
	c.Check(defaults["cloud-default"].Source, gc.Equals, config.JujuControllerSource)

	c.Check(defaults["cloud-region-default"].Value, gc.Equals, "val")
	c.Check(defaults["cloud-region-default"].Source, gc.Equals, config.JujuRegionSource)

	c.Check(defaults["override"].Value, gc.Equals, "val2")
	c.Check(defaults["override"].Source, gc.Equals, config.JujuRegionSource)

	c.Check(defaults["uuid"].Value, gc.Equals, s.modelUUID.String())
	c.Check(defaults["uuid"].Source, gc.Equals, config.JujuControllerSource)
}

// TestModelDefaultsModelNotFound is asserting of all the possible funcs that
// could return a model not found error that this is bubbled up via the service.
// We explicitly don't say which one will return the error here as it could be
// from any of the state methods and is implementation dependent.
func (s *serviceSuite) TestModelDefaultsModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ConfigDefaults(gomock.Any()).Return(nil)

	s.state.EXPECT().ModelCloudDefaults(gomock.Any(), s.modelUUID).Return(
		nil, modelerrors.NotFound,
	).AnyTimes()

	s.state.EXPECT().ModelCloudRegionDefaults(gomock.Any(), s.modelUUID).Return(
		nil, modelerrors.NotFound,
	).AnyTimes()

	s.state.EXPECT().ModelCloudType(gomock.Any(), s.modelUUID).Return(
		"", modelerrors.NotFound,
	).AnyTimes()

	s.state.EXPECT().ModelMetadataDefaults(gomock.Any(), s.modelUUID).Return(
		nil, modelerrors.NotFound,
	).AnyTimes()

	svc := NewService(s.modelConfigProviderFunc(c), s.state)
	_, err := svc.ModelDefaults(context.Background(), s.modelUUID)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestModelDefaultsProviderNotSupported is asserting that when we asked for the
// model defaults for a model and the provider doesn't support
// [environs.ModelConfigProvider] the model defaults don't error out and keep on
// going.
func (s *serviceSuite) TestModelDefaultsProviderNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	providerGetter := func(cloud string) (environs.ModelConfigProvider, error) {
		c.Check(cloud, gc.Equals, "dummy")
		return nil, coreerrors.NotSupported
	}

	s.state.EXPECT().ModelCloudType(gomock.Any(), s.modelUUID).Return(
		"dummy", nil,
	)

	s.state.EXPECT().ConfigDefaults(gomock.Any()).Return(
		map[string]any{
			"juju-default": "val",
		},
	)

	s.state.EXPECT().ModelCloudDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"cloud-default": "val",
		},
		nil,
	)

	s.state.EXPECT().ModelCloudRegionDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"cloud-region-default": "val",
			"override":             "val2",
		},
		nil,
	)

	s.state.EXPECT().ModelMetadataDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"uuid": s.modelUUID.String(),
		},
		nil,
	)

	svc := NewService(ModelConfigProviderFunc(providerGetter), s.state)
	defaults, err := svc.ModelDefaults(context.Background(), s.modelUUID)
	c.Check(err, jc.ErrorIsNil)

	c.Check(defaults["juju-default"].Value, gc.Equals, "val")
	c.Check(defaults["juju-default"].Source, gc.Equals, config.JujuDefaultSource)

	c.Check(defaults["cloud-default"].Value, gc.Equals, "val")
	c.Check(defaults["cloud-default"].Source, gc.Equals, config.JujuControllerSource)

	c.Check(defaults["cloud-region-default"].Value, gc.Equals, "val")
	c.Check(defaults["cloud-region-default"].Source, gc.Equals, config.JujuRegionSource)

	c.Check(defaults["override"].Value, gc.Equals, "val2")
	c.Check(defaults["override"].Source, gc.Equals, config.JujuRegionSource)

	c.Check(defaults["uuid"].Value, gc.Equals, s.modelUUID.String())
	c.Check(defaults["uuid"].Source, gc.Equals, config.JujuControllerSource)
}
