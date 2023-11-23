// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/modeldefaults"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/config"
)

type bootstrapSuite struct {
	schematesting.ModelSuite
}

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

var _ = gc.Suite(&bootstrapSuite{})

func (f ModelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}

func (s *bootstrapSuite) TestSetModelConfig(c *gc.C) {
	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				V:      "bar",
			},
		}, nil
	}

	cfg, err := config.New(config.NoDefaults, map[string]any{
		"name": "wallyworld",
		"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = SetModelConfig(cfg, defaults)(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	rows, err := s.DB().Query("SELECT * FROM model_config")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	configVals := map[string]string{}
	var k, v string
	for rows.Next() {
		err = rows.Scan(&k, &v)
		c.Assert(err, jc.ErrorIsNil)
		configVals[k] = v
	}

	c.Assert(rows.Err(), jc.ErrorIsNil)
	c.Assert(configVals, jc.DeepEquals, map[string]string{
		"name":           "wallyworld",
		"uuid":           "a677bdfd-3c96-46b2-912f-38e25faceaf7",
		"type":           "sometype",
		"foo":            "bar",
		"logging-config": "<root>=INFO",
		"secret-backend": "auto",
	})
}
