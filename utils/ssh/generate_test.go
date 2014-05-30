// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"crypto/rsa"
	"io"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils/ssh"
)

type GenerateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&GenerateSuite{})

var pregeneratedKey *rsa.PrivateKey

// overrideGenerateKey patches out rsa.GenerateKey to create a single testing
// key which is saved and used between tests to save computation time.
func overrideGenerateKey(c *gc.C) testing.Restorer {
	restorer := testing.PatchValue(ssh.RSAGenerateKey, func(random io.Reader, bits int) (*rsa.PrivateKey, error) {
		if pregeneratedKey != nil {
			return pregeneratedKey, nil
		}
		// Ignore requested bits and just use 512 bits for speed
		key, err := rsa.GenerateKey(random, 512)
		if err != nil {
			return nil, err
		}
		key.Precompute()
		pregeneratedKey = key
		return key, nil
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
