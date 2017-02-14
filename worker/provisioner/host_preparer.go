// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mutex"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

// we could pull this from
// github.com/juju/juju/cloudconfig/instancecfg.DefaultBridgePrefix, but it
// feels bad to import that whole package for a single constant.
const DefaultBridgePrefix = "br-"

type PrepareAPI interface {
	HostChangesForContainer(names.MachineTag) ([]network.DeviceToBridge, int, error)
	SetHostMachineNetworkConfig(names.MachineTag, []params.NetworkConfig) error
}

// HostPreparer calls out to the PrepareAPI to find out what changes need to be
// done on this host to allow a new container to be started.
type HostPreparer struct {
	API                PrepareAPI
	ObserveNetworkFunc func() ([]params.NetworkConfig, error)
	LockName           string
	AcquireLockFunc    func(<-chan struct{}) (mutex.Releaser, error)
	MachineTag names.MachineTag
}

func (hp *HostPreparer) Prepare(containerTag names.MachineTag, log loggo.Logger) error {
	devicesToBridge, reconfigureDelay, err := hp.API.HostChangesForContainer(containerTag)
	if err != nil {
		return errors.Annotate(err, "unable to setup network")
	}

	if len(devicesToBridge) == 0 {
		log.Debugf("container %q requires no additional bridges", containerTag)
		return nil
	}

	bridger, err := network.DefaultEtcNetworkInterfacesBridger(activateBridgesTimeout, DefaultBridgePrefix, systemNetworkInterfacesFile)
	if err != nil {
		return errors.Trace(err)
	}

	log.Debugf("Bridging %+v devices on host %q for container %q with delay=%v, acquiring lock %q",
		devicesToBridge, hp.MachineTag.String(), containerTag.String(), reconfigureDelay, hp.LockName)
	// TODO(jam): 2017-02-08 figure out how to thread catacomb.Dying() into
	// this function, so that we can stop trying to acquire the lock if we are
	// stopping.
	releaser, err := hp.AcquireLockFunc(nil)
	if err != nil {
		return errors.Annotatef(err, "failed to acquire lock %q for bridging", hp.LockName)
	}
	defer log.Debugf("releasing lock %q for bridging machine %q for container %q", hp.LockName, hp.MachineTag.String(), containerTag.String())
	defer releaser.Release()
	err = bridger.Bridge(devicesToBridge, reconfigureDelay)
	if err != nil {
		return errors.Annotate(err, "failed to bridge devices")
	}

	// We just changed the hosts' network setup so discover new
	// interfaces/devices and propagate to state.
	observedConfig, err := hp.ObserveNetworkFunc()
	if err != nil {
		return errors.Annotate(err, "cannot discover observed network config")
	}

	if len(observedConfig) > 0 {
		err := hp.API.SetHostMachineNetworkConfig(hp.MachineTag, observedConfig)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("observed network config updated")
	}

	return nil
}
