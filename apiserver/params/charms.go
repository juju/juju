// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// CharmInfo stores parameters for a charms.CharmInfo call.
type CharmInfo struct {
	CharmURL string `json:"charm-url"`
}

// CharmsList stores parameters for a charms.List call
type CharmsList struct {
	Names []string `json:"names"`
}

// CharmsListResult stores result from a charms.List call
type CharmsListResult struct {
	CharmURLs []string `json:"charm-urls"`
}

// IsMeteredResult stores result from a charms.IsMetered call
type IsMeteredResult struct {
	Metered bool `json:"metered"`
}
