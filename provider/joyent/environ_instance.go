// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"launchpad.net/gojoyent/client"
	"launchpad.net/gojoyent/cloudapi"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/tools"
)

var (
	vTypeSmartmachine   = "smartmachine"
	vTypeVirtualmachine = "kvm"
	signedImageDataOnly = false
)

type joyentCompute struct {
	sync.Mutex
	ecfg     *environConfig
	cloudapi *cloudapi.Client
}

func NewCompute(env *JoyentEnviron) *joyentCompute {
	if compute, err := newCompute(env); err == nil {
		return compute
	}
	return nil
}

func newCompute(env *JoyentEnviron) (*joyentCompute, error) {
	client := client.NewClient(env.ecfg.sdcUrl(), cloudapi.DefaultAPIVersion, env.creds, &logger)

	return &joyentCompute{
		ecfg:     env.ecfg,
		cloudapi: cloudapi.New(client)}, nil
}

func (env *JoyentEnviron) machineFullName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", env.Name(), names.MachineTag(machineId))
}

func (env *JoyentEnviron) StartInstance(cons constraints.Value, possibleTools tools.List,
	machineConf *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {

	arches := possibleTools.Arches()

	series := possibleTools.OneSeries()
	spec, err := env.FindInstanceSpec(&instances.InstanceConstraint{
		Region:      env.Ecfg().Region(),
		Series:      series,
		Arches:      arches,
		Constraints: cons,
	})
	if err != nil {
		return nil, nil, err
	}
	tools, err := possibleTools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, nil, fmt.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	machineConf.Tools = tools[0]
	if err := environs.FinishMachineConfig(machineConf, env.Config(), cons); err != nil {
		return nil, nil, err
	}

	userData, err := environs.ComposeUserData(machineConf)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot make user data: %v", err)
	}
	logger.Debugf("joyent user data; %d bytes", len(userData))

	var machine *cloudapi.Machine
	machine, err = env.compute.cloudapi.CreateMachine(cloudapi.CreateMachineOpts{
		//Name:	 env.machineFullName(machineConf.MachineId),
		Package: spec.InstanceType.Name,
		Image:   spec.Image.Id,
		Tags:    map[string]string{"tag.group": "juju", "tag.env": env.Name()},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("cannot create instances: %v", err)
	}
	machineId := machine.Id

	logger.Infof("provisioning instance %q", machineId)

	machine, err = env.compute.cloudapi.GetMachine(machineId)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot start instances: %v", err)
	}

	// wait for machine to start
	for !strings.EqualFold(machine.State, "running") {
		time.Sleep(1 * time.Second)

		machine, err = env.compute.cloudapi.GetMachine(machineId)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot start instances: %v", err)
		}
	}

	logger.Infof("started instance %q", machineId)

	inst := &joyentInstance{
		machine: machine,
		env:     env,
	}

	disk64 := uint64(machine.Disk)
	hc := instance.HardwareCharacteristics{
		Arch:     &spec.Image.Arch,
		Mem:      &spec.InstanceType.Mem,
		CpuCores: &spec.InstanceType.CpuCores,
		CpuPower: spec.InstanceType.CpuPower,
		RootDisk: &disk64,
	}

	return inst, &hc, nil
}

func (env *JoyentEnviron) AllInstances() ([]instance.Instance, error) {
	instances := []instance.Instance{}

	filter := cloudapi.NewFilter()
	filter.Set("tag.group", "juju")
	filter.Set("tag.env", env.Name())

	machines, err := env.compute.cloudapi.ListMachines(filter)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve instances: %v", err)
	}

	for _, m := range machines {
		if strings.EqualFold(m.State, "provisioning") || strings.EqualFold(m.State, "running") {
			copy := m
			instances = append(instances, &joyentInstance{machine: &copy, env: env})
		}
	}

	return instances, nil
}

func (env *JoyentEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	logger.Debugf("Looking for instances %q", ids)

	instances := make([]instance.Instance, len(ids))
	found := 0

	allInstances, err := env.AllInstances()
	if err != nil {
		return nil, err
	}

	for i, id := range ids {
		for _, instance := range allInstances {
			if instance.Id() == id {
				instances[i] = instance
				found++
			}
		}
	}

	logger.Debugf("Found %d instances %q", found, instances)

	if found == 0 {
		return nil, environs.ErrNoInstances
	} else if found < len(ids) {
		return instances, environs.ErrPartialInstances
	}

	return instances, nil
}

func (env *JoyentEnviron) StopInstances(instances []instance.Instance) error {
	// Remove all the instances in parallel so that we incur less round-trips.
	var wg sync.WaitGroup
	//var err error
	wg.Add(len(instances))
	errc := make(chan error, len(instances))
	for _, inst := range instances {
		inst := inst.(*joyentInstance)
		go func() {
			defer wg.Done()
			if err := inst.Stop(); err != nil {
				errc <- err
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return fmt.Errorf("cannot stop all instances: %v", err)
	default:
	}

	return nil
}

// findInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func (env *JoyentEnviron) FindInstanceSpec(ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	packages, err := env.compute.cloudapi.ListPackages(nil)
	if err != nil {
		return nil, err
	}
	allInstanceTypes := []instances.InstanceType{}
	for _, pkg := range packages {
		instanceType := instances.InstanceType{
			Id:       pkg.Id,
			Name:     pkg.Name,
			Arches:   ic.Arches,
			Mem:      uint64(pkg.Memory),
			CpuCores: uint64(pkg.VCPUs),
			RootDisk: uint64(pkg.Disk * 1024),
			VType:    &vTypeVirtualmachine,
		}
		allInstanceTypes = append(allInstanceTypes, instanceType)
	}

	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{ic.Region, env.Ecfg().SdcUrl()},
		Series:    []string{ic.Series},
		Arches:    ic.Arches,
	})
	sources, err := imagemetadata.GetMetadataSources(env)
	if err != nil {
		return nil, err
	}

	matchingImages, _, err := imagemetadata.Fetch(sources, simplestreams.DefaultIndexPath, imageConstraint, signedImageDataOnly)
	if err != nil {
		return nil, err
	}
	images := instances.ImageMetadataToImages(matchingImages)
	spec, err := instances.FindInstanceSpec(images, ic, allInstanceTypes)
	if err != nil {
		return nil, err
	}
	return spec, nil
}
