// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type ModelOperatorSuite struct {
	coretesting.BaseSuite

	authorizer              *apiservertesting.FakeAuthorizer
	api                     *API
	resources               *common.Resources
	state                   *mockState
	controllerConfigService *MockControllerConfigService
}

var _ = gc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) TestProvisioningInfo(c *gc.C) {
	ctrl := m.setupMocks(c)
	defer ctrl.Finish()

	info, err := m.api.ModelOperatorProvisioningInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	controllerConf, err := m.state.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	imagePath, err := podcfg.GetJujuOCIImagePath(controllerConf, info.Version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imagePath, gc.Equals, info.ImageDetails.RegistryPath)

	c.Assert(info.ImageDetails.Auth, gc.Equals, `xxxxx==`)
	c.Assert(info.ImageDetails.Repository, gc.Equals, `test-account`)

	model, err := m.state.Model()
	c.Assert(err, jc.ErrorIsNil)

	modelConfig, err := model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	vers, ok := modelConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)

	c.Assert(vers, jc.DeepEquals, info.Version)
}

func (m *ModelOperatorSuite) TestWatchProvisioningInfo(c *gc.C) {
	defer m.setupMocks(c).Finish()

	controllerConfigChanged := make(chan []string, 1)
	modelConfigChanged := make(chan struct{}, 1)
	apiHostPortsForAgentsChanged := make(chan struct{}, 1)

	watcher := watchertest.NewMockStringsWatcher(controllerConfigChanged)
	m.controllerConfigService.EXPECT().WatchControllerConfig().Return(watcher, nil)

	m.state.apiHostPortsForAgentsWatcher = statetesting.NewMockNotifyWatcher(apiHostPortsForAgentsChanged)
	m.state.model.modelConfigChanged = statetesting.NewMockNotifyWatcher(modelConfigChanged)

	controllerConfigChanged <- []string{}
	apiHostPortsForAgentsChanged <- struct{}{}
	modelConfigChanged <- struct{}{}

	results, err := m.api.WatchModelOperatorProvisioningInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	res := m.resources.Get("1")
	c.Assert(res, gc.FitsTypeOf, (*eventsource.MultiWatcher[struct{}])(nil))
}

func (m *ModelOperatorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	m.controllerConfigService = NewMockControllerConfigService(ctrl)
	m.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.CAASImageRepo: `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
	}, nil).AnyTimes()

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

	api, err := NewAPI(m.authorizer, m.resources, m.state, m.state, m.controllerConfigService, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)

	m.api = api

	return ctrl
}
