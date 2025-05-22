// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/agent/provisioner/mocks"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

type provisionerSuite struct {
	testhelpers.IsolationSuite
}

func TestProvisionerSuite(t *testing.T) {
	tc.Run(t, &provisionerSuite{})
}

func (s *provisionerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *provisionerSuite) setupCaller(ctrl *gomock.Controller) *mocks.MockAPICaller {
	caller := mocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("Provisioner").Return(666)
	return caller
}

func (s *provisionerSuite) TestNew(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := s.setupCaller(ctrl)
	client := provisioner.NewClient(caller)
	c.Assert(client.APIAddresser, tc.NotNil)
	c.Assert(client.ModelConfigWatcher, tc.NotNil)
	c.Assert(client.ControllerConfigAPI, tc.NotNil)
}

func (s *provisionerSuite) expectCall(caller *mocks.MockAPICaller, method, args, results interface{}) {
	caller.EXPECT().APICall(gomock.Any(), "Provisioner", 666, "", method, args, gomock.Any()).SetArg(6, results).Return(nil)
}

func (s *provisionerSuite) TestMachines(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}, {Tag: "machine-42"}},
	}
	results := params.LifeResults{
		Results: []params.LifeResult{{
			Life: "alive",
		}, {
			Error: &params.Error{Message: "FAIL"},
		}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "Life", args, results)

	client := provisioner.NewClient(caller)
	result, err := client.Machines(c.Context(), names.NewMachineTag("666"), names.NewMachineTag("42"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)
	c.Assert(result[1].Err.Message, tc.Equals, "FAIL")

	machine := result[0].Machine
	c.Assert(machine, tc.FitsTypeOf, &provisioner.Machine{})
	c.Assert(machine.Tag(), tc.Equals, names.NewMachineTag("666"))
	c.Assert(machine.Id(), tc.Equals, "666")
	c.Assert(machine.Life(), tc.Equals, life.Alive)
}

func (s *provisionerSuite) TestMachinesWithTransientErrors(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	results := params.StatusResults{
		Results: []params.StatusResult{{
			Id:     "666",
			Life:   "alive",
			Status: "error",
			Info:   "provisioning error",
			Data:   map[string]interface{}{"transient": true},
		}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "MachinesWithTransientErrors", nil, results)

	client := provisioner.NewClient(caller)
	result, err := client.MachinesWithTransientErrors(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	machine := result[0].Machine
	c.Assert(machine, tc.FitsTypeOf, &provisioner.Machine{})
	c.Assert(machine.Tag(), tc.Equals, names.NewMachineTag("666"))
	c.Assert(machine.Id(), tc.Equals, "666")
	c.Assert(machine.Life(), tc.Equals, life.Alive)
	c.Assert(result[0].Status, tc.DeepEquals, params.StatusResult{
		Id:     "666",
		Life:   "alive",
		Status: "error",
		Info:   "provisioning error",
		Data:   map[string]interface{}{"transient": true},
	})
}

func (s *provisionerSuite) TestDistributionGroupByMachineId(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.StringsResults{
		Results: []params.StringsResult{{
			Result: []string{"id-1", "id-2"},
		}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "DistributionGroupByMachineId", args, results)

	client := provisioner.NewClient(caller)
	result, err := client.DistributionGroupByMachineId(c.Context(), names.NewMachineTag("666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []provisioner.DistributionGroupResult{{
		MachineIds: []string{"id-1", "id-2"},
	}})
}

func (s *provisionerSuite) TestProvisioningInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{{
			Result: &params.ProvisioningInfo{},
		}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "ProvisioningInfo", args, results)

	client := provisioner.NewClient(caller)
	result, err := client.ProvisioningInfo(c.Context(), []names.MachineTag{names.NewMachineTag("666")})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, results)
}

func (s *provisionerSuite) TestHostChangesForContainer(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.HostNetworkChangeResults{
		Results: []params.HostNetworkChange{{
			NewBridges: []params.DeviceBridgeInfo{{
				HostDeviceName: "host",
				BridgeName:     "bridge",
				MACAddress:     "mac",
			}},
		}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "HostChangesForContainers", args, results)

	client := provisioner.NewClient(caller)
	result, err := client.HostChangesForContainer(c.Context(), names.NewMachineTag("666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []network.DeviceToBridge{{
		DeviceName: "host",
		BridgeName: "bridge",
		MACAddress: "mac",
	}})
}

func (s *provisionerSuite) TestContainerManagerConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ContainerManagerConfigParams{
		Type: "lxd",
	}
	results := params.ContainerManagerConfig{
		ManagerConfig: map[string]string{"foo": "bar"},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "ContainerManagerConfig", args, results)

	client := provisioner.NewClient(caller)
	result, err := client.ContainerManagerConfig(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, results)
}

func (s *provisionerSuite) TestFindTools(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	vers := semversion.MustParse("6.6.6")
	args := params.FindToolsParams{
		Number:       vers,
		MajorVersion: 0,
		Arch:         "arm64",
		OSType:       "ubuntu",
	}
	results := params.FindToolsResult{
		List: tools.List{{
			Version: semversion.MustParseBinary("6.6.6-ubuntu-arm64"),
			URL:     "http://here",
			SHA256:  "deadbeaf",
			Size:    666,
		}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "FindTools", args, results)

	client := provisioner.NewClient(caller)
	result, err := client.FindTools(c.Context(), vers, "ubuntu", "arm64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, tools.List{{
		Version: semversion.MustParseBinary("6.6.6-ubuntu-arm64"),
		URL:     "http://here",
		SHA256:  "deadbeaf",
		Size:    666,
	}})
}

func (s *provisionerSuite) TestContainerConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := params.ContainerConfig{
		ProviderType: "ec2",
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "ContainerConfig", nil, cfg)

	client := provisioner.NewClient(caller)
	result, err := client.ContainerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, cfg)
}

func (s *provisionerSuite) TestWatchModelMachines(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	results := params.StringsWatchResult{
		Error: &params.Error{Message: "FAIL"},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "WatchModelMachines", nil, results)

	client := provisioner.NewClient(caller)
	_, err := client.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) setupMachines(c *tc.C, ctrl *gomock.Controller) (*mocks.MockAPICaller, provisioner.MachineProvisioner) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.LifeResults{
		Results: []params.LifeResult{{Life: "alive"}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "Life", args, results)

	client := provisioner.NewClient(caller)
	result, err := client.Machines(c.Context(), names.NewMachineTag("666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	return caller, result[0].Machine
}

func (s *provisionerSuite) TestSetStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-666",
			Status: "error",
			Info:   "failed",
			Data:   map[string]interface{}{"foo": "bar"},
		}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	s.expectCall(caller, "SetStatus", args, results)

	err := machine.SetStatus(c.Context(), status.Error, "failed", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *provisionerSuite) TestStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.StatusResults{
		Results: []params.StatusResult{{
			Status: "error",
			Info:   "failed",
		}},
	}

	s.expectCall(caller, "Status", args, results)

	st, info, err := machine.Status(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st, tc.Equals, status.Error)
	c.Assert(info, tc.Equals, "failed")
}

func (s *provisionerSuite) TestSetInstanceStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-666",
			Status: "error",
			Info:   "failed",
			Data:   map[string]interface{}{"foo": "bar"},
		}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	s.expectCall(caller, "SetInstanceStatus", args, results)

	err := machine.SetInstanceStatus(c.Context(), status.Error, "failed", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *provisionerSuite) TestInstanceStatus(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.StatusResults{
		Results: []params.StatusResult{{
			Status: "error",
			Info:   "failed",
		}},
	}

	s.expectCall(caller, "InstanceStatus", args, results)

	st, info, err := machine.InstanceStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(st, tc.Equals, status.Error)
	c.Assert(info, tc.Equals, "failed")
}

func (s *provisionerSuite) TestEnsureDead(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	s.expectCall(caller, "EnsureDead", args, results)

	err := machine.EnsureDead(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *provisionerSuite) TestRemove(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	s.expectCall(caller, "Remove", args, results)

	err := machine.Remove(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *provisionerSuite) TestMarkForRemoval(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	s.expectCall(caller, "MarkMachinesForRemoval", args, results)

	err := machine.MarkForRemoval(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *provisionerSuite) TestRefresh(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.LifeResults{
		Results: []params.LifeResult{{Life: "dying"}},
	}
	s.expectCall(caller, "Life", args, results)
	err := machine.Refresh(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machine.Life(), tc.Equals, life.Dying)
}

func (s *provisionerSuite) TestInstanceId(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.StringResults{
		Results: []params.StringResult{{
			Result: "id-666",
		}},
	}

	s.expectCall(caller, "InstanceId", args, results)

	id, err := machine.InstanceId(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id, tc.Equals, instance.Id("id-666"))
}

func (s *provisionerSuite) TestSetInstanceInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	hwChars := instance.MustParseHardware("cores=123", "mem=4G")

	volumes := []params.Volume{{
		VolumeTag: "volume-1-0",
		Info: params.VolumeInfo{
			VolumeId: "vol-123",
			Size:     124,
		},
	}}
	volumeAttachments := map[string]params.VolumeAttachmentInfo{
		"volume-1-0": {
			DeviceName: "xvdf1",
		},
	}

	args := params.InstancesInfo{
		Machines: []params.InstanceInfo{{
			Tag:               "machine-666",
			InstanceId:        "i-will",
			DisplayName:       "my machine",
			Nonce:             "fake_nonce",
			Characteristics:   &hwChars,
			Volumes:           volumes,
			VolumeAttachments: volumeAttachments,
			CharmProfiles:     []string{"profile1"},
		}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	s.expectCall(caller, "SetInstanceInfo", args, results)

	err := machine.SetInstanceInfo(
		c.Context(),
		"i-will", "my machine", "fake_nonce", &hwChars, nil, volumes, volumeAttachments, []string{"profile1"},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *provisionerSuite) TestAvailabilityZone(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.StringResults{
		Results: []params.StringResult{{
			Result: "az-666",
		}},
	}

	s.expectCall(caller, "AvailabilityZone", args, results)

	zone, err := machine.AvailabilityZone(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zone, tc.Equals, "az-666")
}

func (s *provisionerSuite) TestSetCharmProfiles(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.SetProfileArgs{
		Args: []params.SetProfileArg{{
			Entity:   params.Entity{Tag: "machine-666"},
			Profiles: []string{"profile"},
		}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

	s.expectCall(caller, "SetCharmProfiles", args, results)

	err := machine.SetCharmProfiles(c.Context(), []string{"profile"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *provisionerSuite) TestKeepInstance(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.BoolResults{
		Results: []params.BoolResult{{Result: true}},
	}

	s.expectCall(caller, "KeepInstance", args, results)

	result, err := machine.KeepInstance(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.IsTrue)
}

func (s *provisionerSuite) TestDistributionGroup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.DistributionGroupResults{
		Results: []params.DistributionGroupResult{{Result: []instance.Id{"id-1", "id-2"}}},
	}

	s.expectCall(caller, "DistributionGroup", args, results)

	result, err := machine.DistributionGroup(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []instance.Id{"id-1", "id-2"})
}

func (s *provisionerSuite) TestWatchContainers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.WatchContainers{
		Params: []params.WatchContainer{
			{MachineTag: "machine-666", ContainerType: "lxd"},
		},
	}

	results := params.StringsWatchResults{
		Results: []params.StringsWatchResult{{
			Error: &params.Error{Message: "FAIL"},
		}},
	}

	s.expectCall(caller, "WatchContainers", args, results)

	_, err := machine.WatchContainers(c.Context(), instance.LXD)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestWatchContainersUnSupportedContainers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	_, machine := s.setupMachines(c, ctrl)

	_, err := machine.WatchContainers(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, `unsupported container type "foo"`)
}

func (s *provisionerSuite) TestSetSupportedContainers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.MachineContainersParams{
		Params: []params.MachineContainers{{
			MachineTag:     "machine-666",
			ContainerTypes: []instance.ContainerType{"lxd"},
		}},
	}
	results := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	s.expectCall(caller, "SetSupportedContainers", args, results)

	err := machine.SetSupportedContainers(c.Context(), []instance.ContainerType{"lxd"}...)
	c.Assert(err, tc.ErrorIsNil)

}

func (s *provisionerSuite) TestSupportedContainers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller, machine := s.setupMachines(c, ctrl)

	args := params.Entities{
		Entities: []params.Entity{{Tag: "machine-666"}},
	}
	results := params.MachineContainerResults{
		Results: []params.MachineContainerResult{{ContainerTypes: []instance.ContainerType{"lxd"}, Determined: true}},
	}

	s.expectCall(caller, "SupportedContainers", args, results)

	result, determined, err := machine.SupportedContainers(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []instance.ContainerType{"lxd"})
	c.Assert(determined, tc.IsTrue)
}
func TestProvisionerContainerSuite(t *testing.T) {
	tc.Run(t, &provisionerContainerSuite{})
}

type provisionerContainerSuite struct {
	containerTag names.MachineTag
}

func (s *provisionerContainerSuite) SetUpTest(_ *tc.C) {
	s.containerTag = names.NewMachineTag("0/lxd/0")
}

func (s *provisionerContainerSuite) setupCaller(ctrl *gomock.Controller) *mocks.MockAPICaller {
	caller := mocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("Provisioner").Return(666)
	return caller
}

func (s *provisionerContainerSuite) expectCall(caller *mocks.MockAPICaller, method, args, results interface{}) {
	caller.EXPECT().APICall(gomock.Any(), "Provisioner", 666, "", method, args, gomock.Any()).SetArg(6, results).Return(nil)
}

func (s *provisionerContainerSuite) TestPrepareContainerInterfaceInfoNoValues(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: s.containerTag.String()}},
	}
	results := params.MachineNetworkConfigResults{Results: []params.MachineNetworkConfigResult{{
		Config: nil,
		Error:  nil,
	}}}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "PrepareContainerInterfaceInfo", args, results)
	provisionerApi := provisioner.NewClient(caller)

	networkInfo, err := provisionerApi.PrepareContainerInterfaceInfo(c.Context(), s.containerTag)
	c.Assert(err, tc.IsNil)
	c.Check(networkInfo, tc.DeepEquals, corenetwork.InterfaceInfos{})
}

func (s *provisionerContainerSuite) TestPrepareContainerInterfaceInfoSingleNIC(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: s.containerTag.String()}},
	}
	results := params.MachineNetworkConfigResults{
		Results: []params.MachineNetworkConfigResult{{
			Config: []params.NetworkConfig{{
				DeviceIndex:         1,
				MACAddress:          "de:ad:be:ff:11:22",
				MTU:                 9000,
				ProviderId:          "prov-id",
				ProviderSubnetId:    "prov-sub-id",
				ProviderSpaceId:     "prov-space-id",
				ProviderAddressId:   "prov-address-id",
				ProviderVLANId:      "prov-vlan-id",
				VLANTag:             25,
				InterfaceName:       "eth5",
				ParentInterfaceName: "parent#br-eth5",
				InterfaceType:       "ethernet",
				Disabled:            false,
				NoAutoStart:         false,
				ConfigType:          "static",
				Addresses: []params.Address{{
					Value: "192.168.0.6",
					Type:  "ipv4",
					Scope: "local-cloud",
					CIDR:  "192.168.0.5/24",
				}},
				DNSServers:       []string{"8.8.8.8"},
				DNSSearchDomains: []string{"mydomain"},
				GatewayAddress:   "192.168.0.1",
				Routes: []params.NetworkRoute{{
					DestinationCIDR: "10.0.0.0/16",
					GatewayIP:       "192.168.0.1",
					Metric:          55,
				}},
			}},
			Error: nil,
		}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "PrepareContainerInterfaceInfo", args, results)
	provisionerApi := provisioner.NewClient(caller)

	networkInfo, err := provisionerApi.PrepareContainerInterfaceInfo(c.Context(), s.containerTag)
	c.Assert(err, tc.IsNil)
	c.Check(networkInfo, tc.DeepEquals, corenetwork.InterfaceInfos{{
		DeviceIndex:         1,
		MACAddress:          "de:ad:be:ff:11:22",
		MTU:                 9000,
		ProviderId:          "prov-id",
		ProviderSubnetId:    "prov-sub-id",
		ProviderSpaceId:     "prov-space-id",
		ProviderAddressId:   "prov-address-id",
		ProviderVLANId:      "prov-vlan-id",
		VLANTag:             25,
		InterfaceName:       "eth5",
		ParentInterfaceName: "parent#br-eth5",
		InterfaceType:       "ethernet",
		Disabled:            false,
		NoAutoStart:         false,
		ConfigType:          "static",
		Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
			"192.168.0.6", corenetwork.WithCIDR("192.168.0.5/24"), corenetwork.WithConfigType(corenetwork.ConfigStatic),
		).AsProviderAddress()},
		DNSServers:       []string{"8.8.8.8"},
		DNSSearchDomains: []string{"mydomain"},
		GatewayAddress:   corenetwork.NewMachineAddress("192.168.0.1").AsProviderAddress(),
		Routes: []corenetwork.Route{{
			DestinationCIDR: "10.0.0.0/16",
			GatewayIP:       "192.168.0.1",
			Metric:          55,
		}},
	}})
}

func (s *provisionerContainerSuite) TestGetContainerProfileInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: s.containerTag.String()}},
	}
	results := params.ContainerProfileResults{
		Results: []params.ContainerProfileResult{
			{
				LXDProfiles: []*params.ContainerLXDProfile{
					{
						Profile: params.CharmLXDProfile{
							Config: map[string]string{
								"security.nesting":    "true",
								"security.privileged": "true",
							},
						},
						Name: "one",
					},
					{
						Profile: params.CharmLXDProfile{
							Devices: map[string]map[string]string{
								"bdisk": {
									"source": "/dev/loop0",
									"type":   "unix-block",
								},
							},
						},
						Name: "two",
					}},
				Error: nil,
			}},
	}

	caller := s.setupCaller(ctrl)
	s.expectCall(caller, "GetContainerProfileInfo", args, results)
	provisionerApi := provisioner.NewClient(caller)

	obtainedResults, err := provisionerApi.GetContainerProfileInfo(c.Context(), s.containerTag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedResults, tc.DeepEquals, []*provisioner.LXDProfileResult{
		{
			Config: map[string]string{
				"security.nesting":    "true",
				"security.privileged": "true",
			},
			Name: "one",
		},
		{
			Devices: map[string]map[string]string{
				"bdisk": {
					"source": "/dev/loop0",
					"type":   "unix-block",
				},
			},
			Name: "two",
		}})

}
