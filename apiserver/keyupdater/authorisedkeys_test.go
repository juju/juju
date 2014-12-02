// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/keyupdater"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type authorisedKeysSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine       *state.Machine
	unrelatedMachine *state.Machine
	keyupdater       *keyupdater.KeyUpdaterAPI
	resources        *common.Resources
	authoriser       apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&authorisedKeysSuite{})

func (s *authorisedKeysSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// Create machines to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.unrelatedMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// The default auth is as a state server
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}
	s.keyupdater, err = keyupdater.NewKeyUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *authorisedKeysSuite) TestNewKeyUpdaterAPIAcceptsStateServer(c *gc.C) {
	endPoint, err := keyupdater.NewKeyUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *authorisedKeysSuite) TestNewKeyUpdaterAPIRefusesNonMachineAgent(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("ubuntu/1")
	endPoint, err := keyupdater.NewKeyUpdaterAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *authorisedKeysSuite) TestWatchAuthorisedKeysNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.keyupdater.WatchAuthorisedKeys(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *authorisedKeysSuite) setAuthorizedKeys(c *gc.C, keys string) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"authorized-keys": keys}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envConfig.AuthorizedKeys(), gc.Equals, keys)
}

func (s *authorisedKeysSuite) TestWatchAuthorisedKeys(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.rawMachine.Tag().String()},
			{Tag: s.unrelatedMachine.Tag().String()},
			{Tag: "machine-42"},
		},
	}
	results, err := s.keyupdater.WatchAuthorisedKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
	c.Assert(results.Results[0].NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Assert(results.Results[0].Error, gc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Assert(resource, gc.NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertNoChange()

	s.setAuthorizedKeys(c, "key1\nkey2")

	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *authorisedKeysSuite) TestAuthorisedKeysForNoone(c *gc.C) {
	// Not an error to request nothing, dumb, but not an error.
	results, err := s.keyupdater.AuthorisedKeys(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *authorisedKeysSuite) TestAuthorisedKeys(c *gc.C) {
	s.setAuthorizedKeys(c, "key1\nkey2")

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.rawMachine.Tag().String()},
			{Tag: s.unrelatedMachine.Tag().String()},
			{Tag: "machine-42"},
		},
	}
	results, err := s.keyupdater.AuthorisedKeys(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{"key1", "key2"}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}
