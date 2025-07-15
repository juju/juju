// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/migrationminion"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	controller "github.com/juju/juju/controller"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/modelmigration"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type MigrationStatusWatcherSuite struct {
	coretesting.BaseSuite

	watcherRegistry *facademocks.MockWatcherRegistry
	authorizer      apiservertesting.FakeAuthorizer

	modelMigrationService   *MockModelMigrationService
	controllerNodeService   *MockControllerNodeService
	controllerConfigService *MockControllerConfigService
}

func TestMigrationStatusWatcherSuite(t *testing.T) {
	tc.Run(t, &MigrationStatusWatcherSuite{})
}

func (s *MigrationStatusWatcherSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("99"),
	}

	s.modelMigrationService = NewMockModelMigrationService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	c.Cleanup(func() {
		s.authorizer = apiservertesting.FakeAuthorizer{}
		s.controllerConfigService = nil
		s.controllerNodeService = nil
		s.modelMigrationService = nil
		s.watcherRegistry = nil
	})

	return ctrl
}

func (s *MigrationStatusWatcherSuite) TestWatcher(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(ch)
	defer workertest.CleanKill(c, w)
	s.watcherRegistry.EXPECT().Get("123").Return(w, nil)

	mig := modelmigration.Migration{
		UUID:    "id",
		Phase:   coremigration.IMPORT,
		Attempt: 2,
		Target: coremigration.TargetInfo{
			Addrs:  []string{"192.0.2.1:5555"},
			CACert: "trust me",
		},
	}
	s.modelMigrationService.EXPECT().Migration(gomock.Any()).Return(mig, nil)

	sourceAPIAddresses := []string{
		"192.0.2.2:5555", "192.0.2.3:5555", "192.0.2.4:5555",
	}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForClients(gomock.Any()).Return(sourceAPIAddresses, nil)

	cfg := controller.Config{
		controller.CACertKey: "no worries",
	}
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(cfg, nil)

	api, err := migrationminion.NewMigrationStatusWatcherAPI(
		s.watcherRegistry,
		s.authorizer,
		s.modelMigrationService,
		s.controllerNodeService,
		s.controllerConfigService,
		"123",
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	result, err := api.Next(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.MigrationStatus{
		MigrationId:    "id",
		Attempt:        2,
		Phase:          "IMPORT",
		SourceAPIAddrs: []string{"192.0.2.2:5555", "192.0.2.3:5555", "192.0.2.4:5555"},
		SourceCACert:   "no worries",
		TargetAPIAddrs: []string{"192.0.2.1:5555"},
		TargetCACert:   "trust me",
	})
}
