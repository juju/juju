// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

// NewCharmRevisionUpdaterAPITest creates a new charmrevisionupdater API
// with a State interface directly, for use in tests.
func NewCharmRevisionUpdaterAPITest(state State) (*CharmRevisionUpdaterAPI, error) {
	return &CharmRevisionUpdaterAPI{state: state}, nil
}
