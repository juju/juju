// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasmodeloperator"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloudconfig/podcfg"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type ModelOperatorSuite struct {
	coretesting.BaseSuite

	authorizer *apiservertesting.FakeAuthorizer
	api        *caasmodeloperator.API
	resources  *common.Resources
	state      *mockState
}

var _ = gc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) SetUpTest(c *gc.C) {
	m.BaseSuite.SetUpTest(c)

	m.resources = common.NewResources()

	m.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewModelTag("model-deadbeef-0bad-400d-8000-4b1d0d06f00d"),
		Controller: true,
	}

	m.state = newMockState()
	m.state.operatorRepo = `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}`[1:]

	c.Logf("m.state.1operatorRepo %q", m.state.operatorRepo)

	api, err := caasmodeloperator.NewAPI(m.authorizer, m.resources, m.state, m.state)
	c.Assert(err, jc.ErrorIsNil)

	m.api = api
}

func (m *ModelOperatorSuite) TestProvisioningInfoImageRepoUnset(c *gc.C) {
	model, err := m.state.Model()
	c.Assert(err, jc.ErrorIsNil)

	modelConfig, err := model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	m.state.model.modelConfig = modelConfig

	vers, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	info, err := m.api.ModelOperatorProvisioningInfo()
	c.Assert(err, jc.ErrorIsNil)

	controllerConf, err := m.state.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	imagePath, err := podcfg.GetJujuOCIImagePath(controllerConf, modelConfig, info.Version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imagePath, gc.Equals, info.ImageDetails.RegistryPath)

	c.Assert(info.ImageDetails.Auth, gc.Equals, `xxxxx==`)
	c.Assert(info.ImageDetails.Repository, gc.Equals, `test-account`)

	c.Assert(vers, jc.DeepEquals, info.Version)
}

func (m *ModelOperatorSuite) TestProvisioningInfoImageRepoSet(c *gc.C) {
	model, err := m.state.Model()

	c.Assert(err, jc.ErrorIsNil)

	modelConfig, err := model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	m.state.model.modelConfig = modelConfig

	vers, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	imageRepo := "ghcr.io/juju"
	err = model.UpdateModelConfig(map[string]interface{}{"model-caas-image-repo": imageRepo}, nil)
	c.Assert(err, jc.ErrorIsNil)

	info, err := m.api.ModelOperatorProvisioningInfo()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(info.ImageDetails.Repository, gc.Equals, imageRepo)

	c.Assert(vers, jc.DeepEquals, info.Version)
}

func (m *ModelOperatorSuite) TestWatchProvisioningInfo(c *gc.C) {
	controllerConfigChanged := make(chan struct{}, 1)
	modelConfigChanged := make(chan struct{}, 1)
	apiHostPortsForAgentsChanged := make(chan struct{}, 1)
	m.state.controllerConfigWatcher = statetesting.NewMockNotifyWatcher(controllerConfigChanged)
	m.state.apiHostPortsForAgentsWatcher = statetesting.NewMockNotifyWatcher(apiHostPortsForAgentsChanged)
	m.state.model.modelConfigChanged = statetesting.NewMockNotifyWatcher(modelConfigChanged)

	controllerConfigChanged <- struct{}{}
	apiHostPortsForAgentsChanged <- struct{}{}
	modelConfigChanged <- struct{}{}

	results, err := m.api.WatchModelOperatorProvisioningInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	res := m.resources.Get("1")
	c.Assert(res, gc.FitsTypeOf, (*common.MultiNotifyWatcher)(nil))
}
