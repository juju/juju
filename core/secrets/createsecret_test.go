// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/secrets"
)

type CreateSecretSuite struct{}

func TestCreateSecretSuite(t *testing.T) {
	tc.Run(t, &CreateSecretSuite{})
}

func (s *CreateSecretSuite) TestBadKey(c *tc.C) {
	_, err := secrets.CreateSecretData([]string{"fo=bar"})
	c.Assert(err, tc.ErrorMatches, `key "fo" not valid`)
}

func (s *CreateSecretSuite) TestKeyValues(c *tc.C) {
	data, err := secrets.CreateSecretData([]string{"foo=bar", "hello=world", "goodbye#base64=world"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, secrets.SecretData{
		"foo":     "YmFy",
		"hello":   "d29ybGQ=",
		"goodbye": "world",
	})
}

func (s *CreateSecretSuite) TestKeyContentTooLarge(c *tc.C) {
	content := strings.Repeat("a", 9*1024)
	_, err := secrets.CreateSecretData([]string{"foo=" + content})
	c.Assert(err, tc.ErrorMatches, `secret content for key "foo" too large: 9216 bytes`)
}

func (s *CreateSecretSuite) TestTotalContentTooLarge(c *tc.C) {
	content := strings.Repeat("a", 4*1024)
	var args []string
	for i := 1; i <= 20; i++ {
		args = append(args, fmt.Sprintf("key%d=%s", i, content))
	}
	_, err := secrets.CreateSecretData(args)
	c.Assert(err, tc.ErrorMatches, `secret content too large: 81920 bytes`)
}

func (s *CreateSecretSuite) TestSecretKeyFromFile(c *tc.C) {
	content := `
      -----BEGIN CERTIFICATE-----
      MIIFYjCCA0qgAwIBAgIQKaPND9YggIG6+jOcgmpk3DANBgkqhkiG9w0BAQsFADAz
      MRwwGgYDVQQKExNsaW51eGNvbnRhaW5lcnMub3JnMRMwEQYDVQQDDAp0aW1AZWx3
      -----END CERTIFICATE-----`[1:]

	dir := c.MkDir()
	fileName := filepath.Join(dir, "secret-data.bin")
	err := os.WriteFile(fileName, []byte(content), os.FileMode(0644))
	c.Assert(err, tc.ErrorIsNil)

	data, err := secrets.CreateSecretData([]string{"key1=value1", "key2#file=" + fileName})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(data, tc.DeepEquals, secrets.SecretData{
		"key1": "dmFsdWUx",
		"key2": `ICAgICAgLS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCiAgICAgIE1JSUZZakNDQTBxZ0F3SUJBZ0lRS2FQTkQ5WWdnSUc2K2pPY2dtcGszREFOQmdrcWhraUc5dzBCQVFzRkFEQXoKICAgICAgTVJ3d0dnWURWUVFLRXhOc2FXNTFlR052Ym5SaGFXNWxjbk11YjNKbk1STXdFUVlEVlFRRERBcDBhVzFBWld4MwogICAgICAtLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0t`,
	})
}

func (s *CreateSecretSuite) TestYAMLFile(c *tc.C) {
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
	err := os.WriteFile(fileName, []byte(data), os.FileMode(0644))
	c.Assert(err, tc.ErrorIsNil)

	attrs, err := secrets.ReadSecretData(fileName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attrs, tc.DeepEquals, secrets.SecretData{
		"hello":       "d29ybGQ=",
		"goodbye":     "world",
		"another-key": `R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5OTk6enp56enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLCAgjoEwnuNAFOhpEMTRiggcz4BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=`,
	})
}

func (s *CreateSecretSuite) TestJSONFile(c *tc.C) {
	data := `{
    "hello": "world",
    "goodbye#base64": "world",
}`

	dir := c.MkDir()
	fileName := filepath.Join(dir, "secret.json")
	err := os.WriteFile(fileName, []byte(data), os.FileMode(0644))
	c.Assert(err, tc.ErrorIsNil)

	attrs, err := secrets.ReadSecretData(fileName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attrs, tc.DeepEquals, secrets.SecretData{
		"hello":   "d29ybGQ=",
		"goodbye": "world",
	})
}
