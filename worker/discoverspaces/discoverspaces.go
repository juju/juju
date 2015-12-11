// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.discoverspaces")

const networkingFacade = "Networking"

// API provides access to the API facade.
type API struct {
	*common.EnvironWatcher
	facade base.FacadeCaller
}

// NewAPI creates a new facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	facadeCaller := base.NewFacadeCaller(caller, networkingFacade)
	return &API{
		EnvironWatcher: common.NewEnvironWatcher(facadeCaller),
		facade:         facadeCaller,
	}
}

type discoverspacesWorker struct {
	api      *API
	tomb     tomb.Tomb
	observer *worker.EnvironObserver
}

// NewWorker returns a worker
func NewWorker(api *API) worker.Worker {
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
	for {
	}
	return err
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
	stateSpaceMap := make(map[string]params.Space)
	for _, space := range listSpacesResult.Results {
		stateSpaceMap[space.ProviderId] = space
	}

	// TODO(mfoord): we also need to attempt to delete spaces that no
	// longer exist, so long as they're not in use.
	for _, space := range providerSpaces {
		_, ok := stateSpaceMap[space.Name]
		if !ok {
			// We need to create the space.
			// XXX in the apiserver the name should be generated and
			// IsPublic set to false.
			err = dw.api.AddSpace(space.ProviderId)
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

			err = dw.api.AddSubnet(subnetTag, subnet.ProviderId, spaceTag, subnet.AvailabilityZones)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}
