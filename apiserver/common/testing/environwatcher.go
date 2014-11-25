// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

const (
	HasSecrets = true
	NoSecrets  = false
)

type EnvironmentWatcher interface {
	WatchForEnvironConfigChanges() (params.NotifyWatchResult, error)
	EnvironConfig() (params.EnvironConfigResult, error)
}

type EnvironWatcherTest struct {
	envWatcher EnvironmentWatcher
	st         *state.State
	resources  *common.Resources
	hasSecrets bool
}

func NewEnvironWatcherTest(
	envWatcher EnvironmentWatcher,
	st *state.State,
	resources *common.Resources,
	hasSecrets bool) *EnvironWatcherTest {
	return &EnvironWatcherTest{envWatcher, st, resources, hasSecrets}
}

// AssertEnvironConfig provides a method to test the config from the
// envWatcher.  This allows other tests that embed this type to have
// more than just the default test.
func (s *EnvironWatcherTest) AssertEnvironConfig(c *gc.C, envWatcher EnvironmentWatcher, hasSecrets bool) {
	envConfig, err := s.st.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	result, err := envWatcher.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	configAttributes := envConfig.AllAttrs()
	// If the implementor doesn't provide secrets, we need to replace the config
	// values in our environment to compare against with the secrets replaced.
	if !hasSecrets {
		env, err := environs.New(envConfig)
		c.Assert(err, jc.ErrorIsNil)
		secretAttrs, err := env.Provider().SecretAttrs(envConfig)
		c.Assert(err, jc.ErrorIsNil)
		for key := range secretAttrs {
			configAttributes[key] = "not available"
		}
	}

	c.Assert(result.Config, jc.DeepEquals, params.EnvironConfig(configAttributes))
}

func (s *EnvironWatcherTest) TestEnvironConfig(c *gc.C) {
	s.AssertEnvironConfig(c, s.envWatcher, s.hasSecrets)
}

func (s *EnvironWatcherTest) TestWatchForEnvironConfigChanges(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	result, err := s.envWatcher.WatchForEnvironConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned"
	// in the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.st, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}
