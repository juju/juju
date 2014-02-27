// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type environWatcherSuite struct {
	testbase.LoggingSuite

	testingEnvConfig *config.Config
}

var _ = gc.Suite(&environWatcherSuite{})

type fakeEnvironAccessor struct {
	envConfig      *config.Config
	envConfigError error
}

func (*fakeEnvironAccessor) WatchForEnvironConfigChanges() state.NotifyWatcher {
	changes := make(chan struct{}, 1)
	// Simulate initial event.
	changes <- struct{}{}
	return &fakeNotifyWatcher{changes}
}

func (f *fakeEnvironAccessor) EnvironConfig() (*config.Config, error) {
	if f.envConfigError != nil {
		return nil, f.envConfigError
	}
	return f.envConfig, nil
}

func (s *environWatcherSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func (*environWatcherSuite) TestWatchSuccess(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return true
		}, nil
	}
	resources := common.NewResources()
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{},
		resources,
		getCanWatch,
		nil,
	)
	result, err := e.WatchForEnvironConfigChanges()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{"1", nil})
	c.Assert(resources.Count(), gc.Equals, 1)
}

func (*environWatcherSuite) TestWatchGetAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{},
		resources,
		getCanWatch,
		nil,
	)
	_, err := e.WatchForEnvironConfigChanges()
	c.Assert(err, gc.ErrorMatches, "pow")
	c.Assert(resources.Count(), gc.Equals, 0)
}

func (*environWatcherSuite) TestWatchAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return false
		}, nil
	}
	resources := common.NewResources()
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{},
		resources,
		getCanWatch,
		nil,
	)
	result, err := e.WatchForEnvironConfigChanges()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(result.Error, jc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(resources.Count(), gc.Equals, 0)
}

func (*environWatcherSuite) TestEnvironConfigSuccess(c *gc.C) {
	getCanReadSecrets := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return true
		}, nil
	}

	testingEnvConfig := testingEnvConfig(c)
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{envConfig: testingEnvConfig},
		nil,
		nil,
		getCanReadSecrets,
	)
	result, err := e.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)
	// Make sure we can read the secret attribute (i.e. it's not masked).
	c.Check(result.Config["secret"], gc.Equals, "pork")
	c.Check(map[string]interface{}(result.Config), jc.DeepEquals, testingEnvConfig.AllAttrs())
}

func (*environWatcherSuite) TestEnvironConfigFetchError(c *gc.C) {
	getCanReadSecrets := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return true
		}, nil
	}
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{
			envConfigError: fmt.Errorf("pow"),
		},
		nil,
		nil,
		getCanReadSecrets,
	)
	_, err := e.EnvironConfig()
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*environWatcherSuite) TestEnvironConfigGetAuthError(c *gc.C) {
	getCanReadSecrets := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{envConfig: testingEnvConfig(c)},
		nil,
		nil,
		getCanReadSecrets,
	)
	_, err := e.EnvironConfig()
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*environWatcherSuite) TestEnvironConfigReadSecretsFalse(c *gc.C) {
	getCanReadSecrets := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return false
		}, nil
	}

	testingEnvConfig := testingEnvConfig(c)
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{envConfig: testingEnvConfig},
		nil,
		nil,
		getCanReadSecrets,
	)
	result, err := e.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)
	// Make sure the secret attribute is masked.
	c.Check(result.Config["secret"], gc.Equals, "not available")
	// And only that is masked.
	result.Config["secret"] = "pork"
	c.Check(map[string]interface{}(result.Config), jc.DeepEquals, testingEnvConfig.AllAttrs())
}

func testingEnvConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, testing.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
	return env.Config()
}
