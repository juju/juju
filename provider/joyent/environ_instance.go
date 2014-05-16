// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/joyent/gocommon/client"
	"github.com/joyent/gosdc/cloudapi"
	"github.com/juju/errors"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/arch"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
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

func newCompute(cfg *environConfig) (*joyentCompute, error) {
	creds, err := credentials(cfg)
	if err != nil {
		return nil, err
	}
	client := client.NewClient(cfg.sdcUrl(), cloudapi.DefaultAPIVersion, creds, &logger)

	return &joyentCompute{
		ecfg:     cfg,
		cloudapi: cloudapi.New(client)}, nil
}

func (env *joyentEnviron) machineFullName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", env.Name(), names.MachineTag(machineId))
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.Tags,
}

// ConstraintsValidator is defined on the Environs interface.
func (env *joyentEnviron) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, err
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)
	packages, err := env.compute.cloudapi.ListPackages(nil)
	if err != nil {
		return nil, err
	}
	instTypeNames := make([]string, len(packages))
	for i, pkg := range packages {
		instTypeNames[i] = pkg.Name
	}
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)
	return validator, nil
}

func (env *joyentEnviron) StartInstance(args environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {

	if args.MachineConfig.HasNetworks() {
		return nil, nil, nil, fmt.Errorf("starting instances with networks is not supported yet.")
	}

	series := args.Tools.OneSeries()
	arches := args.Tools.Arches()
	spec, err := env.FindInstanceSpec(&instances.InstanceConstraint{
		Region:      env.Ecfg().Region(),
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	tools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	args.MachineConfig.Tools = tools[0]

	if err := environs.FinishMachineConfig(args.MachineConfig, env.Config(), args.Constraints); err != nil {
		return nil, nil, nil, err
	}
	userData, err := environs.ComposeUserData(args.MachineConfig, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cannot make user data: %v", err)
	}

	// Unzipping as Joyent API expects it as string
	userData, err = utils.Gunzip(userData)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cannot make user data: %v", err)
	}
	logger.Debugf("joyent user data: %d bytes", len(userData))

	var machine *cloudapi.Machine
	machine, err = env.compute.cloudapi.CreateMachine(cloudapi.CreateMachineOpts{
		//Name:	 env.machineFullName(machineConf.MachineId),
		Package:  spec.InstanceType.Name,
		Image:    spec.Image.Id,
		Metadata: map[string]string{"metadata.cloud-init:user-data": string(userData)},
		Tags:     map[string]string{"tag.group": "juju", "tag.env": env.Name()},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cannot create instances: %v", err)
	}
	machineId := machine.Id

	logger.Infof("provisioning instance %q", machineId)

	machine, err = env.compute.cloudapi.GetMachine(machineId)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cannot start instances: %v", err)
	}

	// wait for machine to start
	for !strings.EqualFold(machine.State, "running") {
		time.Sleep(1 * time.Second)

		machine, err = env.compute.cloudapi.GetMachine(machineId)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("cannot start instances: %v", err)
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

	return inst, &hc, nil, nil
}

func (env *joyentEnviron) AllInstances() ([]instance.Instance, error) {
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

func (env *joyentEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
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

// AllocateAddress requests a new address to be allocated for the
// given instance on the given network. This is not implemented on the
// Joyent provider yet.
func (*joyentEnviron) AllocateAddress(_ instance.Id, _ network.Id) (instance.Address, error) {
	return instance.Address{}, errors.NotImplementedf("AllocateAddress")
}

func (env *joyentEnviron) StopInstances(ids ...instance.Id) error {
	// Remove all the instances in parallel so that we incur less round-trips.
	var wg sync.WaitGroup
	//var err error
	wg.Add(len(ids))
	errc := make(chan error, len(ids))
	for _, id := range ids {
		id := id // copy to new free var for closure
		go func() {
			defer wg.Done()
			if err := env.stopInstance(string(id)); err != nil {
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

func (env *joyentEnviron) stopInstance(id string) error {
	// wait for machine to be running
	// if machine is still provisioning stop will fail
	for !env.pollMachineState(id, "running") {
		time.Sleep(1 * time.Second)
	}

	err := env.compute.cloudapi.StopMachine(id)
	if err != nil {
		return fmt.Errorf("cannot stop instance %s: %v", id, err)
	}

	// wait for machine to be stopped
	for !env.pollMachineState(id, "stopped") {
		time.Sleep(1 * time.Second)
	}

	err = env.compute.cloudapi.DeleteMachine(id)
	if err != nil {
		return fmt.Errorf("cannot delete instance %s: %v", id, err)
	}

	return nil
}

func (env *joyentEnviron) pollMachineState(machineId, state string) bool {
	machineConfig, err := env.compute.cloudapi.GetMachine(machineId)
	if err != nil {
		return false
	}
	return strings.EqualFold(machineConfig.State, state)
}

func (env *joyentEnviron) listInstanceTypes() ([]instances.InstanceType, error) {
	packages, err := env.compute.cloudapi.ListPackages(nil)
	if err != nil {
		return nil, err
	}
	allInstanceTypes := []instances.InstanceType{}
	for _, pkg := range packages {
		instanceType := instances.InstanceType{
			Id:       pkg.Id,
			Name:     pkg.Name,
			Arches:   []string{arch.AMD64},
			Mem:      uint64(pkg.Memory),
			CpuCores: uint64(pkg.VCPUs),
			RootDisk: uint64(pkg.Disk * 1024),
			VirtType: &vTypeVirtualmachine,
		}
		allInstanceTypes = append(allInstanceTypes, instanceType)
	}
	return allInstanceTypes, nil
}

// FindInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func (env *joyentEnviron) FindInstanceSpec(ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	allInstanceTypes, err := env.listInstanceTypes()
	if err != nil {
		return nil, err
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
