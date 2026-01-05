// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import "github.com/juju/juju/domain/application/architecture"

// agentVersion represents the version of the agents running in the controller.
type agentVersion struct {
	Version string `db:"version"`
}

type agentBinaryStore struct {
	Version        string `db:"version"`
	ArchitectureID int    `db:"architecture_id"`
}

type architectures []architecture.Architecture
