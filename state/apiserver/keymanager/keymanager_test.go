// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"strings"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/keymanager"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	statetesting "launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/utils/ssh"
	sshtesting "launchpad.net/juju-core/utils/ssh/testing"
)

type keyManagerSuite struct {
	jujutesting.JujuConnSuite

	keymanager *keymanager.KeyManagerAPI
	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&keyManagerSuite{})

func (s *keyManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag:      "user-admin",
		LoggedIn: true,
		Client:   true,
	}
	var err error
	s.keymanager, err = keymanager.NewKeyManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
}

func (s *keyManagerSuite) TestNewKeyManagerAPIAcceptsClient(c *gc.C) {
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *keyManagerSuite) TestNewKeyManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Client = false
	endPoint, err := keymanager.NewKeyManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *keyManagerSuite) setAuthorisedKeys(c *gc.C, keys string) {
	err := statetesting.UpdateConfig(s.State, map[string]interface{}{"authorized-keys": keys})
	c.Assert(err, gc.IsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(envConfig.AuthorizedKeys(), gc.Equals, keys)
}

func (s *keyManagerSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorisedKeys(c, strings.Join([]string{key1, key2, "bad key"}, "\n"))

	args := params.ListSSHKeys{
		Entities: params.Entities{[]params.Entity{
			{Tag: "admin"},
			{Tag: "invalid"},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.keymanager.ListKeys(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{key1, key2, "Invalid key: bad key"}},
			{Error: apiservertesting.NotFoundError(`user "invalid"`)},
		},
	})
}
