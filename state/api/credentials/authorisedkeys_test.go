// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentials_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/credentials"
	"launchpad.net/juju-core/state/testing"
)

type credentialsSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine

	credentials *credentials.State
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var stateAPI *api.State
	stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c)
	c.Assert(stateAPI, gc.NotNil)
	s.credentials = stateAPI.Credentials()
	c.Assert(s.credentials, gc.NotNil)

}

func (s *credentialsSuite) TestAuthorisedKeysNoSuchMachine(c *gc.C) {
	_, err := s.credentials.AuthorisedKeys("machine-42")
	c.Assert(err, gc.ErrorMatches, "machine 42 not found")
}

func (s *credentialsSuite) TestAuthorisedKeysForbiddenMachine(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobManageEnviron, state.JobManageState)
	c.Assert(err, gc.IsNil)
	_, err = s.credentials.AuthorisedKeys(m.Tag())
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *credentialsSuite) TestAuthorisedKeys(c *gc.C) {
	s.setAuthorisedKeys(c, "key1\nkey2")
	keys, err := s.credentials.AuthorisedKeys(s.rawMachine.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(keys, gc.DeepEquals, []string{"key1", "key2"})
}

func (s *credentialsSuite) setAuthorisedKeys(c *gc.C, keys string) {
	err := testing.UpdateConfig(s.BackingState, map[string]interface{}{"authorized-keys": keys})
	c.Assert(err, gc.IsNil)
}

func (s *credentialsSuite) TestWatchAuthorisedKeys(c *gc.C) {
	watcher, err := s.credentials.WatchAuthorisedKeys(s.rawMachine.Tag())
	c.Assert(err, gc.IsNil)
	defer testing.AssertStop(c, watcher)
	wc := testing.NewNotifyWatcherC(c, s.BackingState, watcher)
	// Initial event
	wc.AssertOneChange()

	s.setAuthorisedKeys(c, "key1\nkey2")
	// One change noticing the new version
	wc.AssertOneChange()
	// Setting the version to the same value doesn't trigger a change
	s.setAuthorisedKeys(c, "key1\nkey2")
	wc.AssertNoChange()

	s.setAuthorisedKeys(c, "key1\nkey2\nkey3")
	wc.AssertOneChange()
	testing.AssertStop(c, watcher)
	wc.AssertClosed()
}
