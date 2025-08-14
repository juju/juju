// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"fmt"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	corebase "github.com/juju/juju/core/base"
	instance "github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type machineConfigSuite struct {
	store        *MockObjectStore
	cloudService *commonmocks.MockCloudService

	controllerConfigService *MockControllerConfigService
	controllerNodeService   *MockControllerNodeService
	keyUpdaterService       *MockKeyUpdaterService
	modelConfigService      *MockModelConfigService
	machineService          *MockMachineService
	bootstrapEnviron        *MockBootstrapEnviron
	agentPasswordService    *MockAgentPasswordService
	agentBinaryService      *MockAgentBinaryService
}

func TestMachineConfigSuite(t *testing.T) {
	tc.Run(t, &machineConfigSuite{})
}

func (s *machineConfigSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)

	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.store = NewMockObjectStore(ctrl)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.bootstrapEnviron = NewMockBootstrapEnviron(ctrl)
	s.agentPasswordService = NewMockAgentPasswordService(ctrl)
	s.agentBinaryService = NewMockAgentBinaryService(ctrl)

	c.Cleanup(func() {
		s.controllerNodeService = nil
		s.controllerConfigService = nil
		s.cloudService = nil
		s.store = nil
		s.keyUpdaterService = nil
		s.modelConfigService = nil
		s.machineService = nil
		s.bootstrapEnviron = nil
		s.agentBinaryService = nil
	})

	return ctrl
}

func (s *machineConfigSuite) TestMachineConfig(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()

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

	s.machineService.EXPECT().GetMachineBase(gomock.Any(), coremachine.Name("0")).Return(corebase.MustParseBaseFromString("ubuntu@20.04/stable"), nil)

	hc := instance.MustParseHardware("mem=4G arch=amd64")

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return("deadbeef", nil)
	s.machineService.EXPECT().GetHardwareCharacteristics(gomock.Any(), coremachine.UUID("deadbeef")).Return(&hc, nil)
	s.agentPasswordService.EXPECT().SetMachinePassword(gomock.Any(), coremachine.Name("0"), gomock.Any()).Return(nil)

	metadata := []agentbinary.Metadata{{
		Version: "2.6.6",
		Arch:    "amd64",
	}}
	s.agentBinaryService.EXPECT().ListAgentBinaries(gomock.Any()).Return(metadata, nil)

	addrs := []string{"1.2.3.4:1"}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil).MinTimes(2)

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
		AgentBinaryService:      s.agentBinaryService,
	}

	modelID := coremodel.GenUUID(c)

	icfg, err := InstanceConfig(c.Context(), controllerUUID.String(), modelID, services, "0", "nonce", "")
	c.Check(err, tc.ErrorIsNil)
	c.Check(icfg.APIInfo.Addrs, tc.DeepEquals, []string{"1.2.3.4:1"})
	c.Check(icfg.ToolsList().URLs(), tc.DeepEquals, map[semversion.Binary][]string{
		icfg.AgentVersion(): {fmt.Sprintf("https://1.2.3.4:1/model/%s/tools/2.6.6-ubuntu-amd64", modelID.String())},
	})
	c.Check(icfg.AuthorizedKeys, tc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1")
}
