// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	services "github.com/juju/juju/domain/servicefactory"
	"github.com/juju/juju/testing"
)

// CheckLogger is a loggo.Logger that logs to a *testing.T or *check.C.
type CheckLogger struct {
	testing.CheckLogger
}

// NewCheckLogger returns a CheckLogger that logs to the given CheckLog.
func NewCheckLogger(log testing.CheckLog) CheckLogger {
	return CheckLogger{
		CheckLogger: testing.NewCheckLogger(log),
	}
}

// Child implements services.Logger.
func (c CheckLogger) Child(name string) services.Logger {
	return c
}
