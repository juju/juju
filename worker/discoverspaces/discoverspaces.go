// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	"launchpad.net/tomb"
)

var logger = loggo.GetLogger("juju.discoverspaces")

type discoverspacesWorker struct {
	api  *discoverspaces.API
	tomb tomb.Tomb
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
	envCfg, err := dw.api.EnvironConfig()
	if err != nil {
		return err
	}
	environ, err := environs.New(envCfg)
	if err != nil {
		return err
	}
	networkingEnviron, ok := environs.SupportsNetworking(environ)

	if ok {
		// TODO: (mfoord) API should be switched off until this is
		// completed.
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

	stateSubnets, err := dw.api.ListSubnets(params.SubnetsFilters{})
	if err != nil {
		return errors.Trace(err)
	}
	stateSubnetIds := make(set.Strings)
	for _, subnet := range stateSubnets.Results {
		stateSubnetIds.Add(subnet.ProviderId)
	}
	stateSpaceMap := make(map[string]params.ProviderSpace)
	spaceNames := make(set.Strings)
	for _, space := range listSpacesResult.Results {
		stateSpaceMap[space.ProviderId] = space
		spaceNames.Add(space.Name)
	}

	// TODO(mfoord): we need to delete spaces and subnets that no longer
	// exist, so long as they're not in use.
	for _, space := range providerSpaces {
		spaceName := string(space.ProviderId)
		spaceName = strings.Replace(spaceName, " ", "-", -1)
		spaceName = strings.ToLower(spaceName)
		if !names.IsValidSpace(spaceName) {
			// XXX generate a valid name here
			logger.Errorf("invalid space name %v", spaceName)
			return errors.Errorf("invalid space name: %q", spaceName)
		}
		spaceTag := names.NewSpaceTag(spaceName)
		_, ok := stateSpaceMap[string(space.ProviderId)]
		if !ok {
			// XXX skip spaces with no subnets(?)
			// We need to create the space.
			args := params.CreateSpacesParams{
				Spaces: []params.CreateSpaceParams{{
					Public:   false,
					SpaceTag: spaceTag.String(),
				}}}
			// XXX check the error result too.
			_, err = dw.api.CreateSpaces(args)
			if err != nil {
				logger.Errorf("invalid creating space %v", err)
				return errors.Trace(err)
			}
		}
		// TODO(mfoord): currently no way of removing subnets, or
		// changing the space they're in, so we can only add ones we
		// don't already know about.
		logger.Debugf("Created space %v with %v subnets", spaceName, len(space.Subnets))
		for _, subnet := range space.Subnets {
			if stateSubnetIds.Contains(string(subnet.ProviderId)) {
				continue
			}
			zones := subnet.AvailabilityZones
			if len(zones) == 0 {
				zones = []string{"default"}
			}
			args := params.AddSubnetsParams{
				Subnets: []params.AddSubnetParams{{
					SubnetProviderId: string(subnet.ProviderId),
					SpaceTag:         spaceTag.String(),
					Zones:            zones,
				}}}
			// XXX check the error result too.
			logger.Tracef("Adding subnet %v", subnet.CIDR)
			result, err := dw.api.AddSubnets(args)
			if err != nil {
				logger.Errorf("invalid creating subnet %v", err)
				return errors.Trace(err)
			}

			// XXX needs doing properly (check len(result.Results
			// == 1).
			if result.Results[0].Error != nil {
				logger.Errorf("error creating subnet %v", result.Results[0].Error)
				return errors.Errorf("error creating subnet %v", result.Results[0].Error)
			}
		}
	}
	return nil
}
