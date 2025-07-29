// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	modeloperatorapi "github.com/juju/juju/api/controller/caasmodeloperator"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/worker/caasmodeloperator"
)

type dummyAPI struct {
	provInfo      func() (modeloperatorapi.ModelOperatorProvisioningInfo, error)
	setPassword   func(password string) error
	watchProvInfo func() (watcher.NotifyWatcher, error)
}

type dummyBroker struct {
	ensureModelOperator             func(string, string, *caas.ModelOperatorConfig) error
	modelOperator                   func() (*caas.ModelOperatorConfig, error)
	modelOperatorExists             func() (bool, error)
	getModelOperatorDeploymentImage func() (string, error)
}

type ModelOperatorManagerSuite struct{}

var _ = gc.Suite(&ModelOperatorManagerSuite{})

func (b *dummyBroker) EnsureModelOperator(modelUUID, agentPath string, c *caas.ModelOperatorConfig) error {
	if b.ensureModelOperator == nil {
		return nil
	}
	return b.ensureModelOperator(modelUUID, agentPath, c)
}

func (b *dummyBroker) ModelOperator() (*caas.ModelOperatorConfig, error) {
	if b.modelOperator == nil {
		return nil, nil
	}
	return b.modelOperator()
}

func (b *dummyBroker) ModelOperatorExists() (bool, error) {
	if b.modelOperatorExists == nil {
		return false, nil
	}
	return b.modelOperatorExists()
}

func (b *dummyBroker) GetModelOperatorDeploymentImage() (string, error) {
	if b.getModelOperatorDeploymentImage == nil {
		return "ghcr.io/juju/jujud-operator:3.6.9", nil
	}
	return b.getModelOperatorDeploymentImage()
}

func (a *dummyAPI) ModelOperatorProvisioningInfo() (modeloperatorapi.ModelOperatorProvisioningInfo, error) {
	if a.provInfo == nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, nil
	}
	return a.provInfo()
}

func (a *dummyAPI) WatchModelOperatorProvisioningInfo() (watcher.NotifyWatcher, error) {
	if a.watchProvInfo == nil {
		return watcher.NewMultiNotifyWatcher(), nil
	}
	return a.watchProvInfo()
}

func (a *dummyAPI) SetPassword(p string) error {
	if a.setPassword == nil {
		return nil
	}
	return a.setPassword(p)
}

func (m *ModelOperatorManagerSuite) TestModelOperatorManagerApplying(c *gc.C) {
	const n = 3
	var (
		iteration = 0 // ... n

		apiAddresses      = [n][]string{{"fe80:abcd::1"}, {"fe80:abcd::2"}, {"fe80:abcd::3"}}
		modelUUID         = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
		imagePath         = [n]string{"docker.io/jujusolutions/jujud-operator:1", "docker.io/jujusolutions/jujud-operator:2", "docker.io/jujusolutions/jujud-operator:3"}
		ver               = [n]version.Number{version.MustParse("2.8.2"), version.MustParse("2.9.1"), version.MustParse("2.9.99")}
		expectedImagePath = [n]string{"docker.io/jujusolutions/jujud-operator:1", "docker.io/jujusolutions/jujud-operator:2.9.1", "docker.io/jujusolutions/jujud-operator:2.9.99"}
		password          = ""
		lastConfig        = (*caas.ModelOperatorConfig)(nil)
	)

	changed := make(chan struct{})
	api := &dummyAPI{
		provInfo: func() (modeloperatorapi.ModelOperatorProvisioningInfo, error) {
			return modeloperatorapi.ModelOperatorProvisioningInfo{
				APIAddresses: apiAddresses[iteration],
				ImageDetails: resources.DockerImageDetails{RegistryPath: imagePath[iteration]},
				Version:      ver[iteration],
			}, nil
		},
		watchProvInfo: func() (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(changed), nil
		},
	}

	broker := &dummyBroker{
		ensureModelOperator: func(_, _ string, conf *caas.ModelOperatorConfig) error {
			defer func() {
				iteration++
			}()
			lastConfig = conf

			c.Check(conf.ImageDetails.RegistryPath, gc.Equals, expectedImagePath[iteration])

			ac, err := agent.ParseConfigData(conf.AgentConf)
			c.Check(err, jc.ErrorIsNil)
			if err != nil {
				return err
			}
			addresses, _ := ac.APIAddresses()
			c.Check(addresses, gc.DeepEquals, apiAddresses[iteration])
			c.Check(ac.UpgradedToVersion(), gc.Equals, ver[iteration])

			if password == "" {
				password = ac.OldPassword()
			}
			c.Check(ac.OldPassword(), gc.Equals, password)
			c.Check(ac.OldPassword(), gc.HasLen, 24)

			return nil
		},
		modelOperatorExists: func() (bool, error) {
			return iteration > 0, nil
		},
		modelOperator: func() (*caas.ModelOperatorConfig, error) {
			if iteration == 0 {
				return nil, errors.NotFoundf("model operator")
			}
			return lastConfig, nil
		},
		getModelOperatorDeploymentImage: func() (string, error) {
			return imagePath[iteration], nil
		},
	}

	worker, err := caasmodeloperator.NewModelOperatorManager(loggo.Logger{},
		api, broker, modelUUID, &mockAgentConfig{})
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < n; i++ {
		changed <- struct{}{}
	}

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(iteration, gc.Equals, n)
}
