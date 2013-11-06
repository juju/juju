// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

// Export meaningful bits for tests only.

var IfaceExpander = ifaceExpander

func NewStore(url string) *CharmStore {
	return &CharmStore{BaseURL: url}
}
