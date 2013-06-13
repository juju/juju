// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

// srvState serves agent-specific top-level state API methods.
type srvState struct {
	root *srvRoot
}

// Ping just returns no error and is used by the clients to ensure server connection is still alive.
func (r *srvState) Ping() error {
	return nil
}
