// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/rs/xid"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
)

type SecretURISuite struct{}

var _ = gc.Suite(&SecretURISuite{})

const (
	secretID  = "9m4e2mr0ui3e8a215n4g"
	secretURI = "secret:9m4e2mr0ui3e8a215n4g"
)

func (s *SecretURISuite) TestParseURI(c *gc.C) {
	for _, t := range []struct {
		in       string
		str      string
		shortStr string
		expected *secrets.URI
		err      string
	}{
		{
			in:  "http:nope",
			err: `secret URI scheme "http" not valid`,
		}, {
			in:  "secret:a/b/c",
			err: `secret URI "secret:a/b/c" not valid`,
		}, {
			in:  "secret:a.b.",
			err: `secret URI "secret:a.b." not valid`,
		}, {
			in:  "secret:a.b#",
			err: `secret URI "secret:a.b#" not valid`,
		}, {
			in:       secretURI,
			shortStr: secretURI,
			expected: &secrets.URI{
				ID: secretID,
			},
		}, {
			in:       secretID,
			str:      secretURI,
			shortStr: secretURI,
			expected: &secrets.URI{
				ID: secretID,
			},
		},
	} {
		result, err := secrets.ParseURI(t.in)
		if t.err != "" || result == nil {
			c.Check(err, gc.ErrorMatches, t.err)
		} else {
			c.Check(result, jc.DeepEquals, t.expected)
			c.Check(result.String(), gc.Equals, t.shortStr)
			if t.str != "" {
				c.Check(result.String(), gc.Equals, t.str)
			} else {
				c.Check(result.String(), gc.Equals, t.in)
			}
		}
	}
}

func (s *SecretURISuite) TestString(c *gc.C) {
	expected := &secrets.URI{
		ID: secretID,
	}
	str := expected.String()
	c.Assert(str, gc.Equals, secretURI)
	uri, err := secrets.ParseURI(str)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uri, jc.DeepEquals, expected)
}

func (s *SecretURISuite) TestName(c *gc.C) {
	uri := &secrets.URI{ID: secretID}
	name := uri.Name(666)
	c.Assert(name, gc.Equals, `9m4e2mr0ui3e8a215n4g-666`)
}

func (s *SecretURISuite) TestNew(c *gc.C) {
	URI := secrets.NewURI()
	_, err := xid.FromString(URI.ID)
	c.Assert(err, jc.ErrorIsNil)
}

type SecretSuite struct{}

var _ = gc.Suite(&SecretSuite{})

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretSuite) TestValidateConfig(c *gc.C) {
	cfg := secrets.SecretConfig{
		RotatePolicy: ptr(secrets.RotateDaily),
	}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "cannot specify a secret rotate policy without a next rotate time")

	cfg = secrets.SecretConfig{
		NextRotateTime: ptr(time.Now()),
	}
	err = cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "cannot specify a secret rotate time without a rotate policy")
}
