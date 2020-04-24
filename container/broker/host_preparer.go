// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

// PrepareAPI is the functional interface that we need to be able to ask what
// changes are necessary, and to then report back what changes have been done
// to the host machine.
type PrepareAPI interface {
	// HostChangesForContainer returns the list of bridges to be created on the
	// host machine, and the time to sleep after creating the bridges before
	// bringing them up.
	HostChangesForContainer(names.MachineTag) ([]network.DeviceToBridge, int, error)
	// SetHostMachineNetworkConfig allows us to report back the host machine's
	// current networking config. This is called after we've created new
	// bridges to inform the Controller what the current networking interfaces
	// are.
	SetHostMachineNetworkConfig(names.MachineTag, []params.NetworkConfig) error
}

// HostPreparerParams is the configuration for HostPreparer
type HostPreparerParams struct {
	API                PrepareAPI
	ObserveNetworkFunc func() ([]params.NetworkConfig, error)
	AcquireLockFunc    func(string, <-chan struct{}) (func(), error)
	CreateBridger      func() (network.Bridger, error)
	AbortChan          <-chan struct{}
	MachineTag         names.MachineTag
	Logger             loggo.Logger
}

// HostPreparer calls out to the PrepareAPI to find out what changes need to be
// done on this host to allow a new container to be started.
type HostPreparer struct {
	api                PrepareAPI
	observeNetworkFunc func() ([]params.NetworkConfig, error)
	acquireLockFunc    func(string, <-chan struct{}) (func(), error)
	createBridger      func() (network.Bridger, error)
	abortChan          <-chan struct{}
	machineTag         names.MachineTag
	logger             loggo.Logger
}

// NewHostPreparer creates a HostPreparer using the supplied parameters
func NewHostPreparer(params HostPreparerParams) *HostPreparer {
	return &HostPreparer{
		api:                params.API,
		observeNetworkFunc: params.ObserveNetworkFunc,
		acquireLockFunc:    params.AcquireLockFunc,
		createBridger:      params.CreateBridger,
		abortChan:          params.AbortChan,
		machineTag:         params.MachineTag,
		logger:             params.Logger,
	}
}

// Prepare applies changes to the host machine that are necessary to create
// the requested container.
func (hp *HostPreparer) Prepare(containerTag names.MachineTag) error {
	devicesToBridge, reconfigureDelay, err := hp.api.HostChangesForContainer(containerTag)
	if err != nil {
		return errors.Annotate(err, "unable to setup network")
	}

	if len(devicesToBridge) == 0 {
		hp.logger.Debugf("container %q requires no additional bridges", containerTag)
		return nil
	}

	bridger, err := hp.createBridger()
	if err != nil {
		return errors.Trace(err)
	}

	hp.logger.Debugf("bridging %+v devices on host %q for container %q with delay=%v",
		devicesToBridge, hp.machineTag.String(), containerTag.String(), reconfigureDelay)
	releaser, err := hp.acquireLockFunc("bridging devices", hp.abortChan)
	if err != nil {
		return errors.Annotatef(err, "failed to acquire machine lock for bridging")
	}
	defer releaser()
	// TODO(jam): 2017-02-15 bridger.Bridge should probably also take AbortChan
	// if it is going to have reconfigureDelay
	err = bridger.Bridge(devicesToBridge, reconfigureDelay)
	if err != nil {
		return errors.Annotate(err, "failed to bridge devices")
	}

	// We just changed the hosts' network setup so discover new
	// interfaces/devices and propagate to state.
	observedConfig, err := hp.observeNetworkFunc()
	if err != nil {
		return errors.Annotate(err, "cannot discover observed network config")
	}

	if len(observedConfig) > 0 {
		hp.logger.Debugf("updating observed network config for %q to %#v", hp.machineTag.String(), observedConfig)
		err := hp.api.SetHostMachineNetworkConfig(hp.machineTag, observedConfig)
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}
