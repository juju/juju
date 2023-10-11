// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}
