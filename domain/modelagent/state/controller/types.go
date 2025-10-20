// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

type ids []int

type agentVersionArchitecture struct {
	Version        string `db:"version"`
	ArchitectureID int    `db:"architecture_id"`
}
