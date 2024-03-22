// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	coretesting "github.com/juju/juju/testing"
)

type machineConfigSuite struct {
	ctrlSt       *MockControllerBackend
	st           *MockInstanceConfigBackend
	store        *MockObjectStore
	cloudService *commonmocks.MockCloudService
	credService  *commonmocks.MockCredentialService
	model        *MockModel

	controllerConfigService *MockControllerConfigService
}

var _ = gc.Suite(&machineConfigSuite{})

func (s *machineConfigSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	s.ctrlSt = NewMockControllerBackend(ctrl)
	s.st = NewMockInstanceConfigBackend(ctrl)
	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.credService = commonmocks.NewMockCredentialService(ctrl)
	s.store = NewMockObjectStore(ctrl)

	s.model = NewMockModel(ctrl)
	s.model.EXPECT().UUID().Return("uuid").AnyTimes()
	s.model.EXPECT().ModelTag().Return(coretesting.ModelTag).AnyTimes()

	return ctrl
}

func (s *machineConfigSuite) TestMachineConfig(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.st.EXPECT().Model().Return(s.model, nil)
	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        true,
		"enable-os-refresh-update": true,
	})))
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil).AnyTimes()

	machine0 := NewMockMachine(ctrl)
	machine0.EXPECT().Base().Return(state.Base{OS: "ubuntu", Channel: "20.04/stable"}).AnyTimes()
	machine0.EXPECT().Tag().Return(names.NewMachineTag("0")).AnyTimes()
	hc := instance.MustParseHardware("mem=4G arch=amd64")
	machine0.EXPECT().HardwareCharacteristics().Return(&hc, nil)
	machine0.EXPECT().SetPassword(gomock.Any()).Return(nil)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	storageCloser := NewMockStorageCloser(ctrl)
	storageCloser.EXPECT().AllMetadata().Return([]binarystorage.Metadata{{
		Version: "2.6.6-ubuntu-amd64",
	}}, nil)
	storageCloser.EXPECT().Close().Return(nil)
	s.st.EXPECT().ToolsStorage(gomock.Any()).Return(storageCloser, nil)

	s.ctrlSt.EXPECT().APIHostPortsForAgents(gomock.Any()).Return([]network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}, nil).MinTimes(1)
	s.ctrlSt.EXPECT().ControllerConfig().Return(coretesting.FakeControllerConfig(), nil).MinTimes(1)
	s.ctrlSt.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()

	services := machinemanager.InstanceConfigServices{
		ControllerConfigService: s.controllerConfigService,
		CloudService:            s.cloudService,
		CredentialService:       s.credService,
		ObjectStore:             s.store,
	}

	icfg, err := machinemanager.InstanceConfig(context.Background(), s.ctrlSt, s.st, services, "0", "nonce", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(icfg.APIInfo.Addrs, gc.DeepEquals, []string{"1.2.3.4:1"})
	c.Assert(icfg.ToolsList().URLs(), gc.DeepEquals, map[version.Binary][]string{
		icfg.AgentVersion(): {"https://1.2.3.4:1/model/uuid/tools/2.6.6-ubuntu-amd64"},
	})
}
