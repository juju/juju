// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type ModelOperatorSuite struct {
	internaltesting.BaseSuite

	authorizer              *apiservertesting.FakeAuthorizer
	api                     *API
	state                   *mockState
	controllerConfigService *MockControllerConfigService
	controllerNodeService   *MockControllerNodeService
	modelConfigService      *MockModelConfigService
	passwordService         *MockAgentPasswordService
	watcherRegistry         *facademocks.MockWatcherRegistry
}

func TestModelOperatorSuite(t *testing.T) {
	tc.Run(t, &ModelOperatorSuite{})
}

func (m *ModelOperatorSuite) TestProvisioningInfo(c *tc.C) {
	defer m.setupMocks(c).Finish()
	// Arrange
	m.expectControllerConfig()

	addrs := []string{"addresses:1"}
	m.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)
	m.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(config.New(false, map[string]any{
		config.NameKey:         "controller",
		config.UUIDKey:         "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		config.TypeKey:         "ec2",
		config.AgentVersionKey: "4.0.0",
	}))

	// Act
	info, err := m.api.ModelOperatorProvisioningInfo(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	controllerConf, err := m.state.ControllerConfig()
	c.Assert(err, tc.ErrorIsNil)

	imagePath, err := podcfg.GetJujuOCIImagePath(controllerConf, info.Version)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imagePath, tc.Equals, info.ImageDetails.RegistryPath)

	c.Assert(info.ImageDetails.Auth, tc.Equals, `xxxxx==`)
	c.Assert(info.ImageDetails.Repository, tc.Equals, `test-account`)

	expectedVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info.Version, tc.DeepEquals, expectedVersion)
}

func (m *ModelOperatorSuite) TestWatchProvisioningInfo(c *tc.C) {
	defer m.setupMocks(c).Finish()

	m.expectControllerConfig()

	controllerConfigChanged := make(chan []string, 1)
	modelConfigChanged := make(chan []string, 1)
	apiHostPortsForAgentsChanged := make(chan struct{}, 1)

	controllerConfigWatcher := watchertest.NewMockStringsWatcher(controllerConfigChanged)
	m.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(controllerConfigWatcher, nil)

	hostPortWatcher := watchertest.NewMockNotifyWatcher(apiHostPortsForAgentsChanged)
	m.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(hostPortWatcher, nil)

	modelConfigWatcher := watchertest.NewMockStringsWatcher(modelConfigChanged)
	m.modelConfigService.EXPECT().Watch().Return(modelConfigWatcher, nil)

	m.watcherRegistry.EXPECT().Register(gomock.Any()).Return("42", nil)

	controllerConfigChanged <- []string{}
	apiHostPortsForAgentsChanged <- struct{}{}
	modelConfigChanged <- []string{}

	results, err := m.api.WatchModelOperatorProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Error, tc.IsNil)
	c.Assert(results.NotifyWatcherId, tc.Equals, "42")
}

func (s *ModelOperatorSuite) TestSetUnitPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(nil)

	api := &API{
		PasswordChanger: common.NewPasswordChanger(s.passwordService, nil, alwaysAllow),
	}

	result, err := api.SetPasswords(c.Context(), params.EntityPasswords{
		Changes: []params.EntityPassword{
			{
				Tag:      names.NewUnitTag("foo/1").String(),
				Password: "password",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: nil,
			},
		},
	})
}

func (s *ModelOperatorSuite) TestSetUnitPasswordUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(applicationerrors.UnitNotFound)

	api := &API{
		PasswordChanger: common.NewPasswordChanger(s.passwordService, nil, alwaysAllow),
	}

	result, err := api.SetPasswords(c.Context(), params.EntityPasswords{
		Changes: []params.EntityPassword{
			{
				Tag:      names.NewUnitTag("foo/1").String(),
				Password: "password",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: apiservererrors.ServerError(errors.NotFoundf(`unit "foo/1"`)),
			},
		},
	})
}

func (m *ModelOperatorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	m.passwordService = NewMockAgentPasswordService(ctrl)
	m.controllerConfigService = NewMockControllerConfigService(ctrl)
	m.controllerNodeService = NewMockControllerNodeService(ctrl)
	m.modelConfigService = NewMockModelConfigService(ctrl)

	m.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

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

	api, err := NewAPI(m.authorizer, m.state, m.passwordService,
		m.controllerConfigService, m.controllerNodeService, m.modelConfigService,
		loggertesting.WrapCheckLog(c), model.UUID(internaltesting.ModelTag.Id()),
		m.watcherRegistry)
	c.Assert(err, tc.ErrorIsNil)

	m.api = api

	c.Cleanup(func() {
		m.api = nil
		m.authorizer = nil
		m.controllerConfigService = nil
		m.controllerNodeService = nil
		m.modelConfigService = nil
		m.passwordService = nil
		m.state = nil
		m.watcherRegistry = nil
	})

	return ctrl
}

func (m *ModelOperatorSuite) expectControllerConfig() {
	m.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controller.Config{
		controller.CAASImageRepo: `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
	}, nil).AnyTimes()

}

func alwaysAllow(ctx context.Context) (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return true
	}, nil
}
