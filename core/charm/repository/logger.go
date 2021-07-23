// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

// Logger describes a logging API.
type Logger interface {
	Errorf(string, ...interface{})
	Tracef(string, ...interface{})
}
