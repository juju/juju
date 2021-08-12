// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
)

type SecretConfigSuite struct{}

var _ = gc.Suite(&SecretConfigSuite{})

func (s *SecretConfigSuite) TestNewSecretConfig(c *gc.C) {
	cfg := secrets.NewSecretConfig("app", "catalog")
	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, &secrets.SecretConfig{
		Type:   secrets.TypeBlob,
		Path:   "app.catalog",
		Scope:  secrets.ScopeApplication,
		Params: nil,
	})
}

func (s *SecretConfigSuite) TestNewPasswordSecretConfig(c *gc.C) {
	cfg := secrets.NewPasswordSecretConfig(10, true, "app", "password")
	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, &secrets.SecretConfig{
		Type:  secrets.TypePassword,
		Path:  "app.password",
		Scope: secrets.ScopeApplication,
		Params: map[string]interface{}{
			"password-length":        10,
			"password-special-chars": true,
		},
	})
}

func (s *SecretConfigSuite) TestSecretConfigInvalidScope(c *gc.C) {
	cfg := secrets.NewPasswordSecretConfig(10, true, "app", "password")
	cfg.Scope = "foo"
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, `secret scope "foo" not valid`)
}

func (s *SecretConfigSuite) TestSecretConfigInvalidType(c *gc.C) {
	cfg := secrets.NewPasswordSecretConfig(10, true, "app", "password")
	cfg.Type = "foo"
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, `secret type "foo" not valid`)
}

func (s *SecretConfigSuite) TestSecretConfigPath(c *gc.C) {
	cfg := secrets.NewPasswordSecretConfig(10, true, "app", "password")
	cfg.Path = "foo=bar"
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, `secret path "foo=bar" not valid`)
}
