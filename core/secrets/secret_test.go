// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/rs/xid"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
)

type SecretURISuite struct{}

var _ = gc.Suite(&SecretURISuite{})

const (
	controllerUUID = "555be5b3-987b-4848-80d0-966289f735f1"
	secretID       = "9m4e2mr0ui3e8a215n4g"
	secretURI      = "secret:9m4e2mr0ui3e8a215n4g"
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
		}, {
			in:       "secret:" + controllerUUID + "/" + secretID,
			shortStr: secretURI,
			expected: &secrets.URI{
				ControllerUUID: controllerUUID,
				ID:             secretID,
			},
		},
	} {
		result, err := secrets.ParseURI(t.in)
		if t.err != "" || result == nil {
			c.Check(err, gc.ErrorMatches, t.err)
		} else {
			c.Check(result, jc.DeepEquals, t.expected)
			c.Check(result.ShortString(), gc.Equals, t.shortStr)
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
		ControllerUUID: controllerUUID,
		ID:             secretID,
	}
	str := expected.String()
	c.Assert(str, gc.Equals, "secret:"+controllerUUID+"/"+secretID)
	uri, err := secrets.ParseURI(str)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uri, jc.DeepEquals, expected)
}

func (s *SecretURISuite) TestShortString(c *gc.C) {
	expected := &secrets.URI{
		ControllerUUID: controllerUUID,
		ID:             secretID,
	}
	str := expected.ShortString()
	c.Assert(str, gc.Equals, secretURI)
	uri, err := secrets.ParseURI(str)
	c.Assert(err, jc.ErrorIsNil)
	expected.ControllerUUID = ""
	c.Assert(uri, jc.DeepEquals, expected)
}

func (s *SecretURISuite) TestNew(c *gc.C) {
	URI := secrets.NewURI()
	c.Assert(URI.ControllerUUID, gc.Equals, "")
	_, err := xid.FromString(URI.ID)
	c.Assert(err, jc.ErrorIsNil)
}
