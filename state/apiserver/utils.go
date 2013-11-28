// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"launchpad.net/juju-core/state"
)

// isMachineWithJob returns whether the given entity is a machine that
// is configured to run the given job.
func isMachineWithJob(e state.Authenticator, j state.MachineJob) bool {
	m, ok := e.(*state.Machine)
	if !ok {
		return false
	}
	for _, mj := range m.Jobs() {
		if mj == j {
			return true
		}
	}
	return false
}

// isAgent returns whether the given entity is an agent.
func isAgent(e state.Authenticator) bool {
	_, isUser := e.(*state.User)
	return !isUser
}

func setPassword(e state.Authenticator, password string) error {
	// Catch expected common case of misspelled
	// or missing Password parameter.
	if password == "" {
		return fmt.Errorf("password is empty")
	}
	return e.SetPassword(password)
}
