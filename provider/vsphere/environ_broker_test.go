// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"fmt"
	"io/ioutil"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"github.com/vmware/govmomi/vim25/mo"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/provider/vsphere/internal/ovatest"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

type environBrokerSuite struct {
	EnvironFixture
	statusCallbackStub testing.Stub
}

var _ = gc.Suite(&environBrokerSuite{})

func (s *environBrokerSuite) SetUpTest(c *gc.C) {
	s.EnvironFixture.SetUpTest(c)
	s.statusCallbackStub.ResetCalls()

	s.client.computeResources = []*mo.ComputeResource{
		newComputeResource("z1"),
		newComputeResource("z2"),
	}

	s.client.createdVirtualMachine = buildVM("new-vm").vm()
}

func (s *environBrokerSuite) createStartInstanceArgs(c *gc.C) environs.StartInstanceParams {
	var cons constraints.Value
	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(
		coretesting.FakeControllerConfig(), cons, cons, "trusty", "",
	)
	c.Assert(err, jc.ErrorIsNil)

	setInstanceConfigAuthorizedKeys(c, instanceConfig)
	tools := setInstanceConfigTools(c, instanceConfig)

	return environs.StartInstanceParams{
		AvailabilityZone: "z1",
		ControllerUUID:   instanceConfig.Controller.Config.ControllerUUID(),
		InstanceConfig:   instanceConfig,
		Tools:            tools,
		Constraints:      cons,
		StatusCallback: func(status status.Status, info string, data map[string]interface{}) error {
			s.statusCallbackStub.AddCall("StatusCallback", status, info, data)
			return s.statusCallbackStub.NextErr()
		},
	}
}

func setInstanceConfigTools(c *gc.C, instanceConfig *instancecfg.InstanceConfig) coretools.List {
	tools := []*coretools.Tools{{
		Version: version.Binary{
			Number: version.MustParse("1.2.3"),
			Arch:   arch.AMD64,
			Series: "trusty",
		},
		URL: "https://example.org",
	}}
	err := instanceConfig.SetTools(tools[:1])
	c.Assert(err, jc.ErrorIsNil)
	return tools
}

func setInstanceConfigAuthorizedKeys(c *gc.C, instanceConfig *instancecfg.InstanceConfig) {
	config := fakeConfig(c)
	instanceConfig.AuthorizedKeys = config.AuthorizedKeys()
}

func (s *environBrokerSuite) TestStartInstance(c *gc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	startInstArgs.InstanceConfig.Tags = map[string]string{
		"k0": "v0",
		"k1": "v1",
	}

	result, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	c.Assert(result.Instance, gc.NotNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("new-vm"))

	s.client.CheckCallNames(c, "ComputeResources", "CreateVirtualMachine", "Close")
	call := s.client.Calls()[1]
	c.Assert(call.Args, gc.HasLen, 2)
	c.Assert(call.Args[0], gc.Implements, new(context.Context))
	c.Assert(call.Args[1], gc.FitsTypeOf, vsphereclient.CreateVirtualMachineParams{})

	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.UserData, gc.Not(gc.Equals), "")
	c.Assert(createVMArgs.ReadOVA, gc.NotNil)
	readOVA := createVMArgs.ReadOVA
	createVMArgs.UserData = ""
	createVMArgs.Constraints = constraints.Value{}
	createVMArgs.UpdateProgress = nil
	createVMArgs.Clock = nil
	createVMArgs.ReadOVA = nil
	createVMArgs.NetworkDevices = []vsphereclient.NetworkDevice{}
	c.Assert(createVMArgs, jc.DeepEquals, vsphereclient.CreateVirtualMachineParams{
		Name:                   "juju-f75cba-0",
		Folder:                 `Juju Controller (deadbeef-1bad-500d-9000-4b1d0d06f00d)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
		VMDKDirectory:          "juju-vmdks/deadbeef-1bad-500d-9000-4b1d0d06f00d",
		Series:                 startInstArgs.Tools.OneSeries(),
		OVASHA256:              ovatest.FakeOVASHA256(),
		Metadata:               startInstArgs.InstanceConfig.Tags,
		ComputeResource:        s.client.computeResources[0],
		UpdateProgressInterval: 5 * time.Second,
	})

	ovaLocation, ovaReadCloser, err := readOVA()
	c.Assert(err, jc.ErrorIsNil)
	defer ovaReadCloser.Close()
	c.Assert(
		ovaLocation, gc.Equals,
		s.imageServer.URL+"/server/releases/trusty/release-20150305/ubuntu-14.04-server-cloudimg-amd64.ova",
	)
	ovaBody, err := ioutil.ReadAll(ovaReadCloser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ovaBody, jc.DeepEquals, ovatest.FakeOVAContents())
}

func (s *environBrokerSuite) TestStartInstanceNetwork(c *gc.C) {
	env, err := s.provider.Open(environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"primary-network":    "foo",
			"external-network":   "bar",
			"image-metadata-url": s.imageServer.URL,
		}),
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := env.StartInstance(s.createStartInstanceArgs(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)

	call := s.client.Calls()[1]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.NetworkDevices, gc.HasLen, 2)
	c.Assert(createVMArgs.NetworkDevices[0].Network, gc.Equals, "foo")
	c.Assert(createVMArgs.NetworkDevices[1].Network, gc.Equals, "bar")
}

func (s *environBrokerSuite) TestStartInstanceLongModelName(c *gc.C) {
	env, err := s.provider.Open(environs.OpenParams{
		Cloud: fakeCloudSpec(),
		Config: fakeConfig(c, coretesting.Attrs{
			"name":               "supercalifragilisticexpialidocious",
			"image-metadata-url": s.imageServer.URL,
		}),
	})
	c.Assert(err, jc.ErrorIsNil)
	startInstArgs := s.createStartInstanceArgs(c)
	_, err = env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	call := s.client.Calls()[1]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	// The model name in the folder name should be truncated
	// so that the final part of the model name is 80 characters.
	c.Assert(path.Base(createVMArgs.Folder), gc.HasLen, 80)
	c.Assert(createVMArgs.Folder, gc.Equals,
		`Juju Controller (deadbeef-1bad-500d-9000-4b1d0d06f00d)/Model "supercalifragilisticexpialidociou" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)`,
	)
}

func (s *environBrokerSuite) TestStartInstanceWithUnsupportedConstraints(c *gc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	startInstArgs.Tools[0].Version.Arch = "someArch"
	_, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, gc.ErrorMatches, "no matching images found for given constraints: .*")
	c.Assert(err, jc.Satisfies, environs.IsAvailabilityZoneIndependent)
}

// if tools for multiple architectures are avaliable, provider should filter tools by arch of the selected image
func (s *environBrokerSuite) TestStartInstanceFilterToolByArch(c *gc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	tools := []*coretools.Tools{{
		Version: version.Binary{Arch: arch.I386, Series: "trusty"},
		URL:     "https://example.org",
	}, {
		Version: version.Binary{Arch: arch.AMD64, Series: "trusty"},
		URL:     "https://example.org",
	}}

	// Setting tools to I386, but provider should update them to AMD64,
	// because our fake simplestream server returns only an AMD64 image.
	startInstArgs.Tools = tools
	err := startInstArgs.InstanceConfig.SetTools(coretools.List{
		tools[0],
	})
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.Arch, gc.Equals, arch.AMD64)
	c.Assert(startInstArgs.InstanceConfig.AgentVersion().Arch, gc.Equals, arch.AMD64)
}

func (s *environBrokerSuite) TestStartInstanceDefaultConstraintsApplied(c *gc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	res, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	var (
		arch     = "amd64"
		rootDisk = common.MinRootDiskSizeGiB("trusty") * 1024
	)
	c.Assert(res.Hardware, jc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:     &arch,
		RootDisk: &rootDisk,
	})
}

func (s *environBrokerSuite) TestStartInstanceCustomConstraintsApplied(c *gc.C) {
	var (
		cpuCores uint64 = 4
		cpuPower uint64 = 2001
		mem      uint64 = 2002
		rootDisk uint64 = 10003
	)
	startInstArgs := s.createStartInstanceArgs(c)
	startInstArgs.Constraints.CpuCores = &cpuCores
	startInstArgs.Constraints.CpuPower = &cpuPower
	startInstArgs.Constraints.Mem = &mem
	startInstArgs.Constraints.RootDisk = &rootDisk

	res, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	arch := "amd64"
	c.Assert(res.Hardware, jc.DeepEquals, &instance.HardwareCharacteristics{
		Arch:     &arch,
		CpuCores: &cpuCores,
		CpuPower: &cpuPower,
		Mem:      &mem,
		RootDisk: &rootDisk,
	})
}

func (s *environBrokerSuite) TestStartInstanceCallsFinishMachineConfig(c *gc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	s.PatchValue(&vsphere.FinishInstanceConfig, func(mcfg *instancecfg.InstanceConfig, cfg *config.Config) (err error) {
		return errors.New("FinishMachineConfig called")
	})
	_, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, gc.ErrorMatches, "FinishMachineConfig called")
}

func (s *environBrokerSuite) TestStartInstanceDefaultDiskSizeIsUsedForSmallConstraintValue(c *gc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	rootDisk := uint64(1000)
	startInstArgs.Constraints.RootDisk = &rootDisk
	res, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*res.Hardware.RootDisk, gc.Equals, common.MinRootDiskSizeGiB("trusty")*uint64(1024))
}

func (s *environBrokerSuite) TestStartInstanceSelectZone(c *gc.C) {
	startInstArgs := s.createStartInstanceArgs(c)
	startInstArgs.AvailabilityZone = "z2"
	_, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	s.client.CheckCallNames(c, "ComputeResources", "CreateVirtualMachine", "Close")
	call := s.client.Calls()[1]
	c.Assert(call.Args, gc.HasLen, 2)
	c.Assert(call.Args[0], gc.Implements, new(context.Context))
	c.Assert(call.Args[1], gc.FitsTypeOf, vsphereclient.CreateVirtualMachineParams{})

	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.ComputeResource, jc.DeepEquals, s.client.computeResources[1])
}

func (s *environBrokerSuite) TestStartInstanceFailsWithAvailabilityZone(c *gc.C) {
	s.client.SetErrors(nil, errors.New("nope"))
	startInstArgs := s.createStartInstanceArgs(c)
	_, err := s.env.StartInstance(startInstArgs)
	c.Assert(err, gc.Not(jc.Satisfies), environs.IsAvailabilityZoneIndependent)

	s.client.CheckCallNames(c, "ComputeResources", "CreateVirtualMachine", "Close")
	createVMCall1 := s.client.Calls()[1]
	createVMArgs1 := createVMCall1.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs1.ComputeResource, jc.DeepEquals, s.client.computeResources[0])
}

func (s *environBrokerSuite) TestStartInstanceDatastore(c *gc.C) {
	cfg := s.env.Config()
	cfg, err := cfg.Apply(map[string]interface{}{
		"datastore": "datastore0",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.env.StartInstance(s.createStartInstanceArgs(c))
	c.Assert(err, jc.ErrorIsNil)

	call := s.client.Calls()[1]
	createVMArgs := call.Args[1].(vsphereclient.CreateVirtualMachineParams)
	c.Assert(createVMArgs.Datastore, gc.Equals, "datastore0")
}

func (s *environBrokerSuite) TestStopInstances(c *gc.C) {
	err := s.env.StopInstances("vm-0", "vm-1")
	c.Assert(err, jc.ErrorIsNil)

	var paths []string
	s.client.CheckCallNames(c, "RemoveVirtualMachines", "RemoveVirtualMachines", "Close")
	for i := 0; i < 2; i++ {
		args := s.client.Calls()[i].Args
		paths = append(paths, args[1].(string))
	}

	// NOTE(axw) we must use SameContents, not DeepEquals, because
	// we run the RemoveVirtualMachines calls concurrently.
	c.Assert(paths, jc.SameContents, []string{
		`Juju Controller (*)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)/vm-0`,
		`Juju Controller (*)/Model "testenv" (2d02eeac-9dbb-11e4-89d3-123b93f75cba)/vm-1`,
	})
}

func (s *environBrokerSuite) TestStopInstancesOneFailure(c *gc.C) {
	s.client.SetErrors(errors.New("bah"))
	err := s.env.StopInstances("vm-0", "vm-1")

	s.client.CheckCallNames(c, "RemoveVirtualMachines", "RemoveVirtualMachines", "Close")
	vmName := path.Base(s.client.Calls()[0].Args[1].(string))
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("failed to stop instance %s: bah", vmName))
}

func (s *environBrokerSuite) TestStopInstancesMultipleFailures(c *gc.C) {
	err1 := errors.New("bah")
	err2 := errors.New("bleh")
	s.client.SetErrors(err1, err2)
	err := s.env.StopInstances("vm-0", "vm-1")

	s.client.CheckCallNames(c, "RemoveVirtualMachines", "RemoveVirtualMachines", "Close")
	vmName1 := path.Base(s.client.Calls()[0].Args[1].(string))
	if vmName1 == "vm-1" {
		err1, err2 = err2, err1
	}
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(
		`failed to stop instances \[vm-0 vm-1\]: \[%s %s\]`,
		err1, err2,
	))
}
