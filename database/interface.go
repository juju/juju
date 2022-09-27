// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

// Logger describes methods for emitting log output.
type Logger interface {
	Errorf(string, ...interface{})
}
