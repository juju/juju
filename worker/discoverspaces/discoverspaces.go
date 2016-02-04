// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.discoverspaces")

type discoverspacesWorker struct {
	api               *discoverspaces.API
	tomb              tomb.Tomb
	discoveringSpaces chan struct{}
}

var dashPrefix = regexp.MustCompile("^-*")
var dashSuffix = regexp.MustCompile("-*$")
var multipleDashes = regexp.MustCompile("--+")

func convertSpaceName(name string, existing set.Strings) string {
	// First lower case and replace spaces with dashes.
	name = strings.Replace(name, " ", "-", -1)
	name = strings.ToLower(name)
	// Replace any character that isn't in the set "-", "a-z", "0-9".
	name = network.SpaceInvalidChars.ReplaceAllString(name, "")
	// Get rid of any dashes at the start as that isn't valid.
	name = dashPrefix.ReplaceAllString(name, "")
	// And any at the end.
	name = dashSuffix.ReplaceAllString(name, "")
	// Repleace multiple dashes with a single dash.
	name = multipleDashes.ReplaceAllString(name, "-")
	// Special case of when the space name was only dashes or invalid
	// characters!
	if name == "" {
		name = "empty"
	}
	// If this name is in use add a numerical suffix.
	if existing.Contains(name) {
		counter := 2
		for existing.Contains(name + fmt.Sprintf("-%d", counter)) {
			counter += 1
		}
		name = name + fmt.Sprintf("-%d", counter)
	}
	return name
}

// NewWorker returns a worker
func NewWorker(api *discoverspaces.API) (worker.Worker, chan struct{}) {
	dw := &discoverspacesWorker{
		api:               api,
		discoveringSpaces: make(chan struct{}),
	}
	go func() {
		defer dw.tomb.Done()
		dw.tomb.Kill(dw.loop())
	}()
	return dw, dw.discoveringSpaces
}

func (dw *discoverspacesWorker) Kill() {
	dw.tomb.Kill(nil)
}

func (dw *discoverspacesWorker) Wait() error {
	return dw.tomb.Wait()
}

func (dw *discoverspacesWorker) loop() (err error) {
	ensureClosed := func() {
		select {
		case <-dw.discoveringSpaces:
			// Already closed.
			return
		default:
			close(dw.discoveringSpaces)
		}
	}
	defer ensureClosed()
	modelCfg, err := dw.api.ModelConfig()
	if err != nil {
		return err
	}
	model, err := environs.New(modelCfg)
	if err != nil {
		return err
	}
	networkingModel, ok := environs.SupportsNetworking(model)

	if ok {
		err = dw.handleSubnets(networkingModel)
		if err != nil {
			return errors.Trace(err)
		}
	}
	close(dw.discoveringSpaces)

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
			spaceName = convertSpaceName(spaceName, spaceNames)
			spaceNames.Add(spaceName)
			spaceTag = names.NewSpaceTag(spaceName)
			// We need to create the space.
			args := params.CreateSpacesParams{
				Spaces: []params.CreateSpaceParams{{
					Public:     false,
					SpaceTag:   spaceTag.String(),
					ProviderId: string(space.ProviderId),
				}}}
			result, err := dw.api.CreateSpaces(args)
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
			result, err := dw.api.AddSubnets(args)
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
