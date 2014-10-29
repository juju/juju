// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater_test

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/keyupdater"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type keyupdaterSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine

	keyupdater *keyupdater.State
}

var _ = gc.Suite(&keyupdaterSuite{})

func (s *keyupdaterSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var stateAPI *api.State
	stateAPI, s.rawMachine = s.OpenAPIAsNewMachine(c)
	c.Assert(stateAPI, gc.NotNil)
	s.keyupdater = stateAPI.KeyUpdater()
	c.Assert(s.keyupdater, gc.NotNil)
}

func (s *keyupdaterSuite) TestAuthorisedKeysNoSuchMachine(c *gc.C) {
	_, err := s.keyupdater.AuthorisedKeys(names.NewMachineTag("42"))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *keyupdaterSuite) TestAuthorisedKeysForbiddenMachine(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	_, err = s.keyupdater.AuthorisedKeys(m.Tag().(names.MachineTag))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *keyupdaterSuite) TestAuthorisedKeys(c *gc.C) {
	s.setAuthorisedKeys(c, "key1\nkey2")
	keys, err := s.keyupdater.AuthorisedKeys(s.rawMachine.Tag().(names.MachineTag))
	c.Assert(err, gc.IsNil)
	c.Assert(keys, gc.DeepEquals, []string{"key1", "key2"})
}

func (s *keyupdaterSuite) setAuthorisedKeys(c *gc.C, keys string) {
	err := s.BackingState.UpdateEnvironConfig(map[string]interface{}{"authorized-keys": keys}, nil, nil)
	c.Assert(err, gc.IsNil)
}

func (s *keyupdaterSuite) TestWatchAuthorisedKeys(c *gc.C) {
	watcher, err := s.keyupdater.WatchAuthorisedKeys(s.rawMachine.Tag().(names.MachineTag))
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
