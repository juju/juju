//go:build !dqlite

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package driver

import (
	"database/sql/driver"

	"github.com/juju/juju/internal/database/client"
)

type Error struct {
	Code    int
	Message string
}

func (e Error) Error() string {
	return e.Message
}

const (
	ErrBusy         = 5
	ErrBusyRecovery = 5 | (1 << 8)
	ErrBusySnapshot = 5 | (2 << 8)
)

// New creates a new dqlite driver, which also implements the
// driver.Driver interface.
func New(store client.NodeStore, dialer client.DialFunc) (driver.Driver, error) {
	return nil, nil
}
