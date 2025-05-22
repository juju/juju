// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	modeloperatorapi "github.com/juju/juju/api/controller/caasmodeloperator"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasmodeloperator"
)

type dummyAPI struct {
	provInfo      func() (modeloperatorapi.ModelOperatorProvisioningInfo, error)
	setPassword   func(password string) error
	watchProvInfo func() (watcher.NotifyWatcher, error)
}

type dummyBroker struct {
	ensureModelOperator func(context.Context, string, string, *caas.ModelOperatorConfig) error
	modelOperator       func(ctx context.Context) (*caas.ModelOperatorConfig, error)
	modelOperatorExists func(context.Context) (bool, error)
}

type ModelOperatorManagerSuite struct{}

func TestModelOperatorManagerSuite(t *testing.T) {
	tc.Run(t, &ModelOperatorManagerSuite{})
}
func (b *dummyBroker) EnsureModelOperator(ctx context.Context, modelUUID, agentPath string, c *caas.ModelOperatorConfig) error {
	if b.ensureModelOperator == nil {
		return nil
	}
	return b.ensureModelOperator(ctx, modelUUID, agentPath, c)
}

func (b *dummyBroker) ModelOperator(ctx context.Context) (*caas.ModelOperatorConfig, error) {
	if b.modelOperator == nil {
		return nil, nil
	}
	return b.modelOperator(ctx)
}

func (b *dummyBroker) ModelOperatorExists(ctx context.Context) (bool, error) {
	if b.modelOperatorExists == nil {
		return false, nil
	}
	return b.modelOperatorExists(ctx)
}

func (a *dummyAPI) ModelOperatorProvisioningInfo(ctx context.Context) (modeloperatorapi.ModelOperatorProvisioningInfo, error) {
	if a.provInfo == nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, nil
	}
	return a.provInfo()
}

func (a *dummyAPI) WatchModelOperatorProvisioningInfo(ctx context.Context) (watcher.NotifyWatcher, error) {
	if a.watchProvInfo == nil {
		return eventsource.NewMultiNotifyWatcher(ctx)
	}
	return a.watchProvInfo()
}

func (a *dummyAPI) SetPassword(ctx context.Context, p string) error {
	if a.setPassword == nil {
		return nil
	}
	return a.setPassword(p)
}

func (m *ModelOperatorManagerSuite) TestModelOperatorManagerApplying(c *tc.C) {
	const n = 3
	var (
		iteration = 0 // ... n

		apiAddresses = [n][]string{{"fe80:abcd::1"}, {"fe80:abcd::2"}, {"fe80:abcd::3"}}
		imagePath    = [n]string{"juju/jujud:1", "juju/jujud:2", "juju/jujud:3"}
		modelUUID    = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
		ver          = [n]semversion.Number{semversion.MustParse("2.8.2"), semversion.MustParse("2.9.1"), semversion.MustParse("2.9.99")}

		password   = ""
		lastConfig = (*caas.ModelOperatorConfig)(nil)
	)

	changed := make(chan struct{})
	api := &dummyAPI{
		provInfo: func() (modeloperatorapi.ModelOperatorProvisioningInfo, error) {
			return modeloperatorapi.ModelOperatorProvisioningInfo{
				APIAddresses: apiAddresses[iteration],
				ImageDetails: resource.DockerImageDetails{RegistryPath: imagePath[iteration]},
				Version:      ver[iteration],
			}, nil
		},
		watchProvInfo: func() (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(changed), nil
		},
	}

	broker := &dummyBroker{
		ensureModelOperator: func(_ context.Context, _, _ string, conf *caas.ModelOperatorConfig) error {
			defer func() {
				iteration++
			}()
			lastConfig = conf

			c.Check(conf.ImageDetails.RegistryPath, tc.Equals, imagePath[iteration])

			ac, err := agent.ParseConfigData(conf.AgentConf)
			c.Check(err, tc.ErrorIsNil)
			if err != nil {
				return err
			}
			addresses, _ := ac.APIAddresses()
			c.Check(addresses, tc.DeepEquals, apiAddresses[iteration])
			c.Check(ac.UpgradedToVersion(), tc.Equals, ver[iteration])

			if password == "" {
				password = ac.OldPassword()
			}
			c.Check(ac.OldPassword(), tc.Equals, password)
			c.Check(ac.OldPassword(), tc.HasLen, 24)

			return nil
		},
		modelOperatorExists: func(context.Context) (bool, error) {
			return iteration > 0, nil
		},
		modelOperator: func(context.Context) (*caas.ModelOperatorConfig, error) {
			if iteration == 0 {
				return nil, errors.NotFoundf("model operator")
			}
			return lastConfig, nil
		},
	}

	worker, err := caasmodeloperator.NewModelOperatorManager(
		loggertesting.WrapCheckLog(c),
		api, broker, modelUUID, &mockAgentConfig{})
	c.Assert(err, tc.ErrorIsNil)

	for i := 0; i < n; i++ {
		changed <- struct{}{}
	}

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(iteration, tc.Equals, n)
}
