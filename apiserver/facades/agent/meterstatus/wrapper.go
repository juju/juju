// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// MeterStatusWrapper takes a MeterStatus and converts it into an 'api friendly' form where
// Not Set and Not Available (which are important distinctions in state) are converted
// into Amber and Red respectively in the api.
func MeterStatusWrapper(getter func() (state.MeterStatus, error)) (state.MeterStatus, error) {
	status, err := getter()
	if err != nil {
		return state.MeterStatus{}, errors.Trace(err)
	}
	if status.Code == state.MeterNotSet {
		return state.MeterStatus{state.MeterAmber, "not set"}, nil
	}
	if status.Code == state.MeterNotAvailable {

		return state.MeterStatus{state.MeterRed, "not available"}, nil
	}
	return status, nil
}
