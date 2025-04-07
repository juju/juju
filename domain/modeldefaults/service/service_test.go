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
	cloudtesting "github.com/juju/juju/core/cloud/testing"
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
	err = svc.UpdateCloudDefaults(context.Background(), "test", attr)
	c.Assert(err, jc.ErrorIsNil)
}

// TestUpdateCloudDefaultsNotFound is asserting that is we try and update the
// cloud defaults for a cloud that does not exist we get back an error that
// satisfies [clouderrors.NotFound].
func (s *serviceSuite) TestUpdateCloudDefaultsNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "noexist").Return(
		cloud.UUID(""), clouderrors.NotFound,
	).AnyTimes()

	err := NewService(s.modelConfigProviderFunc(c), s.state).UpdateCloudDefaults(
		context.Background(),
		"noexist",
		map[string]any{
			"foo": "bar",
		},
	)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)

	// Assert that if we still pass a cloud that doesn't exist but would be
	// considered a noop we still check the cloud exists.
	err = NewService(s.modelConfigProviderFunc(c), s.state).UpdateCloudDefaults(
		context.Background(),
		"noexist",
		nil,
	)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
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
	err = svc.UpdateCloudRegionDefaults(context.Background(), "test", "east", attr)
	c.Assert(err, jc.ErrorIsNil)
}

// TestUpdateCloudRegionDefaultsNotFoundCloud is a test to assert that if the
// cloud does not exist when trying to update cloud region defaults we get back
// an error that satisfies [clouderrors.NotFound].
func (s *serviceSuite) TestUpdateCloudRegionDefaultsNotFoundCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "noexist").Return(cloud.UUID(""), clouderrors.NotFound)
	err := NewService(s.modelConfigProviderFunc(c), s.state).UpdateCloudRegionDefaults(
		context.Background(),
		"noexist",
		"east",
		map[string]any{"foo": "bar"},
	)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
}

// TestUpdateCloudRegionDeaultsNotFoundRegion is a test to assert that if the
// cloud region does not exist when trying to update cloud region defaults we
// get back an error that satisfies [clouderrors.NotFound].
func (s *serviceSuite) TestUpdateCloudRegionDefaultsNotFoundRegion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID := cloudtesting.GenCloudUUID(c)

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "foo").Return(cloudUUID, nil)
	s.state.EXPECT().UpdateCloudRegionDefaults(
		gomock.Any(), cloudUUID, "east", gomock.Any(),
	).Return(clouderrors.NotFound)

	err := NewService(s.modelConfigProviderFunc(c), s.state).UpdateCloudRegionDefaults(
		context.Background(),
		"foo",
		"east",
		map[string]any{"foo": "bar"},
	)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
}

func (s *serviceSuite) TestRemoveCloudRegionDefaultValues(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID, err := cloud.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "test").Return(cloudUUID, nil)
	s.state.EXPECT().DeleteCloudRegionDefaults(gomock.Any(), cloudUUID, "east", []string{"wallyworld"})

	svc := NewService(s.modelConfigProviderFunc(c), s.state)
	err = svc.RemoveCloudRegionDefaults(context.Background(), "test", "east", []string{"wallyworld"})
	c.Assert(err, jc.ErrorIsNil)
}

// TestRemoveCloudRegionDefaultsCloudNotFound is testing that if we attempt to
// remove cloud region defaults for a cloud that doesn't exist we get back a
// [clouderrors.NotFound] error.
func (s *serviceSuite) TestRemoveCloudRegionDefaultsCloudNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetCloudUUID(gomock.Any(), "noexist").Return(cloud.UUID(""), clouderrors.NotFound)

	err := NewService(s.modelConfigProviderFunc(c), s.state).RemoveCloudRegionDefaults(
		context.Background(),
		"noexist",
		"east",
		[]string{"foo"},
	)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
}

// TestRemoveCloudRegionDefaultsCloudRegionNotFound is asserting that we try
// to remove default attributes for a cloud region and the region doesn't exist
// we get back a [clouderrors.NotFound] error.
func (s *serviceSuite) TestRemoveCloudRegionDefaultsCloudRegionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloudUUID := cloudtesting.GenCloudUUID(c)
	s.state.EXPECT().GetCloudUUID(gomock.Any(), "foo").Return(cloudUUID, nil)
	s.state.EXPECT().DeleteCloudRegionDefaults(
		gomock.Any(),
		cloudUUID,
		"east",
		[]string{"foo"},
	).Return(clouderrors.NotFound)

	err := NewService(s.modelConfigProviderFunc(c), s.state).RemoveCloudRegionDefaults(
		context.Background(),
		"foo",
		"east",
		[]string{"foo"},
	)
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
}

// TestModelDefaultsNoProviderDefaults is a regression test to ensure a bug fix.
// With this test we want to see that provider model defaults are not populated
// in model default values set by the user. More specifically if a provider says
// that the hard coded default value for key "foo" is "bar" we need to be
// certain that when reading model default values set by a user for either a
// cloud or a region that "foo" is not set unless the user has explicitly done
// this.
func (s *serviceSuite) TestModelDefaultsNoProviderDefaults(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	cloudUUID := cloudtesting.GenCloudUUID(c)

	s.state.EXPECT().GetModelCloudUUID(gomock.Any(), modelUUID).Return(cloudUUID, nil)
	s.state.EXPECT().CloudDefaults(gomock.Any(), cloudUUID).Return(map[string]string{}, nil)
	s.state.EXPECT().ConfigDefaults(gomock.Any()).Return(nil)
	s.state.EXPECT().CloudType(gomock.Any(), cloudUUID).Return("dummy", nil)
	s.state.EXPECT().ModelCloudRegionDefaults(gomock.Any(), modelUUID).Return(map[string]string{}, nil)
	s.state.EXPECT().ModelMetadataDefaults(gomock.Any(), modelUUID).Return(map[string]string{
		"uuid": modelUUID.String(),
		"name": "test",
		"type": "dummy",
	}, nil)

	s.modelConfigProvider.EXPECT().ConfigSchema().Return(schema.Fields{
		"test-provider-key": schema.String(),
	}).AnyTimes()
	s.modelConfigProvider.EXPECT().ConfigDefaults().Return(schema.Defaults{
		"test-provider-key": "val",
	}).AnyTimes()
	s.modelConfigProvider.EXPECT().ModelConfigDefaults(gomock.Any()).Return(nil, nil)

	defaults, err := NewService(s.modelConfigProviderFunc(c), s.state).ModelDefaults(
		context.Background(),
		modelUUID,
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults["test-provider-key"].Region, gc.IsNil)
	c.Check(defaults["test-provider-key"].Controller, gc.IsNil)
}
