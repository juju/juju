// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"strings"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/keymanager"
	"launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/utils/ssh"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

type keymanagerSuite struct {
	jujutesting.JujuConnSuite

	keymanager *keymanager.State
}

var _ = gc.Suite(&keymanagerSuite{})

func (s *keymanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.keymanager = s.APIState.KeyManager()
	c.Assert(s.keymanager, gc.NotNil)

}

func (s *keymanagerSuite) setAuthorisedKeys(c *gc.C, keys string) {
	err := testing.UpdateConfig(s.BackingState, map[string]interface{}{"authorized-keys": keys})
	c.Assert(err, gc.IsNil)
}

func (s *keymanagerSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2}, "\n"))

	keyResults, err := s.keymanager.ListKeys(ssh.Fingerprints, "admin")
	c.Assert(err, gc.IsNil)
	c.Assert(len(keyResults), gc.Equals, 1)
	result := keyResults[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.DeepEquals,
		[]string{sshtesting.ValidKeyOne.Fingerprint + " (user@host)", sshtesting.ValidKeyTwo.Fingerprint})
}

func (s *keymanagerSuite) TestListKeysErrors(c *gc.C) {
	keyResults, err := s.keymanager.ListKeys(ssh.Fingerprints, "invalid")
	c.Assert(err, gc.IsNil)
	c.Assert(len(keyResults), gc.Equals, 1)
	result := keyResults[0]
	c.Assert(result.Error, gc.ErrorMatches, `user "invalid" not found`)
}
