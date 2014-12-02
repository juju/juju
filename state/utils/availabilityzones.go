// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	provcommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

// AZResult associates an availability zone name with a tag.
type AZResult struct {
	// Tag is the queried tag.
	Tag names.Tag
	// Zone is the associated available zone name.
	Zone string
	// Err is any error that happened relative to the tag.
	Err error
}

// AvailabilityZones returns the availability zones associated with
// a set of tags.
func AvailabilityZones(st *state.State, tags ...names.Tag) ([]AZResult, error) {
	results := make([]AZResult, len(tags))

	// Get the provider.
	env, err := GetEnvironment(st)
	if err != nil {
		return results, errors.Trace(err)
	}
	zenv, ok := env.(provcommon.ZonedEnviron)
	if !ok {
		return results, errors.NotSupportedf("zones for provider %v", env)
	}

	// Collect the instance IDs. We send a single request to the
	// provider for all the entities at once. So that means gathering a
	// list of instance IDs, one for each entity. If an entity has any
	// sort of trouble then we skip it.
	var instIDs []instance.Id
	for i, tag := range tags {
		results[i].Tag = tag
		instID, err := InstanceID(st, tag)
		if err != nil {
			results[i].Err = errors.Trace(err)
		} else {
			instIDs = append(instIDs, instID)
		}
	}

	// Collect the zones. We expect that we will get back the same
	// number as we passed in and in the same order.
	zones, err := zenv.InstanceAvailabilityZoneNames(instIDs)
	if err != nil {
		return results, errors.Trace(err)
	}
	if len(zones) != len(instIDs) {
		return results, errors.Errorf("received invalid zones: expected %d, got %d", len(instIDs), len(zones))
	}

	// Update the results. The number of zone names we get back should
	// match the number of results without an error. Their order will
	// match as well.
	for i, result := range results {
		if result.Err != nil {
			continue
		}
		// Do another sanity check on the zones we got back. The non-
		// error results should match up exactly with the zones we got
		// back.
		if len(zones) == 0 {
			return results, errors.Errorf("got back too few zones")
		}
		// We do not worry about checking the zone names. About the only
		// one worth checking for would be "", which could have some
		// significance like "zone name not known". However, that case
		// can be addressed by the API caller and does not need to be
		// addressed on the server side.
		results[i].Zone = zones[0]
		zones = zones[1:]
	}
	// Do one last sanity check on matching up the zones we got back
	// with the non-error results.
	if len(zones) > 0 {
		return results, errors.Errorf("got %d extra zones back", len(zones))
	}

	return results, nil
}
