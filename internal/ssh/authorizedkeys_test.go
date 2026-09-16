// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/tc"
)

type authorizedKeysSuite struct {
}

func TestAuthorizedKeysSuite(t *testing.T) {
	tc.Run(t, &authorizedKeysSuite{})
}

func (*authorizedKeysSuite) TestPublicKeysForPrivateKeyFiles(c *tc.C) {
	keyDirectory := c.MkDir()
	privateKeys := []string{
		filepath.Join(keyDirectory, "one"),
		filepath.Join(keyDirectory, "two"),
	}
	for i, privateKey := range privateKeys {
		key := fmt.Appendf(nil, "key-%d\n", i)
		c.Assert(os.WriteFile(privateKey+publicKeyFileSuffix, key, 0600), tc.ErrorIsNil)
	}

	keys, err := PublicKeysForPrivateKeyFiles(privateKeys)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []string{"key-0", "key-1"})
}

func (*authorizedKeysSuite) TestPublicKeysForPrivateKeyFilesMissingPublicKey(c *tc.C) {
	_, err := PublicKeysForPrivateKeyFiles([]string{filepath.Join(c.MkDir(), "missing")})
	c.Assert(err, tc.ErrorMatches, `reading public key for .*: open .*: no such file or directory`)
}

// TestSplitAuthorizedKeysFile is testing authorized keys splitting based on the
// the raw contents from a file.
func (*authorizedKeysSuite) TestSplitAuthorizedKeysFile(c *tc.C) {
	fileStr := `
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is
# This is a comment line for some reason
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is
		# This is another comment line indented with two tabs
`
	file := strings.NewReader(fileStr)
	keys, err := SplitAuthorizedKeysReaderByDelimiter('\n', file)
	c.Check(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})

	file = strings.NewReader(fileStr)
	keys, err = SplitAuthorizedKeysReader(file)
	c.Check(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})
}

// TestSplitAuthorizedKeysConfig is testing authorized keys splitting based on
// the raw contents that we are likely to encounter with a config string where
// instead of newlines we use the ';' delimiter.
func (*authorizedKeysSuite) TestSplitAuthorizedKeysConfig(c *tc.C) {
	configStr := `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is;# This is a comment line for some reason;ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is;# This is another comment line indented with two tabs`
	configReader := strings.NewReader(configStr)
	keys, err := SplitAuthorizedKeysReaderByDelimiter(';', configReader)
	c.Check(err, tc.ErrorIsNil)
	c.Check(keys, tc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})
}

// TestParsePublicKeySingleKey is asserting that the surrounding whitespace,
// blank lines and comment lines that can accompany a single key in an
// authorized_keys file are all accepted.
func (*authorizedKeysSuite) TestParsePublicKeySingleKey(c *tc.C) {
	key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is"

	for _, test := range []string{
		key,
		key + "\n",
		key + "\n\n",
		key + "\n# a comment line\n",
	} {
		parsed, err := ParsePublicKey(test)
		c.Check(err, tc.ErrorIsNil, tc.Commentf("input %q", test))
		c.Check(parsed.Comment, tc.Equals, "jimbo@juju.is")
	}
}

// TestParsePublicKeyMultipleKeys is asserting that data describing more than
// one key is rejected. ssh.ParseAuthorizedKey only reads the first key it
// understands, and callers persist the string they handed us, so accepting
// this would authorise the trailing keys without ever validating them or
// recording their fingerprint.
func (*authorizedKeysSuite) TestParsePublicKeyMultipleKeys(c *tc.C) {
	_, err := ParsePublicKey(
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is\n" +
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	)
	c.Check(err, tc.NotNil)
}

// TestMakeAuthorizedKeysString is asserting that for a given set of keys they
// are written out in a standard compliant way to be an authorized_keys file.
func (*authorizedKeysSuite) TestMakeAuthorizedKeysString(c *tc.C) {
	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	}

	authorized := MakeAuthorizedKeysString(keys)
	c.Check(authorized, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is\n")
}

// TestWriteAuthorizedKeys is asserting that for a given set of keys they are
// written out in a standard compliant way to the writer.
func (*authorizedKeysSuite) TestWriteAuthorizedKeys(c *tc.C) {
	builder := strings.Builder{}
	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	}
	WriteAuthorizedKeys(&builder, keys)
	c.Check(builder.String(), tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is\n")
}
