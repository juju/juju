// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"
)

var logger = loggo.GetLogger("juju.discoverspaces")

type discoverspacesWorker struct {
	api      *discoverspaces.API
	tomb     tomb.Tomb
	observer *worker.EnvironObserver
}

// NewWorker returns a worker
func NewWorker(api *discoverspaces.API) worker.Worker {
	dw := &discoverspacesWorker{
		api: api,
	}
	go func() {
		defer dw.tomb.Done()
		dw.tomb.Kill(dw.loop())
	}()
	return dw
}

func (dw *discoverspacesWorker) Kill() {
	dw.tomb.Kill(nil)
}

func (dw *discoverspacesWorker) Wait() error {
	return dw.tomb.Wait()
}

func (dw *discoverspacesWorker) loop() (err error) {
	dw.observer, err = worker.NewEnvironObserver(dw.api)
	if err != nil {
		return err
	}
	defer func() {
		obsErr := worker.Stop(dw.observer)
		if err == nil {
			err = obsErr
		}
	}()
	environ := dw.observer.Environ()
	networkingEnviron, ok := environs.SupportsNetworking(environ)

	if ok {
		err = dw.handleSubnets(networkingEnviron)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// TODO(mfoord): we'll have a watcher here checking if we need to
	// update the spaces/subnets definition.
	dying := dw.tomb.Dying()
	for {
		select {
		case <-dying:
			return nil
		}
	}
	return nil
}

func (dw *discoverspacesWorker) handleSubnets(env environs.NetworkingEnviron) error {
	ok, err := env.SupportsSpaceDiscovery()
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		// Nothing to do.
		return nil
	}
	providerSpaces, err := env.Spaces()
	if err != nil {
		return errors.Trace(err)
	}
	listSpacesResult, err := dw.api.ListSpaces()
	if err != nil {
		return errors.Trace(err)
	}
	stateSpaceMap := make(map[string]params.ProviderSpace)
	for _, space := range listSpacesResult.Results {
		stateSpaceMap[space.ProviderId] = space
	}

	// TODO(mfoord): we also need to attempt to delete spaces and subnets
	// that no longer exist, so long as they're not in use.
	for _, space := range providerSpaces {
		_, ok := stateSpaceMap[space.Name]
		if !ok {
			// We need to create the space.
			// XXX in the apiserver the name should be generated and
			// IsPublic set to false.
			args := params.CreateSpacesParams{}
			_, err := dw.api.CreateSpaces(args)
			if err != nil {
				return errors.Trace(err)
			}
		}
		// TODO(mfoord): currently no way of removing subnets, or
		// changing the space they're in, so we can only add ones we
		// don't already know about.
		for _, subnet := range space.Subnets {
			spaceTag, err := names.ParseSpaceTag(space.Name)
			if err != nil {
				return errors.Trace(err)
			}
			subnetTag, err := names.ParseSubnetTag(subnet.CIDR)
			if err != nil {
				return errors.Trace(err)
			}

			args := params.AddSubnetsParams{
				Subnets: []params.AddSubnetParams{{
					SubnetTag:        subnetTag.String(),
					SubnetProviderId: string(subnet.ProviderId),
					SpaceTag:         spaceTag.String(),
					Zones:            subnet.AvailabilityZones,
				}}}
			_, err = dw.api.AddSubnets(args)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}
