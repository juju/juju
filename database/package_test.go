// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"testing"

	"github.com/juju/loggo"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type stubLogger struct{}

func (stubLogger) Errorf(string, ...interface{})            {}
func (stubLogger) Warningf(string, ...interface{})          {}
func (stubLogger) Debugf(string, ...interface{})            {}
func (stubLogger) Logf(loggo.Level, string, ...interface{}) {}
