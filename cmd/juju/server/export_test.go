// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// NewTrustCommand returns a TrustCommand with the api provided as specified.
func NewTrustCommand(api ServerAdminAPI) *TrustCommand {
	return &TrustCommand{
		ServerCommandBase: ServerCommandBase{
			api: api,
		},
	}
}
