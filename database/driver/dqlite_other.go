//go:build !dqlite

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package driver

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
