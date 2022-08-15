// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
)

type CreateSecretSuite struct {
	base64Foo []byte
	base64Bar []byte
}

var _ = gc.Suite(&CreateSecretSuite{})

func (s *CreateSecretSuite) TestBadKey(c *gc.C) {
	_, err := secrets.CreateSecretData([]string{"fo=bar"})
	c.Assert(err, gc.ErrorMatches, `key "fo" not valid`)
}

func (s *CreateSecretSuite) TestKeyValues(c *gc.C) {
	data, err := secrets.CreateSecretData([]string{"foo=bar", "hello=world", "goodbye#base64=world"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, secrets.SecretData{
		"foo":     "YmFy",
		"hello":   "d29ybGQ=",
		"goodbye": "world",
	})
}

func (s *CreateSecretSuite) TestYAMLFile(c *gc.C) {
	data := `
    hello: world
    goodbye#base64: world
    another-key: !!binary |
      R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5
      OTk6enp56enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/+
      +f/++f/++f/++f/++f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLC
      AgjoEwnuNAFOhpEMTRiggcz4BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=`

	dir := c.MkDir()
	fileName := filepath.Join(dir, "secret.yaml")
	err := ioutil.WriteFile(fileName, []byte(data), os.FileMode(0644))
	c.Assert(err, jc.ErrorIsNil)

	attrs, err := secrets.ReadSecretData(fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, jc.DeepEquals, secrets.SecretData{
		"hello":       "d29ybGQ=",
		"goodbye":     "world",
		"another-key": `R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5OTk6enp56enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLCAgjoEwnuNAFOhpEMTRiggcz4BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=`,
	})
}

func (s *CreateSecretSuite) TestJSONFile(c *gc.C) {
	data := `{
    "hello": "world",
    "goodbye#base64": "world",
}`

	dir := c.MkDir()
	fileName := filepath.Join(dir, "secret.json")
	err := ioutil.WriteFile(fileName, []byte(data), os.FileMode(0644))
	c.Assert(err, jc.ErrorIsNil)

	attrs, err := secrets.ReadSecretData(fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, jc.DeepEquals, secrets.SecretData{
		"hello":   "d29ybGQ=",
		"goodbye": "world",
	})
}
