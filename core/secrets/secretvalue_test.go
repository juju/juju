// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/secrets"
)

type SecretValueSuite struct{}

var _ = gc.Suite(&SecretValueSuite{})

func (s *SecretValueSuite) TestEncodedValues(c *gc.C) {
	in := map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	}
	val := secrets.NewSecretValue(in)

	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	})

	c.Assert(val.Singular(), jc.IsFalse)
	_, err := val.EncodedValue()
	c.Assert(err, gc.ErrorMatches, "secret is not a singular value")
}

func (s *SecretValueSuite) TestValues(c *gc.C) {
	in := map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	}
	val := secrets.NewSecretValue(in)

	strValues, err := val.Values()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strValues, jc.DeepEquals, map[string]string{
		"a": "foo",
		"b": "1",
	})

	_, err = val.Value()
	c.Assert(err, gc.ErrorMatches, "secret is not a singular value")
}

func (s *SecretValueSuite) TestSingularValue(c *gc.C) {
	in := map[string]string{
		"data": base64.StdEncoding.EncodeToString([]byte("foo")),
	}
	val := secrets.NewSecretValue(in)

	sval, err := val.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sval, gc.Equals, "foo")
}

func (s *SecretValueSuite) TestSingularEncodedValue(c *gc.C) {
	in := map[string]string{
		"data": base64.StdEncoding.EncodeToString([]byte("foo")),
	}
	val := secrets.NewSecretValue(in)
	c.Assert(val.Singular(), jc.IsTrue)

	bval, err := val.EncodedValue()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bval, gc.Equals, in["data"])
}
