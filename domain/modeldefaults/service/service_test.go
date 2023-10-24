// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/testing"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/domain/modeldefaults/service/testing"
)

type serviceSuite struct{}

var _ = gc.Suite(&serviceSuite{})

func (_ *serviceSuite) TestModelDefaultsForNonExistentModel(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	svc := NewService(&testing.State{})

	defaults, err := svc.ModelDefaults(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
	c.Assert(len(defaults), gc.Equals, 0)

	defaults, err = svc.ModelDefaultsProvider(uuid)(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
	c.Assert(len(defaults), gc.Equals, 0)
}

func (_ *serviceSuite) TestModelDefaultsProviderNotFound(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	svc := NewService(&testing.State{
		Defaults: map[string]any{
			"wallyworld": "peachy",
		},
		CloudDefaults: map[model.UUID]map[string]string{
			uuid: {
				"foo": "bar",
			},
		},
	})

	defaults, err := svc.ModelDefaults(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, modeldefaults.Defaults{
		"wallyworld": modeldefaults.DefaultAttributeValue{
			Default: "peachy",
		},
		"foo": modeldefaults.DefaultAttributeValue{
			Controller: "bar",
		},
	})

	defaults, err = svc.ModelDefaultsProvider(uuid)(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, modeldefaults.Defaults{
		"wallyworld": modeldefaults.DefaultAttributeValue{
			Default: "peachy",
		},
		"foo": modeldefaults.DefaultAttributeValue{
			Controller: "bar",
		},
	})
}

func (_ *serviceSuite) TestModelDefaults(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	svc := NewService(&testing.State{
		Defaults: map[string]any{
			"wallyworld": "peachy",
		},
		CloudDefaults: map[model.UUID]map[string]string{
			uuid: {
				"foo": "bar",
			},
		},
		CloudRegionDefaults: map[model.UUID]map[string]string{
			uuid: {
				"bar": "foo",
			},
		},
	})

	defaults, err := svc.ModelDefaults(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, modeldefaults.Defaults{
		"wallyworld": modeldefaults.DefaultAttributeValue{
			Default: "peachy",
		},
		"foo": modeldefaults.DefaultAttributeValue{
			Controller: "bar",
		},
		"bar": modeldefaults.DefaultAttributeValue{
			Region: "foo",
		},
	})

	defaults, err = svc.ModelDefaultsProvider(uuid)(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(defaults, jc.DeepEquals, modeldefaults.Defaults{
		"wallyworld": modeldefaults.DefaultAttributeValue{
			Default: "peachy",
		},
		"foo": modeldefaults.DefaultAttributeValue{
			Controller: "bar",
		},
		"bar": modeldefaults.DefaultAttributeValue{
			Region: "foo",
		},
	})
}
