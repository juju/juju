// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// CharmInfo stores parameters for a charms.CharmInfo call.
type CharmInfo struct {
	CharmURL string
}

// CharmsList stores parameters for a charms.List call
type CharmsList struct {
	Names []string
}

// CharmsListResult stores result from a charms.List call
type CharmsListResult struct {
	CharmURLs []string
}

// IsMeteredResult stores result from a charms.IsMetered call
type IsMeteredResult struct {
	Metered bool
}
