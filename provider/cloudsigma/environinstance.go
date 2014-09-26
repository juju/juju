// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/localstorage"
	"github.com/juju/loggo"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
)

//
// Imlementation of InstanceBroker: methods for starting and stopping instances.
//

var findInstanceImage = func(
	env *environ, ic *imagemetadata.ImageConstraint) (*imagemetadata.ImageMetadata, error) {

	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, err
	}


	matchingImages, _, err := imagemetadata.Fetch(sources, ic, false)
	if err != nil {
		return nil, err
	}
	if len(matchingImages) == 0 {
		return nil, fmt.Errorf("no matching image meta data");
	}

	return matchingImages[0], nil;
}


// StartInstance asks for a new instance to be created, associated with
// the provided config in machineConfig. The given config describes the juju
// state for the new instance to connect to. The config MachineNonce, which must be
// unique within an environment, is used by juju to protect against the
// consequences of multiple instances being started with the same machine id.
func (env *environ) StartInstance(args environs.StartInstanceParams) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {
	logger.Infof("sigmaEnviron.StartInstance...")

	if args.MachineConfig == nil {
		return nil, nil, nil, fmt.Errorf("machine configuration is nil")
	}

	if args.MachineConfig.HasNetworks() {
		return nil, nil, nil, fmt.Errorf("starting instances with networks is not supported yet")
	}

	if len(args.Tools) == 0 {
		return nil, nil, nil, fmt.Errorf("tools not found")
	}

	region, _ := env.Region()
	img, err := findInstanceImage(env, imagemetadata.NewImageConstraint(simplestreams.LookupParams{
			CloudSpec: region,
			Series:    args.Tools.AllSeries(),
			Arches:    args.Tools.Arches(),
			Stream:    env.Config().ImageStream(),
		}))
	if err != nil {
		return nil, nil, nil, err
	}

	client := env.client
	server, rootdrive, err := client.newInstance(args, img)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed start instance: %v", err)
	}

	inst := &sigmaInstance{server: server}

	// prepare hardware characteristics
	hwch := inst.hardware(rootdrive.Arch(), rootdrive.Size())

	logger.Tracef("hardware: %v", hwch)

	return inst, hwch, nil, nil
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
		logger.Tracef("environ.Instances failed: %v", err)
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
	logger.Infof("stop instances %+v", instances)

	var err error

	for _, instance := range instances {
		if e := env.client.stopInstance(instance); e != nil {
			err = e
		}
	}

	return err
}

func (env *environ) prepareStorage(addr string, mcfg *cloudinit.MachineConfig) error {
	storagePort := env.ecfg.storagePort()
	storageDir := mcfg.DataDir + "/" + storageSubdir

	logger.Debugf("Moving local temporary storage to %s:%d (%s)...", addr, storagePort, storageDir)
	if err := env.storage.MoveToSSH("ubuntu", addr); err != nil {
		return err
	}

	if strings.Contains(mcfg.Tools.URL, "%s") {
		mcfg.Tools.URL = fmt.Sprintf(mcfg.Tools.URL, "file://"+storageDir)
		logger.Tracef("Tools URL patched to %q", mcfg.Tools.URL)
	}

	// prepare configuration for local storage at bootstrap host
	storageConfig := storageConfig{
		ecfg:        env.ecfg,
		storageDir:  storageDir,
		storageAddr: addr,
		storagePort: storagePort,
	}

	agentEnv, err := localstorage.StoreConfig(&storageConfig)
	if err != nil {
		return err
	}

	for k, v := range agentEnv {
		mcfg.AgentEnvironment[k] = v
	}

	return nil
}

// AllocateAddress requests a new address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instID instance.Id, netID network.Id) (network.Address, error) {
	return network.Address{}, errors.NotSupportedf("AllocateAddress")
}

// ListNetworks returns basic information about all networks known
// by the provider for the environment. They may be unknown to juju
// yet (i.e. when called initially or when a new network was created).
func (env *environ) ListNetworks() ([]network.BasicInfo, error) {
	return nil, errors.NotImplementedf("ListNetworks")
}
