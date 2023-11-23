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
	"github.com/juju/juju/domain/modeldefaults/service/testing"
)

type serviceSuite struct{}

var _ = gc.Suite(&serviceSuite{})

// TestModelDefaultsForNonExistentModel is here to establish that when we ask
// for model defaults for a model that does not exist we get back a error that
// satisfies [modelerrors.NotFound].
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

// TestModelDefaultsProviderNotFound is testing the fact that if we can not find
// provider defaults for the models cloud that we handle the error gracefully
// internally and the service still returns defaults but just without any
// provider defaults. i.e we want a NotFound error from the provider to be
// transparent.
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
	c.Check(defaults["wallyworld"].V, gc.Equals, "peachy")
	c.Check(defaults["wallyworld"].Source, gc.Equals, "default")
	c.Check(defaults["foo"].V, gc.Equals, "bar")
	c.Check(defaults["foo"].Source, gc.Equals, "controller")

	defaults, err = svc.ModelDefaultsProvider(uuid)(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(defaults["wallyworld"].V, gc.Equals, "peachy")
	c.Check(defaults["wallyworld"].Source, gc.Equals, "default")
	c.Check(defaults["foo"].V, gc.Equals, "bar")
	c.Check(defaults["foo"].Source, gc.Equals, "controller")
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
	c.Check(defaults["wallyworld"].V, gc.Equals, "peachy")
	c.Check(defaults["wallyworld"].Source, gc.Equals, "default")
	c.Check(defaults["foo"].V, gc.Equals, "bar")
	c.Check(defaults["foo"].Source, gc.Equals, "controller")
	c.Check(defaults["bar"].V, gc.Equals, "foo")
	c.Check(defaults["bar"].Source, gc.Equals, "region")

	defaults, err = svc.ModelDefaultsProvider(uuid)(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(defaults["wallyworld"].V, gc.Equals, "peachy")
	c.Check(defaults["wallyworld"].Source, gc.Equals, "default")
	c.Check(defaults["foo"].V, gc.Equals, "bar")
	c.Check(defaults["foo"].Source, gc.Equals, "controller")
	c.Check(defaults["bar"].V, gc.Equals, "foo")
	c.Check(defaults["bar"].Source, gc.Equals, "region")
}
