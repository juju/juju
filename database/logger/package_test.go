// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package logger -destination clock_mock_test.go github.com/juju/clock Clock,Timer

func Test(t *testing.T) {
	gc.TestingT(t)
}

type stubLogger struct{}

func (stubLogger) Warningf(string, ...interface{}) {}
