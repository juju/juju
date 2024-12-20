//go:build dqlite && linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package driver

import (
	sqldriver "database/sql/driver"

	"github.com/canonical/go-dqlite/v2/driver"

	"github.com/juju/juju/internal/database/client"
)

// Error is returned in case of database errors.
type Error = driver.Error

const (
	ErrBusy         = driver.ErrBusy
	ErrBusyRecovery = driver.ErrBusyRecovery
	ErrBusySnapshot = driver.ErrBusySnapshot
)

// New creates a new dqlite driver, which also implements the
// driver.Driver interface.
func New(store client.NodeStore, dialer client.DialFunc) (sqldriver.Driver, error) {
	return driver.New(store, driver.WithDialFunc(dialer))
}
