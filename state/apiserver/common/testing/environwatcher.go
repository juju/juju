// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state/api/params"
)

const (
	HasSecrets = true
	NoSecrets  = false
)

type Implementer interface {
	WatchForEnvironConfigChanges() (params.NotifyWatchResult, error)
	EnvironConfig() (params.EnvironConfigResult, error)
}

type EnvironWatcherTest struct {
	implementer Implementer
	st          *state.State
	//backing     *state.State
	hasSecrets bool
}

func NewEnvironWatcherTest(
	implementer Implementer,
	st *state.State,
	//backing *state.State,
	hasSecrets bool) *EnvironWatcherTest {
	//return &EnvironWatcherTest{implementer, st, backing, hasSecrets}
	return &EnvironWatcherTest{implementer, st, hasSecrets}
}

// AssertEnvironConfig provides a method to test the config from the
// implementer.  This allows other tests that embed this type to have
// more than just the default test.
func (s *EnvironWatcherTest) AssertEnvironConfig(c *gc.C, implementer Implementer, hasSecrets bool) {
	envConfig, err := s.st.EnvironConfig()
	c.Assert(err, gc.IsNil)

	result, err := implementer.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)

	configAttributes := envConfig.AllAttrs()
	// If the implementor doesn't provide secrets, we need to replace the config
	// values in our environment to compare against with the secrets replaced.
	if !s.hasSecrets {
		env, err := environs.New(envConfig)
		c.Assert(err, gc.IsNil)
		secretAttrs, err := env.Provider().SecretAttrs(envConfig)
		c.Assert(err, gc.IsNil)
		for key := range secretAttrs {
			configAttributes[key] = "not available"
		}
	}

	c.Assert(result.Config, jc.DeepEquals, params.EnvironConfig(configAttributes))
}

func (s *EnvironWatcherTest) TestEnvironConfig(c *gc.C) {
	s.AssertEnvironConfig(c, s.implementer, s.hasSecrets)
}
