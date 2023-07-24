//go:build !dqlite || !linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

func NewDQLiteDBModelProvider() ModelProvider {
	panic("not implemented")
}
