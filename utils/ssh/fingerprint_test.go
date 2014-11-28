// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/utils/ssh"
	sshtesting "github.com/juju/juju/utils/ssh/testing"
)

type FingerprintSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FingerprintSuite{})

func (s *FingerprintSuite) TestKeyFingerprint(c *gc.C) {
	keys := []sshtesting.SSHKey{
		sshtesting.ValidKeyOne,
		sshtesting.ValidKeyTwo,
		sshtesting.ValidKeyThree,
	}
	for _, k := range keys {
		fingerprint, _, err := ssh.KeyFingerprint(k.Key)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(fingerprint, gc.Equals, k.Fingerprint)
	}
}

func (s *FingerprintSuite) TestKeyFingerprintError(c *gc.C) {
	_, _, err := ssh.KeyFingerprint("invalid key")
	c.Assert(err, gc.ErrorMatches, `generating key fingerprint: invalid authorized_key "invalid key"`)
}
