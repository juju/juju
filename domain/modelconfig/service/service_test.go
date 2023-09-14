// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/schema"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

type dummyConfigSchemaSource struct {
	defaults schema.Defaults
	schema   schema.Fields
}

type serviceSuite struct {
	cloudDefaultsState   *MockCloudDefaultsState
	cloudService         *MockCloudService
	staticConfigProvider *MockStaticConfigProvider
}

var _ = gc.Suite(&serviceSuite{})

// ConfigDefaults implements ConfigSchemaSource
func (d *dummyConfigSchemaSource) ConfigDefaults() schema.Defaults {
	return d.defaults
}

// ConfigSchema implements ConfigSchemaSource
func (d *dummyConfigSchemaSource) ConfigSchema() schema.Fields {
	return d.schema
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.cloudDefaultsState = NewMockCloudDefaultsState(ctrl)
	s.cloudService = NewMockCloudService(ctrl)
	s.staticConfigProvider = NewMockStaticConfigProvider(ctrl)
	return ctrl
}

func (s *serviceSuite) TestUpdateModelDefaultsWithBadSpec(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.cloudDefaultsState, s.cloudService, s.staticConfigProvider).
		UpdateModelDefaults(
			context.Background(),
			map[string]string{
				"wallyworld": "peachy",
				"foo":        "bar",
				"key":        "val",
			},
			[]string{"noexist"},
			cloudspec.CloudRegionSpec{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestUpdateModelDefaultsWithCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.cloudDefaultsState.EXPECT().UpdateCloudDefaults(
		gomock.Any(),
		"test",
		map[string]string{
			"wallyworld": "peachy",
			"foo":        "bar",
			"key":        "val",
		},
		[]string{"noexist"}).Return(nil)

	err := NewService(s.cloudDefaultsState, s.cloudService, s.staticConfigProvider).
		UpdateModelDefaults(
			context.Background(),
			map[string]string{
				"wallyworld": "peachy",
				"foo":        "bar",
				"key":        "val",
			},
			[]string{"noexist"},
			cloudspec.CloudRegionSpec{
				Cloud: "test",
			})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelDefaultsWithCloudRegion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.cloudDefaultsState.EXPECT().UpdateCloudRegionDefaults(
		gomock.Any(),
		"test",
		"region1",
		map[string]string{
			"wallyworld": "peachy",
			"foo":        "bar",
			"key":        "val",
		},
		[]string{"noexist"}).Return(nil)

	err := NewService(s.cloudDefaultsState, s.cloudService, s.staticConfigProvider).
		UpdateModelDefaults(
			context.Background(),
			map[string]string{
				"wallyworld": "peachy",
				"foo":        "bar",
				"key":        "val",
			},
			[]string{"noexist"},
			cloudspec.CloudRegionSpec{
				Cloud:  "test",
				Region: "region1",
			})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestModelDefaults(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.cloudService.EXPECT().Get(gomock.Any(), "test").Return(&cloud.Cloud{
		Name: "test",
		Type: "test",
	}, nil)

	s.staticConfigProvider.EXPECT().ConfigDefaults().Return(map[string]any{
		"juju-user": "wallyworld",
	})

	schemaSource := dummyConfigSchemaSource{
		defaults: schema.Defaults{
			"test1":      "val1",
			"wallyworld": "!peachy",
		},
		schema: schema.Fields{
			"test1":      schema.Any(),
			"wallyworld": schema.Any(),
		},
	}

	s.staticConfigProvider.EXPECT().CloudConfig("test").Return(&schemaSource, nil)

	s.cloudDefaultsState.EXPECT().CloudDefaults(gomock.Any(), "test").Return(map[string]string{
		"wallyworld": "peachy",
		"foo":        "bar",
	}, nil)

	s.cloudDefaultsState.EXPECT().CloudAllRegionDefaults(gomock.Any(), "test").Return(
		map[string]map[string]string{
			"region1": {
				"region1val": "val",
			},
		}, nil)

	result, err := NewService(s.cloudDefaultsState, s.cloudService, s.staticConfigProvider).
		ModelDefaults(context.Background(), "test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, config.ModelDefaultAttributes{
		"juju-user": config.AttributeDefaultValues{
			Default: "wallyworld",
		},
		"test1": config.AttributeDefaultValues{
			Default: "val1",
		},
		"wallyworld": config.AttributeDefaultValues{
			Default:    "!peachy",
			Controller: "peachy",
		},
		"foo": config.AttributeDefaultValues{
			Controller: "bar",
		},
		"region1val": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{{Name: "region1", Value: "val"}},
		},
	})
}
