// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"encoding/base64"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/secrets"
)

type SecretValueSuite struct{}

func TestSecretValueSuite(t *testing.T) {
	tc.Run(t, &SecretValueSuite{})
}

func (s *SecretValueSuite) TestEncodedValues(c *tc.C) {
	in := map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	}
	val := secrets.NewSecretValue(in)

	c.Assert(val.EncodedValues(), tc.DeepEquals, map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	})
}

func (s *SecretValueSuite) TestValues(c *tc.C) {
	in := map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	}
	val := secrets.NewSecretValue(in)

	strValues, err := val.Values()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(strValues, tc.DeepEquals, map[string]string{
		"a": "foo",
		"b": "1",
	})
}

func (s *SecretValueSuite) TestChecksum(c *tc.C) {
	in := map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	}
	val := secrets.NewSecretValue(in)
	cs, err := val.Checksum()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cs, tc.Equals, "c3d5969f3a16dd48b80a58f21dfe105eaf7fe822fbe564f555f31e3e1a9ba9ac")
}

func (s *SecretValueSuite) TestEmpty(c *tc.C) {
	in := map[string]string{}
	val := secrets.NewSecretValue(in)
	c.Assert(val.IsEmpty(), tc.IsTrue)
}

func (s *SecretValueSuite) TestKeyValue(c *tc.C) {
	in := map[string]string{
		"a": base64.StdEncoding.EncodeToString([]byte("foo")),
		"b": base64.StdEncoding.EncodeToString([]byte("1")),
	}
	val := secrets.NewSecretValue(in)

	v, err := val.KeyValue("a")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(v, tc.Equals, "foo")
	v, err = val.KeyValue("a#base64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(v, tc.Equals, base64.StdEncoding.EncodeToString([]byte("foo")))
}
