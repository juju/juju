// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/ssh"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

type FingerprintSuite struct {
	testbase.LoggingSuite
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
		c.Assert(err, gc.IsNil)
		c.Assert(fingerprint, gc.Equals, k.Fingerprint)
	}
}

func (s *FingerprintSuite) TestKeyFingerprintError(c *gc.C) {
	_, _, err := ssh.KeyFingerprint("invalid key")
	c.Assert(err, gc.ErrorMatches, `generating key fingerprint: invalid authorized_key "invalid key"`)
}
