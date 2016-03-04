// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/gate"
)

// Facade apes a *discoverspaces.API; it's a bit raw but at
// least it's easily mockable.
type Facade interface {
	CreateSpaces(params.CreateSpacesParams) (params.ErrorResults, error)
	AddSubnets(params.AddSubnetsParams) (params.ErrorResults, error)
	ListSpaces() (params.DiscoverSpacesResults, error)
	ListSubnets(params.SubnetsFilters) (params.ListSubnetsResults, error)
}

// NameFunc returns a string derived from base that is not contained in used.
type NameFunc func(base string, used set.Strings) string

// Config defines the operation of a space discovery worker.
type Config struct {

	// Facade exposes the capabilities of a controller.
	Facade Facade

	// Environ exposes the capabilities of a compute substrate.
	Environ environs.Environ

	// NewName is used to sanitise, and make unique, space names as
	// reported by an Environ (for use in juju, via the Facade). You
	// should probably set it to ConvertSpaceName.
	NewName NameFunc

	// Unlocker, if not nil, will be unlocked when the first discovery
	// attempt completes successfully.
	Unlocker gate.Unlocker
}

// Validate returns an error if the config cannot be expected to
// drive a functional worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Environ == nil {
		return errors.NotValidf("nil Environ")
	}
	if config.NewName == nil {
		return errors.NotValidf("nil NewName")
	}
	// missing Unlocker gate just means "don't bother notifying"
	return nil
}

var logger = loggo.GetLogger("juju.discoverspaces")

type discoverspacesWorker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// NewWorker returns a worker that will attempt to discover the
// configured Environ's spaces, and update the controller via the
// configured Facade. Names are sanitised with NewName, and any
// supplied Unlocker will be Unlock()ed when the first complete
// discovery and update succeeds.
//
// Once that update completes, the worker just waits to be Kill()ed.
// We should probably poll for changes, really, but I'm making an
// effort to preserve existing behaviour where possible.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	dw := &discoverspacesWorker{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &dw.catacomb,
		Work: dw.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return dw, nil
}

// Kill is part of the worker.Worker interface.
func (dw *discoverspacesWorker) Kill() {
	dw.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (dw *discoverspacesWorker) Wait() error {
	return dw.catacomb.Wait()
}

func (dw *discoverspacesWorker) loop() (err error) {

	// TODO(mfoord): we'll have a watcher here checking if we need to
	// update the spaces/subnets definition.
	// TODO(fwereade): for now, use a changes channel that apes the
	// standard initial event behaviour, so we can make the loop
	// follow the standard structure.
	changes := make(chan struct{}, 1)
	changes <- struct{}{}

	gate := dw.config.Unlocker
	for {
		select {
		case <-dw.catacomb.Dying():
			return dw.catacomb.ErrDying()
		case <-changes:
			if err := dw.handleSubnets(); err != nil {
				return errors.Trace(err)
			}
			if gate != nil {
				gate.Unlock()
				gate = nil
			}
		}
	}
}

func (dw *discoverspacesWorker) handleSubnets() error {
	environ, ok := environs.SupportsNetworking(dw.config.Environ)
	if !ok {
		// Nothing to do.
		return nil
	}
	if supported, err := environ.SupportsSpaceDiscovery(); err != nil {
		return errors.Trace(err)
	} else if !supported {
		// Nothing to do.
		return nil
	}
	providerSpaces, err := environ.Spaces()
	if err != nil {
		return errors.Trace(err)
	}

	facade := dw.config.Facade
	listSpacesResult, err := facade.ListSpaces()
	if err != nil {
		return errors.Trace(err)
	}
	stateSubnets, err := facade.ListSubnets(params.SubnetsFilters{})
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
		// Check if the space is already in state, in which case we know
		// its name.
		stateSpace, ok := stateSpaceMap[string(space.ProviderId)]
		var spaceTag names.SpaceTag
		if ok {
			spaceName := stateSpace.Name
			if !names.IsValidSpace(spaceName) {
				// Can only happen if an invalid name is stored
				// in state.
				logger.Errorf("space %q has an invalid name, ignoring", spaceName)
				continue

			}
			spaceTag = names.NewSpaceTag(spaceName)

		} else {
			// The space is new, we need to create a valid name for it
			// in state.
			spaceName := string(space.ProviderId)
			// Convert the name into a valid name that isn't already in
			// use.
			spaceName = dw.config.NewName(spaceName, spaceNames)
			spaceNames.Add(spaceName)
			spaceTag = names.NewSpaceTag(spaceName)
			// We need to create the space.
			args := params.CreateSpacesParams{
				Spaces: []params.CreateSpaceParams{{
					Public:     false,
					SpaceTag:   spaceTag.String(),
					ProviderId: string(space.ProviderId),
				}}}
			result, err := facade.CreateSpaces(args)
			if err != nil {
				logger.Errorf("error creating space %v", err)
				return errors.Trace(err)
			}
			if len(result.Results) != 1 {
				return errors.Errorf("unexpected number of results from CreateSpaces, should be 1: %v", result)
			}
			if result.Results[0].Error != nil {
				return errors.Errorf("error from CreateSpaces: %v", result.Results[0].Error)
			}
		}
		// TODO(mfoord): currently no way of removing subnets, or
		// changing the space they're in, so we can only add ones we
		// don't already know about.
		logger.Debugf("Created space %v with %v subnets", spaceTag.String(), len(space.Subnets))
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
			logger.Tracef("Adding subnet %v", subnet.CIDR)
			result, err := facade.AddSubnets(args)
			if err != nil {
				logger.Errorf("invalid creating subnet %v", err)
				return errors.Trace(err)
			}
			if len(result.Results) != 1 {
				return errors.Errorf("unexpected number of results from AddSubnets, should be 1: %v", result)
			}
			if result.Results[0].Error != nil {
				logger.Errorf("error creating subnet %v", result.Results[0].Error)
				return errors.Errorf("error creating subnet %v", result.Results[0].Error)
			}
		}
	}
	return nil
}
