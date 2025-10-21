// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

// agentVersion represents the version of the agents running in the controller.
type agentVersion struct {
	Version string `db:"version"`
}
