// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/version"
)

type ModelOperatorSuite struct {
	internaltesting.BaseSuite

	authorizer              *apiservertesting.FakeAuthorizer
	api                     *API
	resources               *common.Resources
	state                   *mockState
	controllerConfigService *MockControllerConfigService
	modelConfigService      *MockModelConfigService
}

var _ = gc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) TestProvisioningInfo(c *gc.C) {
	ctrl := m.setupMocks(c)
	defer ctrl.Finish()

	m.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(config.New(false, map[string]any{
		config.NameKey:         "controller",
		config.UUIDKey:         "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		config.TypeKey:         "ec2",
		config.AgentVersionKey: "4.0.0",
	}))

	info, err := m.api.ModelOperatorProvisioningInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	controllerConf, err := m.state.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	imagePath, err := podcfg.GetJujuOCIImagePath(controllerConf, info.Version)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imagePath, gc.Equals, info.ImageDetails.RegistryPath)

	c.Assert(info.ImageDetails.Auth, gc.Equals, `xxxxx==`)
	c.Assert(info.ImageDetails.Repository, gc.Equals, `test-account`)

	expectedVersion, err := version.Parse("4.0.0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Version, jc.DeepEquals, expectedVersion)
}

func (m *ModelOperatorSuite) TestWatchProvisioningInfo(c *gc.C) {
	defer m.setupMocks(c).Finish()

	controllerConfigChanged := make(chan []string, 1)
	modelConfigChanged := make(chan []string, 1)
	apiHostPortsForAgentsChanged := make(chan struct{}, 1)

	controllerConfigWatcher := watchertest.NewMockStringsWatcher(controllerConfigChanged)
	m.controllerConfigService.EXPECT().WatchControllerConfig().Return(controllerConfigWatcher, nil)

	m.state.apiHostPortsForAgentsWatcher = watchertest.NewMockNotifyWatcher(apiHostPortsForAgentsChanged)

	modelConfigWatcher := watchertest.NewMockStringsWatcher(modelConfigChanged)
	m.modelConfigService.EXPECT().Watch().Return(modelConfigWatcher, nil)

	controllerConfigChanged <- []string{}
	apiHostPortsForAgentsChanged <- struct{}{}
	modelConfigChanged <- []string{}

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

	m.modelConfigService = NewMockModelConfigService(ctrl)

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

	api, err := NewAPI(m.authorizer, m.resources, m.state, m.state,
		m.controllerConfigService, m.modelConfigService,
		loggertesting.WrapCheckLog(c), model.UUID(internaltesting.ModelTag.Id()))
	c.Assert(err, jc.ErrorIsNil)

	m.api = api

	return ctrl
}
