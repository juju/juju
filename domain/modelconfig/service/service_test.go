// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	statetesting "github.com/juju/juju/domain/modelconfig/state/testing"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
)

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

type serviceSuite struct {
}

var _ = gc.Suite(&serviceSuite{})

func (f ModelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}

func (s *serviceSuite) TestSetModelConfig(c *gc.C) {
	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Controller: "bar",
			},
		}, nil
	}

	cfg, err := config.New(config.NoDefaults, map[string]any{
		"name": "wallyworld",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	st := statetesting.NewState()
	svc := NewService(defaults, st)

	err = svc.SetModelConfig(context.Background(), cfg)
	c.Assert(err, jc.ErrorIsNil)

	cfg, err = svc.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cfg.AllAttrs(), jc.DeepEquals, map[string]any{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "bar",
		"secret-backend": "auto",
		"logging-config": "<root>=INFO",
	})
}
