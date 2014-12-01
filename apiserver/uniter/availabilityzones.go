// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	provcommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

var getEnvironment = func(st *state.State) (environs.Environ, error) {
	envcfg, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	env, err := environs.New(envcfg)
	return env, errors.Trace(err)
}

func getInstanceID(st *state.State, tag names.Tag) (instance.Id, error) {
	var instID instance.Id
	switch tag := tag.(type) {
	case names.UnitTag:
		unit, err := st.Unit(tag.Id())
		if err != nil {
			return "", errors.Trace(err)
		}
		instID, err = unit.InstanceId()
		if err != nil {
			return "", errors.Trace(err)
		}
	default:
		return "", errors.Errorf("unsupported tag type: %v", tag)
	}
	return instID, nil
}

type azResult struct {
	tag  names.Tag
	zone string
	err  error
}

func availabilityZone(st *state.State, tag names.Tag) (string, error) {
	results, err := availabilityZones(st, tag)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results))
	}
	result := results[0]
	if result.err != nil {
		return "", errors.Trace(result.err)
	}
	return result.zone, nil
}

func availabilityZones(st *state.State, tags ...names.Tag) ([]azResult, error) {
	results := make([]azResult, len(tags))

	// Get the provider.
	env, err := getEnvironment(st)
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
		results[i].tag = tag
		instID, err := getInstanceID(st, tag)
		if err != nil {
			results[i].err = errors.Trace(err)
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
		if result.err != nil {
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
		results[i].zone = zones[0]
		zones = zones[1:]
	}
	// Do one last sanity check on matching up the zones we got back
	// with the non-error results.
	if len(zones) > 0 {
		return results, errors.Errorf("got %d extra zones back", len(zones))
	}

	return results, nil
}
