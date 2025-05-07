// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"fmt"
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/rs/xid"

	"github.com/juju/juju/core/secrets"
)

type SecretURISuite struct{}

var _ = tc.Suite(&SecretURISuite{})

const (
	secretID        = "9m4e2mr0ui3e8a215n4g"
	secretSource    = "deadbeef-1bad-500d-9000-4b1d0d06f00d"
	secretURI       = "secret:9m4e2mr0ui3e8a215n4g"
	remoteSecretURI = "secret://deadbeef-1bad-500d-9000-4b1d0d06f00d/9m4e2mr0ui3e8a215n4g"
	remoteSecretID  = "deadbeef-1bad-500d-9000-4b1d0d06f00d/9m4e2mr0ui3e8a215n4g"
)

func (s *SecretURISuite) TestParseURI(c *tc.C) {
	for _, t := range []struct {
		in       string
		str      string
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
			in: secretURI,
			expected: &secrets.URI{
				ID: secretID,
			},
		}, {
			in:  secretID,
			str: secretURI,
			expected: &secrets.URI{
				ID: secretID,
			},
		}, {
			in:  remoteSecretURI,
			str: remoteSecretURI,
			expected: &secrets.URI{
				ID:         secretID,
				SourceUUID: secretSource,
			},
		}, {
			in:  remoteSecretID,
			str: remoteSecretURI,
			expected: &secrets.URI{
				ID:         secretID,
				SourceUUID: secretSource,
			},
		},
	} {
		result, err := secrets.ParseURI(t.in)
		if t.err != "" || result == nil {
			c.Check(err, tc.ErrorMatches, t.err)
		} else {
			c.Check(result, jc.DeepEquals, t.expected)
			if t.str != "" {
				c.Check(result.String(), tc.Equals, t.str)
			} else {
				c.Check(result.String(), tc.Equals, t.in)
			}
		}
	}
}

func (s *SecretURISuite) TestString(c *tc.C) {
	expected := &secrets.URI{
		ID: secretID,
	}
	str := expected.String()
	c.Assert(str, tc.Equals, secretURI)
	uri, err := secrets.ParseURI(str)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uri, jc.DeepEquals, expected)
}

func (s *SecretURISuite) TestStringWithSource(c *tc.C) {
	expected := &secrets.URI{
		SourceUUID: secretSource,
		ID:         secretID,
	}
	str := expected.String()
	c.Assert(str, tc.Equals, fmt.Sprintf("secret://%s/%s", secretSource, secretID))
	uri, err := secrets.ParseURI(str)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uri, jc.DeepEquals, expected)
}

func (s *SecretURISuite) TestName(c *tc.C) {
	uri := &secrets.URI{ID: secretID}
	name := uri.Name(666)
	c.Assert(name, tc.Equals, `9m4e2mr0ui3e8a215n4g-666`)
}

func (s *SecretURISuite) TestNew(c *tc.C) {
	uri := secrets.NewURI()
	_, err := xid.FromString(uri.ID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretURISuite) TestWithSource(c *tc.C) {
	uri := &secrets.URI{ID: secretID}
	uri = uri.WithSource(secretSource)
	c.Assert(uri.SourceUUID, tc.Equals, secretSource)
	c.Assert(uri.ID, tc.Equals, secretID)
}

func (s *SecretURISuite) TestIsLocal(c *tc.C) {
	uri := secrets.NewURI()
	c.Assert(uri.IsLocal("other-uuid"), jc.IsTrue)
	uri2 := uri.WithSource("some-uuid")
	c.Assert(uri2.IsLocal("some-uuid"), jc.IsTrue)
	c.Assert(uri2.IsLocal("other-uuid"), jc.IsFalse)
}

type SecretSuite struct{}

var _ = tc.Suite(&SecretSuite{})

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretSuite) TestValidateConfig(c *tc.C) {
	cfg := secrets.SecretConfig{
		RotatePolicy: ptr(secrets.RotateDaily),
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "cannot specify a secret rotate policy without a next rotate time")

	cfg = secrets.SecretConfig{
		NextRotateTime: ptr(time.Now()),
	}
	err = cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "cannot specify a secret rotate time without a rotate policy")
}
