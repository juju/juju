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

// DefaultBridgePrefix is the standard prefix we apply to a device to find a
// name for the associated bridge. (eg when bridging ens3 we create br-ens3)
const DefaultBridgePrefix = "br-"

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
	LockName           string
	AcquireLockFunc    func(<-chan struct{}) (mutex.Releaser, error)
	CreateBridger      func() (network.Bridger, error)
	AbortChan          <-chan struct{}
	MachineTag         names.MachineTag
	Logger			   loggo.Logger
}

// HostPreparer calls out to the PrepareAPI to find out what changes need to be
// done on this host to allow a new container to be started.
type HostPreparer struct {
	api                PrepareAPI
	observeNetworkFunc func() ([]params.NetworkConfig, error)
	lockName           string
	acquireLockFunc    func(<-chan struct{}) (mutex.Releaser, error)
	createBridger      func() (network.Bridger, error)
	abortChan          <-chan struct{}
	machineTag         names.MachineTag
	logger			   loggo.Logger
}

// NewHostPreparer creates a HostPreparer using the supplied parameters
func NewHostPreparer(params HostPreparerParams) *HostPreparer {
	return &HostPreparer{
		api:                params.API,
		observeNetworkFunc: params.ObserveNetworkFunc,
		lockName:           params.LockName,
		acquireLockFunc:    params.AcquireLockFunc,
		createBridger:      params.CreateBridger,
		abortChan:          params.AbortChan,
		machineTag:         params.MachineTag,
		logger:			    params.Logger,
	}
}

// DefaultBridgeCreator returns a function that will create a bridger with all the default settings.
func DefaultBridgeCreator() func() (network.Bridger, error) {
	return func() (network.Bridger, error) {
		return network.DefaultEtcNetworkInterfacesBridger(activateBridgesTimeout, DefaultBridgePrefix, systemNetworkInterfacesFile)
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

	hp.logger.Debugf("Bridging %+v devices on host %q for container %q with delay=%v, acquiring lock %q",
		devicesToBridge, hp.machineTag.String(), containerTag.String(), reconfigureDelay, hp.lockName)
	releaser, err := hp.acquireLockFunc(hp.abortChan)
	if err != nil {
		return errors.Annotatef(err, "failed to acquire lock %q for bridging", hp.lockName)
	}
	defer hp.logger.Debugf("releasing lock %q for bridging machine %q for container %q", hp.lockName, hp.machineTag.String(), containerTag.String())
	defer releaser.Release()
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
		err := hp.api.SetHostMachineNetworkConfig(hp.machineTag, observedConfig)
		if err != nil {
			return errors.Trace(err)
		}
		hp.logger.Debugf("observed network config updated")
	}

	return nil
}
