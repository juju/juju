// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	. "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sshtunneler -destination ./service_mock_test.go github.com/juju/juju/internal/sshtunneler State,ControllerInfo,SSHDial
//go:generate go run go.uber.org/mock/mockgen -typed -package sshtunneler -destination ./clock_mock_test.go github.com/juju/clock Clock

func TestPackage(t *T) {
	gc.TestingT(t)
}
