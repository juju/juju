// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/secrets"
)

type CreateSecretSuite struct {
	base64Foo []byte
	base64Bar []byte
}

var _ = gc.Suite(&CreateSecretSuite{})

func (s *CreateSecretSuite) TestInvalidSingularValue(c *gc.C) {
	_, err := secrets.CreatSecretData(false, []string{"token", "foo=bar"})
	c.Assert(err, gc.ErrorMatches, `key value "foo=bar" not valid when a singular value has already been specified`)

	_, err = secrets.CreatSecretData(false, []string{"foo=bar", "token"})
	c.Assert(err, gc.ErrorMatches, `singular value "token" not valid when other key values are specified`)
}

func (s *CreateSecretSuite) TestSingularValue(c *gc.C) {
	data, err := secrets.CreatSecretData(false, []string{"token"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, secrets.SecretData{
		"data": "dG9rZW4=",
	})
}

func (s *CreateSecretSuite) TestSingularValueBase64(c *gc.C) {
	data, err := secrets.CreatSecretData(true, []string{"key="})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, secrets.SecretData{
		"data": "key=",
	})
}

func (s *CreateSecretSuite) TestValues(c *gc.C) {
	data, err := secrets.CreatSecretData(false, []string{"foo=bar", "hello=world"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, secrets.SecretData{
		"foo":   "YmFy",
		"hello": "d29ybGQ=",
	})
}

func (s *CreateSecretSuite) TestValuesBase64(c *gc.C) {
	data, err := secrets.CreatSecretData(true, []string{"foo=bar", "hello=world"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, secrets.SecretData{
		"foo":   "bar",
		"hello": "world",
	})
}
