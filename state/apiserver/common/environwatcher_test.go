// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
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
}

var _ = gc.Suite(&environWatcherSuite{})

type fakeEnvironAccessor struct {
	envConfig      map[string]interface{}
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
	return config.New(config.UseDefaults, f.envConfig)
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

var testingEnvConfig = testing.FakeConfig().Merge(map[string]interface{}{
	"type":         "dummy",
	"name":         "none",
	"state-server": false,
	"state-id":     "1", // needed by the dummy provider to signal an environment is prepared.
	"secret":       "pork",
	// These are optional, but with defaults; we still need them
	// in order to compare the retrieved config, after it passes
	// the config.Validate method.
	"logging-config":     "<root>=DEBUG",
	"tools-metadata-url": "",
	"charm-store-auth":   "",
	"tools-url":          "",
	"syslog-port":        1234,
	"image-metadata-url": "",
})

func (*environWatcherSuite) TestEnvironConfigSuccess(c *gc.C) {
	getCanReadSecrets := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return true
		}, nil
	}

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
	c.Check(testing.Attrs(result.Config), jc.DeepEquals, testingEnvConfig)
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
		&fakeEnvironAccessor{envConfig: testingEnvConfig},
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
	c.Check(testing.Attrs(result.Config), jc.DeepEquals, testingEnvConfig)
}
