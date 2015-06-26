// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type environWatcherSuite struct {
	testing.BaseSuite

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
	return &fakeNotifyWatcher{changes: changes}
}

func (f *fakeEnvironAccessor) EnvironConfig() (*config.Config, error) {
	if f.envConfigError != nil {
		return nil, f.envConfigError
	}
	return f.envConfig, nil
}

func (s *environWatcherSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.BaseSuite.TearDownTest(c)
}

func (s *environWatcherSuite) TestWatchSuccess(c *gc.C) {
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{},
		resources,
		nil,
	)
	result, err := e.WatchForEnvironConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{"1", nil})
	c.Assert(resources.Count(), gc.Equals, 1)
}

func (*environWatcherSuite) TestEnvironConfigSuccess(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	testingEnvConfig := testingEnvConfig(c)
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{envConfig: testingEnvConfig},
		nil,
		authorizer,
	)
	result, err := e.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	// Make sure we can read the secret attribute (i.e. it's not masked).
	c.Check(result.Config["secret"], gc.Equals, "pork")
	c.Check(map[string]interface{}(result.Config), jc.DeepEquals, testingEnvConfig.AllAttrs())
}

func (*environWatcherSuite) TestEnvironConfigFetchError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{
			envConfigError: fmt.Errorf("pow"),
		},
		nil,
		authorizer,
	)
	_, err := e.EnvironConfig()
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*environWatcherSuite) TestEnvironConfigMaskedSecrets(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: false,
	}
	testingEnvConfig := testingEnvConfig(c)
	e := common.NewEnvironWatcher(
		&fakeEnvironAccessor{envConfig: testingEnvConfig},
		nil,
		authorizer,
	)
	result, err := e.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	// Make sure the secret attribute is masked.
	c.Check(result.Config["secret"], gc.Equals, "not available")
	// And only that is masked.
	result.Config["secret"] = "pork"
	c.Check(map[string]interface{}(result.Config), jc.DeepEquals, testingEnvConfig.AllAttrs())
}

func testingEnvConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	env, err := environs.Prepare(cfg, envcmd.BootstrapContext(testing.Context(c)), configstore.NewMem())
	c.Assert(err, jc.ErrorIsNil)
	return env.Config()
}
