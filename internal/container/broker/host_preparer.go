// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/rpc/params"
)

// PrepareAPI is the functional interface that we need to be able to ask what
// changes are necessary, and to then report back what changes have been done
// to the host machine.
type PrepareAPI interface {
	// HostChangesForContainer returns the list of bridges to be created on the
	// host machine, and the time to sleep after creating the bridges before
	// bringing them up.
	HostChangesForContainer(context.Context, names.MachineTag) ([]network.DeviceToBridge, error)
	// SetHostMachineNetworkConfig allows us to report back the host machine's
	// current networking config. This is called after we've created new
	// bridges to inform the Controller what the current networking interfaces
	// are.
	SetHostMachineNetworkConfig(context.Context, names.MachineTag, []params.NetworkConfig) error
}

// HostPreparerParams is the configuration for HostPreparer
type HostPreparerParams struct {
	API                PrepareAPI
	ObserveNetworkFunc func() ([]params.NetworkConfig, error)
	AcquireLockFunc    func(string, <-chan struct{}) (func(), error)
	Bridger            network.Bridger
	AbortChan          <-chan struct{}
	MachineTag         names.MachineTag
	Logger             corelogger.Logger
}

// HostPreparer calls out to the PrepareAPI to find out what changes need to be
// done on this host to allow a new container to be started.
type HostPreparer struct {
	api                PrepareAPI
	observeNetworkFunc func() ([]params.NetworkConfig, error)
	acquireLockFunc    func(string, <-chan struct{}) (func(), error)
	bridger            network.Bridger
	abortChan          <-chan struct{}
	machineTag         names.MachineTag
	logger             corelogger.Logger
}

// NewHostPreparer creates a HostPreparer using the supplied parameters
func NewHostPreparer(params HostPreparerParams) *HostPreparer {
	return &HostPreparer{
		api:                params.API,
		observeNetworkFunc: params.ObserveNetworkFunc,
		acquireLockFunc:    params.AcquireLockFunc,
		bridger:            params.Bridger,
		abortChan:          params.AbortChan,
		machineTag:         params.MachineTag,
		logger:             params.Logger,
	}
}

// Prepare applies changes to the host machine that are necessary to create
// the requested container.
func (hp *HostPreparer) Prepare(ctx context.Context, containerTag names.MachineTag) error {
	releaser, err := hp.acquireLockFunc("bridging devices", hp.abortChan)
	if err != nil {
		return errors.Annotatef(err, "failed to acquire machine lock for bridging")
	}
	defer releaser()

	devicesToBridge, err := hp.api.HostChangesForContainer(ctx, containerTag)
	if err != nil {
		return errors.Annotate(err, "unable to setup network")
	}

	if len(devicesToBridge) == 0 {
		hp.logger.Debugf(ctx, "container %q requires no additional bridges", containerTag)
		return nil
	}

	hp.logger.Debugf(ctx, "bridging %+v devices on host %q for container %q",
		devicesToBridge, hp.machineTag.String(), containerTag.String())

	err = hp.bridger.Bridge(devicesToBridge)
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
		hp.logger.Debugf(ctx, "updating observed network config for %q to %#v", hp.machineTag.String(), observedConfig)
		err := hp.api.SetHostMachineNetworkConfig(ctx, hp.machineTag, observedConfig)
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}
