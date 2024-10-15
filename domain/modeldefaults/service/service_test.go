// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/schema"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/environs"
)

type serviceSuite struct {
	modelConfigProvider *MockModelConfigProvider
	state               *MockState
	modelUUID           coremodel.UUID
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
}

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

// TestModelDefault is asserting the happy path of model defaults by showing
// that if everything is working we get a curated list of what we expect at the
// end. This includes some cases of demonstrating value override and also
// provider defaults.
func (s *serviceSuite) TestModelDefaults(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID, err := cloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetModelCloudUUID(gomock.Any(), s.modelUUID).Return(cloudUUID, nil)

	s.modelConfigProvider.EXPECT().ModelConfigDefaults(gomock.Any()).Return(
		map[string]any{
			"provider-default": "val",
			"override":         "val1",
		},
		nil,
	)

	s.modelConfigProvider.EXPECT().ConfigSchema().Return(
		schema.Fields{
			"provider-config-field":            schema.Int(),
			"provider-config-field-no-default": schema.Any(),
		},
	).AnyTimes()

	s.modelConfigProvider.EXPECT().ConfigDefaults().Return(
		schema.Defaults{
			"provider-config-field": 666,
		},
	).AnyTimes()

	s.state.EXPECT().CloudType(gomock.Any(), cloudUUID).Return(
		"dummy", nil,
	)

	s.state.EXPECT().ConfigDefaults(gomock.Any()).Return(
		map[string]any{
			"foo": "juju-default",
		},
	)

	s.state.EXPECT().CloudDefaults(gomock.Any(), cloudUUID).Return(
		map[string]string{
			"foo": "cloud-default",
		},
		nil,
	)

	s.state.EXPECT().ModelCloudRegionDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"foo":                   "cloud-region-default",
			"provider-config-field": "668",
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
	c.Assert(err, jc.ErrorIsNil)

	c.Check(defaults["provider-default"].Default, gc.Equals, "val")

	c.Check(defaults["provider-config-field"].Default, gc.Equals, int64(666))

	c.Check(defaults["provider-config-field"].Region, gc.Equals, int64(668))

	// This provider field doesn't have a default so it shouldn't be set
	c.Check(defaults["provider-config-field-no-default"].Default, gc.Equals, nil)

	c.Check(defaults["foo"].Default, gc.Equals, "juju-default")

	c.Check(defaults["foo"].Controller, gc.Equals, "cloud-default")

	c.Check(defaults["foo"].Region, gc.Equals, "cloud-region-default")

	c.Check(defaults["uuid"].Controller, gc.Equals, s.modelUUID.String())
}

// TestModelDefaultsModelNotFound is asserting of all the possible funcs that
// could return a model not found error that this is bubbled up via the service.
// We explicitly don't say which one will return the error here as it could be
// from any of the state methods and is implementation dependent.
func (s *serviceSuite) TestModelDefaultsModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID, err := cloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetModelCloudUUID(gomock.Any(), s.modelUUID).Return(cloudUUID, nil)

	s.state.EXPECT().CloudType(gomock.Any(), cloudUUID).Return(
		"", clouderrors.NotFound,
	)

	svc := NewService(s.modelConfigProviderFunc(c), s.state)
	_, err = svc.ModelDefaults(context.Background(), s.modelUUID)
	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)
}

// TestModelDefaultsProviderNotSupported is asserting that when we asked for the
// model defaults for a model and the provider doesn't support
// [environs.ModelConfigProvider] the model defaults don't error out and keep on
// going.
func (s *serviceSuite) TestModelDefaultsProviderNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID, err := cloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetModelCloudUUID(gomock.Any(), s.modelUUID).Return(cloudUUID, nil)

	providerGetter := func(cloud string) (environs.ModelConfigProvider, error) {
		c.Check(cloud, gc.Equals, "dummy")
		return nil, coreerrors.NotSupported
	}

	s.state.EXPECT().CloudType(gomock.Any(), cloudUUID).Return(
		"dummy", nil,
	)

	s.state.EXPECT().ConfigDefaults(gomock.Any()).Return(
		map[string]any{
			"foo": "juju-default",
		},
	)

	s.state.EXPECT().CloudDefaults(gomock.Any(), cloudUUID).Return(
		map[string]string{
			"foo": "cloud-default",
		},
		nil,
	)

	s.state.EXPECT().ModelCloudRegionDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"foo": "cloud-region-default",
		},
		nil,
	)

	s.state.EXPECT().ModelMetadataDefaults(gomock.Any(), s.modelUUID).Return(
		map[string]string{
			"uuid": s.modelUUID.String(),
		},
		nil,
	)

	svc := NewService(providerGetter, s.state)
	defaults, err := svc.ModelDefaults(context.Background(), s.modelUUID)
	c.Check(err, jc.ErrorIsNil)

	c.Check(defaults["foo"].Default, gc.Equals, "juju-default")

	c.Check(defaults["foo"].Controller, gc.Equals, "cloud-default")

	c.Check(defaults["foo"].Region, gc.Equals, "cloud-region-default")

	c.Check(defaults["uuid"].Controller, gc.Equals, s.modelUUID.String())
}

// TestModelDefaultsForNonExistentModel is here to establish that when we ask
// for model defaults for a model that does not exist we get back a error that
// satisfies [clouderrors.NotFound].
func (s *serviceSuite) TestModelDefaultsForNonExistentCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelCloudUUID(gomock.Any(), s.modelUUID).
		Return("", clouderrors.NotFound).Times(2)

	svc := NewService(s.modelConfigProviderFunc(c), s.state)

	defaults, err := svc.ModelDefaults(context.Background(), s.modelUUID)
	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)
	c.Assert(len(defaults), gc.Equals, 0)

	defaults, err = svc.ModelDefaultsProvider(s.modelUUID)(context.Background())
	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (s *serviceSuite) TestUpdateCloudDefaults(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID, err := cloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "test").Return(cloudUUID, nil)

	s.state.EXPECT().UpdateCloudDefaults(gomock.Any(), cloudUUID, map[string]string{"wallyworld": "peachy2", "lucifer": "668"})

	svc := NewService(s.modelConfigProviderFunc(c), s.state)

	attr := map[string]any{"wallyworld": "peachy2", "lucifer": 668}
	err = svc.UpdateCloudConfigDefaultValues(context.Background(), "test", attr)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveCloudDefaults(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID, err := cloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "test").Return(cloudUUID, nil)
	s.state.EXPECT().DeleteCloudDefaults(gomock.Any(), cloudUUID, []string{"wallyworld"})

	svc := NewService(s.modelConfigProviderFunc(c), s.state)

	err = svc.RemoveCloudDefaults(context.Background(), "test", []string{"wallyworld"})
	c.Assert(err, jc.ErrorIsNil)
}

// TestRemoveCloudDefaultsCloudNotFound is asserting that if we attempt to
// remove defaults for a cloud that does not exist we get back an error
// satisfying [clouderrors.NotFound].
func (s *serviceSuite) TestRemoveCloudDefaultsCloudNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "test").Return(cloud.UUID(""), clouderrors.NotFound)
	err := NewService(s.modelConfigProviderFunc(c), s.state).RemoveCloudDefaults(
		context.Background(),
		"test",
		nil,
	)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
}

func (s *serviceSuite) TestUpdateCloudRegionDefaults(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID, err := cloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "test").Return(cloudUUID, nil)

	s.state.EXPECT().UpdateCloudRegionDefaults(gomock.Any(), cloudUUID, "east", map[string]string{"wallyworld": "peachy2", "lucifer": "668"})

	svc := NewService(s.modelConfigProviderFunc(c), s.state)

	attr := map[string]any{"wallyworld": "peachy2", "lucifer": 668}
	err = svc.UpdateCloudRegionConfigDefaultValues(context.Background(), "test", "east", attr)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveCloudRegionDefaultValues(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID, err := cloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "test").Return(cloudUUID, nil)
	s.state.EXPECT().DeleteCloudRegionDefaults(gomock.Any(), cloudUUID, "east", []string{"wallyworld"})

	svc := NewService(s.modelConfigProviderFunc(c), s.state)
	err = svc.RemoveCloudRegionConfigDefaultValues(context.Background(), []string{"wallyworld"}, "test", "east")
	c.Assert(err, jc.ErrorIsNil)
}
