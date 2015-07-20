// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
)

//
// Imlementation of InstanceBroker: methods for starting and stopping instances.
//

var findInstanceImage = func(env *environ, ic *imagemetadata.ImageConstraint) (*imagemetadata.ImageMetadata, error) {

	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, err
	}

	matchingImages, _, err := imagemetadata.Fetch(sources, ic, false)
	if err != nil {
		return nil, err
	}
	if len(matchingImages) == 0 {
		return nil, errors.New("no matching image meta data")
	}

	return matchingImages[0], nil
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance asks for a new instance to be created, associated with
// the provided config in machineConfig. The given config describes the juju
// state for the new instance to connect to. The config MachineNonce, which must be
// unique within an environment, is used by juju to protect against the
// consequences of multiple instances being started with the same machine id.
func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	logger.Infof("sigmaEnviron.StartInstance...")

	if args.InstanceConfig == nil {
		return nil, errors.New("instance configuration is nil")
	}

	if args.InstanceConfig.HasNetworks() {
		return nil, errors.New("starting instances with networks is not supported yet")
	}

	if len(args.Tools) == 0 {
		return nil, errors.New("tools not found")
	}

	region, _ := env.Region()
	img, err := findInstanceImage(env, imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: region,
		Series:    args.Tools.AllSeries(),
		Arches:    args.Tools.Arches(),
		Stream:    env.Config().ImageStream(),
	}))
	if err != nil {
		return nil, err
	}

	tools, err := args.Tools.Match(tools.Filter{Arch: img.Arch})
	if err != nil {
		return nil, errors.Errorf("chosen architecture %v not present in %v", img.Arch, args.Tools.Arches())
	}

	args.InstanceConfig.Tools = tools[0]
	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.Config()); err != nil {
		return nil, err
	}
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}

	logger.Debugf("cloudsigma user data; %d bytes", len(userData))

	client := env.client
	server, rootdrive, arch, err := client.newInstance(args, img, userData)
	if err != nil {
		return nil, errors.Errorf("failed start instance: %v", err)
	}

	inst := &sigmaInstance{server: server}

	// prepare hardware characteristics
	hwch, err := inst.hardware(arch, rootdrive.Size())
	if err != nil {
		return nil, err
	}

	logger.Debugf("hardware: %v", hwch)
	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwch,
	}, nil
}

// AllInstances returns all instances currently known to the broker.
func (env *environ) AllInstances() ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.

	logger.Tracef("environ.AllInstances...")

	servers, err := env.client.instances()
	if err != nil {
		logger.Tracef("environ.AllInstances failed: %v", err)
		return nil, err
	}

	instances := make([]instance.Instance, 0, len(servers))
	for _, server := range servers {
		instance := sigmaInstance{server: server}
		instances = append(instances, instance)
	}

	if logger.LogLevel() <= loggo.TRACE {
		logger.Tracef("All instances, len = %d:", len(instances))
		for _, instance := range instances {
			logger.Tracef("... id: %q, status: %q", instance.Id(), instance.Status())
		}
	}

	return instances, nil
}

// Instances returns a slice of instances corresponding to the
// given instance ids.  If no instances were found, but there
// was no other error, it will return ErrNoInstances.  If
// some but not all the instances were found, the returned slice
// will have some nil slots, and an ErrPartialInstances error
// will be returned.
func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	logger.Tracef("environ.Instances %#v", ids)
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	// This advice applies even if an instance id passed in corresponds to a
	// real instance that's not part of the environment -- the Environ should
	// treat that no differently to a request for one that does not exist.

	m, err := env.client.instanceMap()
	if err != nil {
		logger.Warningf("environ.Instances failed: %v", err)
		return nil, err
	}

	var found int
	r := make([]instance.Instance, len(ids))
	for i, id := range ids {
		if s, ok := m[string(id)]; ok {
			r[i] = sigmaInstance{server: s}
			found++
		}
	}

	if found == 0 {
		err = environs.ErrNoInstances
	} else if found != len(ids) {
		err = environs.ErrPartialInstances
	}

	return r, err
}

// StopInstances shuts down the given instances.
func (env *environ) StopInstances(instances ...instance.Id) error {
	logger.Debugf("stop instances %+v", instances)

	var err error

	for _, instance := range instances {
		if e := env.client.stopInstance(instance); e != nil {
			err = e
		}
	}

	return err
}

// AllocateAddress requests a new address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instID instance.Id, netID network.Id, addr network.Address, macAddress, hostname string) error {
	return errors.NotSupportedf("AllocateAddress")
}
func (env *environ) ReleaseAddress(instId instance.Id, netId network.Id, addr network.Address, macAddress string) error {
	return errors.NotSupportedf("ReleaseAddress")
}
func (env *environ) Subnets(inst instance.Id) ([]network.SubnetInfo, error) {
	return nil, errors.NotSupportedf("Subnets")
}

// ListNetworks returns basic information about all networks known
// by the provider for the environment. They may be unknown to juju
// yet (i.e. when called initially or when a new network was created).
func (env *environ) ListNetworks() ([]network.SubnetInfo, error) {
	return nil, errors.NotImplementedf("ListNetworks")
}
