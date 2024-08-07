// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type authorizedKeysSuite struct {
}

var _ = gc.Suite(&authorizedKeysSuite{})

// TestSplitAuthorizedKeysFile is testing authorized keys splitting based on the
// the raw contents from a file.
func (*authorizedKeysSuite) TestSplitAuthorizedKeysFile(c *gc.C) {
	fileStr := `
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is
# This is a comment line for some reason
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is
		# This is another comment line indented with two tabs
`
	file := strings.NewReader(fileStr)
	keys, err := SplitAuthorizedKeysReaderByDelimiter('\n', file)
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, jc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})

	file = strings.NewReader(fileStr)
	keys, err = SplitAuthorizedKeysReader(file)
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, jc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})
}

// TestSplitAuthorizedKeysConfig is testing authorized keys splitting based on
// the raw contents that we are likely to encounter with a config string where
// instead of newlines we use the ';' delimiter.
func (*authorizedKeysSuite) TestSplitAuthorizedKeysConfig(c *gc.C) {
	configStr := `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is;# This is a comment line for some reason;ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is;# This is another comment line indented with two tabs`
	configReader := strings.NewReader(configStr)
	keys, err := SplitAuthorizedKeysReaderByDelimiter(';', configReader)
	c.Check(err, jc.ErrorIsNil)
	c.Check(keys, jc.DeepEquals, []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	})
}

// TestMakeAuthorizedKeysString is asserting that for a given set of keys they
// are written out in a standard compliant way to be an authorized_keys file.
func (*authorizedKeysSuite) TestMakeAuthorizedKeysString(c *gc.C) {
	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	}

	authorized := MakeAuthorizedKeysString(keys)
	c.Check(authorized, gc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is\n")
}

// TestWriteAuthorizedKeys is asserting that for a given set of keys they are
// written out in a standard compliant way to the writer.
func (*authorizedKeysSuite) TestWriteAuthorizedKeys(c *gc.C) {
	builder := strings.Builder{}
	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is",
	}
	WriteAuthorizedKeys(&builder, keys)
	c.Check(builder.String(), gc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC jimbo@juju.is\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe barry@juju.is\n")
}
