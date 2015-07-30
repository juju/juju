// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"

	"github.com/juju/juju/state"
)

// StatusProviderForUnitFn is a function which returns a structured
// (e.g. json, yaml, etc.) string representation of the status.
type StatusProviderForUnitFn func(*state.Unit) (interface{}, error)

// statusProvidersForUnits contains all registered statusProviders.
var statusProvidersForUnits = map[string]StatusProviderForUnitFn{}

// RegisterStatusProviderForUnit registers status providers with the
// status API server client.
func RegisterStatusProviderForUnits(statusType string, provider StatusProviderForUnitFn) {
	if _, ok := statusProvidersForUnits[statusType]; ok {
		panic(fmt.Sprintf("duplicate registration of status type: %s", statusType))
	}

	statusProvidersForUnits[statusType] = provider
}
