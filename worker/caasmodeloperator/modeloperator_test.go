// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	modeloperatorapi "github.com/juju/juju/api/caasmodeloperator"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/worker/caasmodeloperator"
)

type dummyAPI struct {
	provInfo    func() (modeloperatorapi.ModelOperatorProvisioningInfo, error)
	setPassword func(password string) error
}

type dummyBroker struct {
	ensureModelOperator func(string, string, *caas.ModelOperatorConfig) error
	modelOperator       func() (*caas.ModelOperatorConfig, error)
	modelOperatorExists func() (bool, error)
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

func (a *dummyAPI) ModelOperatorProvisioningInfo() (modeloperatorapi.ModelOperatorProvisioningInfo, error) {
	if a.provInfo == nil {
		return modeloperatorapi.ModelOperatorProvisioningInfo{}, nil
	}
	return a.provInfo()
}

func (a *dummyAPI) SetPassword(p string) error {
	if a.setPassword == nil {
		return nil
	}
	return a.setPassword(p)
}

func (m *ModelOperatorManagerSuite) TestModelOperatorManagerConstruction(c *gc.C) {
	var (
		apiAddresses = []string{"fe80:abcd::1"}
		apiCalled    = false
		brokerCalled = false
		imagePath    = "juju/jujud:1"
		modelUUID    = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
		ver          = version.MustParse("2.8.2")
	)

	api := &dummyAPI{
		provInfo: func() (modeloperatorapi.ModelOperatorProvisioningInfo, error) {
			apiCalled = true
			return modeloperatorapi.ModelOperatorProvisioningInfo{
				APIAddresses: apiAddresses,
				ImagePath:    imagePath,
				Version:      ver,
			}, nil
		},
	}

	broker := &dummyBroker{
		ensureModelOperator: func(_, _ string, conf *caas.ModelOperatorConfig) error {
			c.Assert(conf.OperatorImagePath, gc.Equals, imagePath)
			brokerCalled = true
			return nil
		},
	}

	worker, err := caasmodeloperator.NewModelOperatorManager(loggo.Logger{},
		api, broker, modelUUID, &mockAgentConfig{})
	c.Assert(err, jc.ErrorIsNil)

	worker.Kill()
	err = worker.Wait()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiCalled, gc.Equals, true)
	c.Assert(brokerCalled, gc.Equals, true)
}
