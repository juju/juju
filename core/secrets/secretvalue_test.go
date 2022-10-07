// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
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
}

func (s *SecretValueSuite) TestEmpty(c *gc.C) {
	in := map[string]string{}
	val := secrets.NewSecretValue(in)
	c.Assert(val.IsEmpty(), jc.IsTrue)
}

func (s *SecretValueSuite) TestKeyValue(c *gc.C) {
	in := map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	}
	val := secrets.NewSecretValue(in)

	v, err := val.KeyValue("a")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.Equals, "foo")
	v, err = val.KeyValue("a#base64")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.Equals, base64.StdEncoding.EncodeToString([]byte("foo")))
}
