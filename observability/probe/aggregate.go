// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package probe

import (
	"github.com/juju/errors"
)

// Aggregate is an implementation of the Prober interface that returns success
// if all probes under it's controler are successful or false if one more of
// the probes fail.
// Convenience NewAggregate() exists to initialise the map.
type Aggregate struct {
	// Probes is a map of probes to run as part of this aggregate with the key
	// corresponding to well known name for the probe.
	Probes map[string]Prober
}

// ProbeResultCallBack is a function signature for receiving the result of a
// probers probe call.
type ProbeResultCallback func(probeKey string, val bool, err error)

func NewAggregate() *Aggregate {
	return &Aggregate{
		Probes: make(map[string]Prober),
	}
}

// Probe implements Prober Probe
func (a *Aggregate) Probe() (bool, error) {
	return a.ProbeWithResultCallback(ProbeResultCallback(func(_ string, _ bool, _ error) {}))
}

// ProbeWithResultCallback functions the same as Probe but for each probe tested
// in the aggregate calls the provided callback with probe name and result.
// Useful for building more details reports of what probes are failing and
// succeeding.
func (a *Aggregate) ProbeWithResultCallback(
	cb ProbeResultCallback,
) (bool, error) {
	rval := true
	var errVal error

	for name, p := range a.Probes {
		val, err := p.Probe()
		cb(name, val, err)
		if err != nil && errVal == nil {
			errVal = errors.Annotatef(err, "probe %s", name)
		} else if err != nil {
			errVal = errors.Wrap(errVal, errors.Annotatef(err, "probe %s", name))
		}

		// only change rval if it's currently true. All probes in the aggregate
		// need to return true to get a true answer.
		rval = rval && val
	}

	return rval, errVal
}
