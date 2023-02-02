//go:build !linux
// +build !linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package driver

type Error = error

const (
	ErrBusy         = 5
	ErrBusyRecovery = 5 | (1 << 8)
	ErrBusySnapshot = 5 | (2 << 8)
)
