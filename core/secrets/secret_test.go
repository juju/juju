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
		Path:   "app/catalog",
		Params: nil,
	})
}

func (s *SecretConfigSuite) TestNewPasswordSecretConfig(c *gc.C) {
	cfg := secrets.NewPasswordSecretConfig(10, true, "app", "mariadb", "password")
	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, &secrets.SecretConfig{
		Path: "app/mariadb/password",
		Params: map[string]interface{}{
			"password-length":        10,
			"password-special-chars": true,
		},
	})
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
			str:      "secret://app/mariadb/password",
			shortStr: "secret://app/mariadb/password",
			expected: &secrets.URL{
				Path: "app/mariadb/password",
			},
		}, {
			str:      "secret://app/mariadb/password2/666",
			shortStr: "secret://app/mariadb/password2/666",
			expected: &secrets.URL{
				Path:     "app/mariadb/password2",
				Revision: 666,
			},
		}, {
			str:      "secret://app/mariadb-k8s/password#attr",
			shortStr: "secret://app/mariadb-k8s/password#attr",
			expected: &secrets.URL{
				Path:      "app/mariadb-k8s/password",
				Attribute: "attr",
			},
		}, {
			str:      "secret://app/mariadb/password/666#attr",
			shortStr: "secret://app/mariadb/password/666#attr",
			expected: &secrets.URL{
				Path:      "app/mariadb/password",
				Attribute: "attr",
				Revision:  666,
			},
		}, {
			str:      "secret://" + controllerUUID + "/app/mariadb/password",
			shortStr: "secret://app/mariadb/password",
			expected: &secrets.URL{
				ControllerUUID: controllerUUID,
				Path:           "app/mariadb/password",
			},
		}, {
			str:      "secret://" + controllerUUID + "/" + modelUUID + "/app/mariadb/password",
			shortStr: "secret://app/mariadb/password",
			expected: &secrets.URL{
				ControllerUUID: controllerUUID,
				ModelUUID:      modelUUID,
				Path:           "app/mariadb/password",
			},
		}, {
			str:      "secret://" + controllerUUID + "/" + modelUUID + "/app/mariadb/password#attr",
			shortStr: "secret://app/mariadb/password#attr",
			expected: &secrets.URL{
				ControllerUUID: controllerUUID,
				ModelUUID:      modelUUID,
				Path:           "app/mariadb/password",
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
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID,
		Path:           "app/mariadb/password",
		Attribute:      "attr",
	}
	str := expected.String()
	c.Assert(str, gc.Equals, "secret://"+controllerUUID+"/"+modelUUID+"/app/mariadb/password#attr")
	url, err := secrets.ParseURL(str)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, jc.DeepEquals, expected)
}

func (s *SecretURLSuite) TestStringWithRevision(c *gc.C) {
	URL := &secrets.URL{
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID,
		Path:           "app/mariadb/password",
		Attribute:      "attr",
	}
	str := URL.String()
	c.Assert(str, gc.Equals, "secret://"+controllerUUID+"/"+modelUUID+"/app/mariadb/password#attr")
	URL.Revision = 1
	str = URL.String()
	c.Assert(str, gc.Equals, "secret://"+controllerUUID+"/"+modelUUID+"/app/mariadb/password/1#attr")
}

func (s *SecretURLSuite) TestShortString(c *gc.C) {
	expected := &secrets.URL{
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID,
		Path:           "app/mariadb/password",
		Attribute:      "attr",
	}
	str := expected.ShortString()
	c.Assert(str, gc.Equals, "secret://app/mariadb/password#attr")
	url, err := secrets.ParseURL(str)
	c.Assert(err, jc.ErrorIsNil)
	expected.ControllerUUID = ""
	expected.ModelUUID = ""
	c.Assert(url, jc.DeepEquals, expected)
}

func (s *SecretURLSuite) TestID(c *gc.C) {
	expected := &secrets.URL{
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID,
		Path:           "app/mariadb/password",
		Attribute:      "attr",
	}
	c.Assert(expected.ID(), gc.Equals, "secret://"+controllerUUID+"/"+modelUUID+"/app/mariadb/password")
}

func (s *SecretURLSuite) TestWithRevision(c *gc.C) {
	expected := &secrets.URL{
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID,
		Path:           "app/mariadb/password",
		Attribute:      "attr",
	}
	expected = expected.WithRevision(666)
	c.Assert(expected.String(), gc.Equals, "secret://"+controllerUUID+"/"+modelUUID+"/app/mariadb/password/666#attr")
}

func (s *SecretURLSuite) TestWithAttribute(c *gc.C) {
	expected := &secrets.URL{
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID,
		Path:           "app/mariadb/password",
	}
	expected = expected.WithAttribute("attr")
	c.Assert(expected.String(), gc.Equals, "secret://"+controllerUUID+"/"+modelUUID+"/app/mariadb/password#attr")
}

func (s *SecretURLSuite) TestNewSimpleURL(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	c.Assert(URL.String(), gc.Equals, "secret://app/mariadb/password")
}

func (s *SecretURLSuite) TestOwnerApplication(c *gc.C) {
	URL := secrets.NewSimpleURL("app/mariadb/password")
	app, ok := URL.OwnerApplication()
	c.Assert(ok, jc.IsTrue)
	c.Assert(app, gc.Equals, "mariadb")

	URL2 := secrets.NewSimpleURL("unit/mariadb-0/password")
	_, ok = URL2.OwnerApplication()
	c.Assert(ok, jc.IsFalse)

}
