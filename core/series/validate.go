// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// ValidateSeries attempts to validate a series if one is found, otherwise it
// uses the fallback series and validates that one.
// Returns the series it validated against or an error if one is found.
// Note: the selected series will be returned if there is an error to help use
// that for a fallback during error scenarios.
func ValidateSeries(supportedSeries set.Strings, series, fallbackPreferredSeries string) (string, error) {
	// Validate the requested series.
	// Attempt to do the validation in one place, so it makes it easier to
	// reason about where the validation happens. This only happens for IAAS
	// models, as CAAS can't take series as an argument.
	var requestedSeries string
	if series != "" {
		requestedSeries = series
	} else {
		// If no bootstrap series is supplied, go and get that information from
		// the fallback. We should still validate the fallback value to ensure
		// that we also work with that series.
		requestedSeries = fallbackPreferredSeries
	}
	if !supportedSeries.Contains(requestedSeries) {
		return requestedSeries, errors.NotSupportedf("%s", requestedSeries)
	}
	return requestedSeries, nil
}
