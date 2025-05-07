// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
)

type validatorSuite struct {
}

var _ = tc.Suite(&validatorSuite{})

func (*validatorSuite) TestControllerNotContainingValidator(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, map[string]any{
		config.NameKey:                 "wallyworld",
		config.UUIDKey:                 testing.ModelTag.Id(),
		config.TypeKey:                 "peachy",
		controller.AllowModelAccessKey: "bar",
		controller.ControllerName:      "bar",
	})
	c.Assert(err, tc.ErrorIsNil)

	rval, err := config.NoControllerAttributesValidator().Validate(context.Background(), cfg, nil)
	valErr, is := errors.AsType[*config.ValidationError](err)
	c.Assert(is, tc.IsTrue)
	c.Assert(valErr.InvalidAttrs, tc.DeepEquals, []string{controller.AllowModelAccessKey, controller.ControllerName})

	// Confirm no modification was done to the config.
	c.Assert(rval.AllAttrs(), tc.DeepEquals, cfg.AllAttrs())
}

func (*validatorSuite) TestModelValidator(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, map[string]any{
		config.NameKey:         "wallyworld",
		config.UUIDKey:         testing.ModelTag.Id(),
		config.TypeKey:         "peachy",
		config.AgentVersionKey: "3.11.1",
	})
	c.Assert(err, tc.ErrorIsNil)

	rval, err := config.ModelValidator().Validate(context.Background(), cfg, nil)
	valErr, is := errors.AsType[*config.ValidationError](err)
	c.Assert(is, tc.IsTrue)
	c.Assert(valErr.InvalidAttrs, tc.DeepEquals, []string{config.AgentVersionKey})

	// Confirm no modification was done to the config.
	c.Assert(rval.AllAttrs(), tc.DeepEquals, cfg.AllAttrs())
}

// Asserting the fact that model config validation when controller only config
// attributes are used.
func (*validatorSuite) TestModelValidatorFailsForControllerAttrs(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, map[string]any{
		config.NameKey:                 "wallyworld",
		config.UUIDKey:                 testing.ModelTag.Id(),
		config.TypeKey:                 "peachy",
		controller.AllowModelAccessKey: "bar",
		controller.ControllerName:      "bar",
	})
	c.Assert(err, tc.ErrorIsNil)

	rval, err := config.ModelValidator().Validate(context.Background(), cfg, nil)
	valErr, is := errors.AsType[*config.ValidationError](err)
	c.Assert(is, tc.IsTrue)
	c.Assert(valErr.InvalidAttrs, tc.DeepEquals, []string{controller.AllowModelAccessKey, controller.ControllerName})

	// Confirm no modification was done to the config.
	c.Assert(rval.AllAttrs(), tc.DeepEquals, cfg.AllAttrs())
}
