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

type SecretURLSuite struct{}

var _ = gc.Suite(&SecretURLSuite{})

const (
	controllerUUID = "555be5b3-987b-4848-80d0-966289f735f1"
	modelUUID      = "3fe4d1cd-17d3-418d-82a9-547f1949b835"
)

func (s *SecretURLSuite) TestParseURL(c *gc.C) {
	for _, t := range []struct {
		str      string
		shortStr string
		expected *secrets.URL
		err      string
	}{
		{
			str: "http://nope",
			err: `secret URL scheme "http" not valid`,
		}, {
			str: "secret://a/b/c",
			err: `secret URL "secret://a/b/c" not valid`,
		}, {
			str: "secret://missingversion",
			err: `secret URL "secret://missingversion" not valid`,
		}, {
			str: "secret://a.b.",
			err: `secret URL "secret://a.b." not valid`,
		}, {
			str: "secret://a.b#",
			err: `secret URL "secret://a.b#" not valid`,
		}, {
			str:      "secret://v1/app.password",
			shortStr: "secret://v1/app.password",
			expected: &secrets.URL{
				Version: "v1",
				Path:    "app.password",
			},
		}, {
			str:      "secret://v1/app.password#attr",
			shortStr: "secret://v1/app.password#attr",
			expected: &secrets.URL{
				Version:   "v1",
				Path:      "app.password",
				Attribute: "attr",
			},
		}, {
			str:      "secret://v1/" + controllerUUID + "/app.password",
			shortStr: "secret://v1/app.password",
			expected: &secrets.URL{
				Version:        "v1",
				ControllerUUID: controllerUUID,
				Path:           "app.password",
			},
		}, {
			str:      "secret://v1/" + controllerUUID + "/" + modelUUID + "/app.password",
			shortStr: "secret://v1/app.password",
			expected: &secrets.URL{
				Version:        "v1",
				ControllerUUID: controllerUUID,
				ModelUUID:      modelUUID,
				Path:           "app.password",
			},
		}, {
			str:      "secret://v1/" + controllerUUID + "/" + modelUUID + "/app.password#attr",
			shortStr: "secret://v1/app.password#attr",
			expected: &secrets.URL{
				Version:        "v1",
				ControllerUUID: controllerUUID,
				ModelUUID:      modelUUID,
				Path:           "app.password",
				Attribute:      "attr",
			},
		},
	} {
		result, err := secrets.ParseURL(t.str)
		if t.err != "" || result == nil {
			c.Check(err, gc.ErrorMatches, t.err)
		} else {
			c.Check(result, jc.DeepEquals, t.expected)
			c.Check(result.ShortString(), gc.Equals, t.shortStr)
			c.Check(result.String(), gc.Equals, t.str)
		}
	}
}

func (s *SecretURLSuite) TestString(c *gc.C) {
	expected := &secrets.URL{
		Version:        "v1",
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID,
		Path:           "app.password",
		Attribute:      "attr",
	}
	str := expected.String()
	c.Assert(str, gc.Equals, "secret://v1/"+controllerUUID+"/"+modelUUID+"/app.password#attr")
	url, err := secrets.ParseURL(str)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, jc.DeepEquals, expected)
}

func (s *SecretURLSuite) TestShortString(c *gc.C) {
	expected := &secrets.URL{
		Version:        "v1",
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID,
		Path:           "app.password",
		Attribute:      "attr",
	}
	str := expected.ShortString()
	c.Assert(str, gc.Equals, "secret://v1/app.password#attr")
	url, err := secrets.ParseURL(str)
	c.Assert(err, jc.ErrorIsNil)
	expected.ControllerUUID = ""
	expected.ModelUUID = ""
	c.Assert(url, jc.DeepEquals, expected)
}
