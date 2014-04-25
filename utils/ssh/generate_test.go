// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"crypto/rsa"
	"io"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/ssh"
)

type GenerateSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&GenerateSuite{})

var pregeneratedKey *rsa.PrivateKey

func overrideGenerateKey(c *gc.C) testing.Restorer {
	restorer := testing.PatchValue(ssh.RSAGenerateKey, func(random io.Reader, bits int) (*rsa.PrivateKey, error) {
		if pregeneratedKey != nil {
			return pregeneratedKey, nil
		}
		key, err := rsa.GenerateKey(random, 512)
		key.Precompute()
		pregeneratedKey = key
		return key, err
	})
	return restorer
}

func (s *GenerateSuite) TestGenerate(c *gc.C) {
	defer overrideGenerateKey(c).Restore()
	private, public, err := ssh.GenerateKey("some-comment")

	c.Check(err, gc.IsNil)
	c.Check(private, jc.HasPrefix, "-----BEGIN RSA PRIVATE KEY-----\n")
	c.Check(private, jc.HasSuffix, "-----END RSA PRIVATE KEY-----\n")
	c.Check(public, jc.HasPrefix, "ssh-rsa ")
	c.Check(public, jc.HasSuffix, " some-comment\n")
}
