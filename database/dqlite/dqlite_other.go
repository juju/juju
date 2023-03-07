//go:build !dqlite

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqlite

type NodeInfo struct {
	Address string
}

func ReconfigureMembership(string, []NodeInfo) error {
	return nil
}
