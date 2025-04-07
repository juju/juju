// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
)

type machineConfigSuite struct {
	ctrlSt       *MockControllerBackend
	st           *MockInstanceConfigBackend
	store        *MockObjectStore
	cloudService *commonmocks.MockCloudService

	controllerConfigService *MockControllerConfigService
	agentFinderService      *MockAgentFinderService
	keyUpdaterService       *MockKeyUpdaterService
	modelConfigService      *MockModelConfigService
	machineService          *MockMachineService
	bootstrapEnviron        *MockBootstrapEnviron
}

var _ = gc.Suite(&machineConfigSuite{})

func (s *machineConfigSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	s.ctrlSt = NewMockControllerBackend(ctrl)
	s.st = NewMockInstanceConfigBackend(ctrl)
	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.store = NewMockObjectStore(ctrl)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.agentFinderService = NewMockAgentFinderService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.bootstrapEnviron = NewMockBootstrapEnviron(ctrl)

	return ctrl
}

func (s *machineConfigSuite) TestMachineConfig(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        true,
		"enable-os-refresh-update": true,
	}))
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(
		cfg, nil,
	)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil).AnyTimes()

	machine0 := NewMockMachine(ctrl)
	machine0.EXPECT().Base().Return(state.Base{OS: "ubuntu", Channel: "20.04/stable"}).AnyTimes()
	machine0.EXPECT().Tag().Return(names.NewMachineTag("0")).AnyTimes()
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return("deadbeef", nil)
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef").Return(&hc, nil)
	machine0.EXPECT().SetPassword(gomock.Any()).Return(nil)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	storageCloser := NewMockStorageCloser(ctrl)
	storageCloser.EXPECT().Close()
	s.st.EXPECT().ToolsStorage(gomock.Any()).Return(storageCloser, nil)

	s.ctrlSt.EXPECT().APIHostPortsForAgents(gomock.Any()).Return([]network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}, nil).MinTimes(1)
	s.ctrlSt.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()

	modelID := modeltesting.GenModelUUID(c)
	s.agentFinderService.EXPECT().FindAgents(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, p modelagent.FindAgentsParams) (tools.List, error) {
		c.Assert(p.Number.String(), gc.Equals, "2.6.6")
		c.Assert(p.MajorVersion, gc.Equals, 0)
		c.Assert(p.MinorVersion, gc.Equals, 0)
		c.Assert(p.AgentStream, gc.Equals, "")
		c.Assert(p.Arch, gc.Equals, "amd64")
		c.Assert(p.AgentStorage, jc.DeepEquals, storageCloser)
		c.Assert(p.ToolsURLsGetter, gc.NotNil)
		return tools.List{{
			Version: semversion.MustParseBinary("2.6.6-ubuntu-amd64"),
			URL:     fmt.Sprintf("https://1.2.3.4:1/model/%s/tools/2.6.6-ubuntu-amd64", modelID.String()),
			SHA256:  "deadbeaf",
			Size:    666,
		}}, nil
	})

	s.keyUpdaterService.EXPECT().GetAuthorisedKeysForMachine(
		gomock.Any(), coremachine.Name("0"),
	).Return([]string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
	}, nil)

	services := InstanceConfigServices{
		ControllerConfigService: s.controllerConfigService,
		AgentFinderService:      s.agentFinderService,
		CloudService:            s.cloudService,
		ObjectStore:             s.store,
		KeyUpdaterService:       s.keyUpdaterService,
		ModelConfigService:      s.modelConfigService,
		MachineService:          s.machineService,
	}

	providerGetter := func(_ context.Context) (environs.BootstrapEnviron, error) {
		return s.bootstrapEnviron, nil
	}

	icfg, err := InstanceConfig(context.Background(), modelID, providerGetter, s.ctrlSt, s.st, services, "0", "nonce", "")
	c.Check(err, jc.ErrorIsNil)
	c.Check(icfg.APIInfo.Addrs, gc.DeepEquals, []string{"1.2.3.4:1"})
	c.Check(icfg.ToolsList().URLs(), gc.DeepEquals, map[semversion.Binary][]string{
		icfg.AgentVersion(): {fmt.Sprintf("https://1.2.3.4:1/model/%s/tools/2.6.6-ubuntu-amd64", modelID.String())},
	})
	c.Check(icfg.AuthorizedKeys, gc.Equals, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1")
}
