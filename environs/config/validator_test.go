// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type validatorSuite struct {
}

var _ = gc.Suite(&validatorSuite{})

func (_ *validatorSuite) TestControllerNotContainingValidator(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, map[string]any{
		config.NameKey:                 "wallyworld",
		config.UUIDKey:                 testing.ModelTag.Id(),
		config.TypeKey:                 "peachy",
		controller.AllowModelAccessKey: "bar",
		controller.ControllerName:      "bar",
	})
	c.Assert(err, jc.ErrorIsNil)

	rval, err := config.NoControllerAttributesValidator().Validate(cfg, nil)
	valErr, is := errors.AsType[*config.ValidationError](err)
	c.Assert(is, jc.IsTrue)
	c.Assert(valErr.InvalidAttrs, jc.DeepEquals, []string{controller.AllowModelAccessKey, controller.ControllerName})

	// Confirm no modification was done to the config.
	c.Assert(rval.AllAttrs(), jc.DeepEquals, cfg.AllAttrs())
}

func (_ *validatorSuite) TestModelValidator(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, map[string]any{
		config.NameKey:         "wallyworld",
		config.UUIDKey:         testing.ModelTag.Id(),
		config.TypeKey:         "peachy",
		config.AgentVersionKey: "3.11.1",
	})
	c.Assert(err, jc.ErrorIsNil)

	rval, err := config.ModelValidator().Validate(cfg, nil)
	valErr, is := errors.AsType[*config.ValidationError](err)
	c.Assert(is, jc.IsTrue)
	c.Assert(valErr.InvalidAttrs, jc.DeepEquals, []string{config.AgentVersionKey})

	// Confirm no modification was done to the config.
	c.Assert(rval.AllAttrs(), jc.DeepEquals, cfg.AllAttrs())
}

// Asserting the fact that model config validation when controller only config
// attributes are used.
func (_ *validatorSuite) TestModelValidatorFailsForControllerAttrs(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, map[string]any{
		config.NameKey:                 "wallyworld",
		config.UUIDKey:                 testing.ModelTag.Id(),
		config.TypeKey:                 "peachy",
		controller.AllowModelAccessKey: "bar",
		controller.ControllerName:      "bar",
	})
	c.Assert(err, jc.ErrorIsNil)

	rval, err := config.ModelValidator().Validate(cfg, nil)
	valErr, is := errors.AsType[*config.ValidationError](err)
	c.Assert(is, jc.IsTrue)
	c.Assert(valErr.InvalidAttrs, jc.DeepEquals, []string{controller.AllowModelAccessKey, controller.ControllerName})

	// Confirm no modification was done to the config.
	c.Assert(rval.AllAttrs(), jc.DeepEquals, cfg.AllAttrs())
}
