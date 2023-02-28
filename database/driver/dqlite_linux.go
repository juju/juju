//go:build dqlite && linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package driver

import "github.com/canonical/go-dqlite/driver"

// Error is returned in case of database errors.
type Error = driver.Error

const (
	ErrBusy         = driver.ErrBusy
	ErrBusyRecovery = driver.ErrBusyRecovery
	ErrBusySnapshot = driver.ErrBusySnapshot
)
