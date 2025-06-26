// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	instance "github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

type machineConfigSuite struct {
	ctrlSt       *MockControllerBackend
	st           *MockInstanceConfigBackend
	store        *MockObjectStore
	cloudService *commonmocks.MockCloudService

	controllerConfigService *MockControllerConfigService
	controllerNodeService   *MockControllerNodeService
	keyUpdaterService       *MockKeyUpdaterService
	modelConfigService      *MockModelConfigService
	machineService          *MockMachineService
	bootstrapEnviron        *MockBootstrapEnviron
	agentPasswordService    *MockAgentPasswordService
}

func TestMachineConfigSuite(t *testing.T) {
	tc.Run(t, &machineConfigSuite{})
}

func (s *machineConfigSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)

	s.ctrlSt = NewMockControllerBackend(ctrl)
	s.st = NewMockInstanceConfigBackend(ctrl)
	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.store = NewMockObjectStore(ctrl)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.bootstrapEnviron = NewMockBootstrapEnviron(ctrl)
	s.agentPasswordService = NewMockAgentPasswordService(ctrl)

	c.Cleanup(func() {
		s.controllerNodeService = nil
		s.controllerConfigService = nil
		s.ctrlSt = nil
		s.st = nil
		s.cloudService = nil
		s.store = nil
		s.keyUpdaterService = nil
		s.modelConfigService = nil
		s.machineService = nil
		s.bootstrapEnviron = nil
	})

	return ctrl
}

func (s *machineConfigSuite) TestMachineConfig(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        true,
		"enable-os-refresh-update": true,
	}))
	c.Assert(err, tc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(
		cfg, nil,
	)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil).AnyTimes()

	machine0 := NewMockMachine(ctrl)
	machine0.EXPECT().Base().Return(state.Base{OS: "ubuntu", Channel: "20.04/stable"}).AnyTimes()
	machine0.EXPECT().Tag().Return(names.NewMachineTag("0")).AnyTimes()
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	hc := instance.MustParseHardware("mem=4G arch=amd64")

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return("deadbeef", nil)
	s.machineService.EXPECT().GetHardwareCharacteristics(gomock.Any(), coremachine.UUID("deadbeef")).Return(&hc, nil)
	s.agentPasswordService.EXPECT().SetMachinePassword(gomock.Any(), coremachine.Name("0"), gomock.Any()).Return(nil)

	storageCloser := NewMockStorageCloser(ctrl)
	storageCloser.EXPECT().AllMetadata().Return([]binarystorage.Metadata{{
		Version: "2.6.6-ubuntu-amd64",
	}}, nil)
	storageCloser.EXPECT().Close().Return(nil)
	s.st.EXPECT().ToolsStorage(gomock.Any()).Return(storageCloser, nil)

	addrs := []string{"1.2.3.4:1"}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgentsInPreferredOrder(gomock.Any()).Return(addrs, nil).MinTimes(1)
	addrs2 := map[string][]string{"one": {"1.2.3.4:1"}}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs2, nil).MinTimes(1)
	s.ctrlSt.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()

	s.keyUpdaterService.EXPECT().GetAuthorisedKeysForMachine(
		gomock.Any(), coremachine.Name("0"),
	).Return([]string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
	}, nil)

	services := InstanceConfigServices{
		ControllerConfigService: s.controllerConfigService,
		ControllerNodeService:   s.controllerNodeService,
		CloudService:            s.cloudService,
		ObjectStore:             s.store,
		KeyUpdaterService:       s.keyUpdaterService,
		ModelConfigService:      s.modelConfigService,
		MachineService:          s.machineService,
		AgentPasswordService:    s.agentPasswordService,
	}

	modelID := modeltesting.GenModelUUID(c)

	providerGetter := func(_ context.Context) (environs.BootstrapEnviron, error) {
		return s.bootstrapEnviron, nil
	}

	icfg, err := InstanceConfig(c.Context(), modelID, providerGetter, s.ctrlSt, s.st, services, "0", "nonce", "")
	c.Check(err, tc.ErrorIsNil)
	c.Check(icfg.APIInfo.Addrs, tc.DeepEquals, []string{"1.2.3.4:1"})
	c.Check(icfg.ToolsList().URLs(), tc.DeepEquals, map[semversion.Binary][]string{
		icfg.AgentVersion(): {fmt.Sprintf("https://1.2.3.4:1/model/%s/tools/2.6.6-ubuntu-amd64", modelID.String())},
	})
	c.Check(icfg.AuthorizedKeys, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1")
}
