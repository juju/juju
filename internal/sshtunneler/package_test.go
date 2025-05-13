// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	. "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sshtunneler -destination ./service_mock_test.go github.com/juju/juju/internal/sshtunneler State,ControllerInfo,SSHDial
//go:generate go run go.uber.org/mock/mockgen -typed -package sshtunneler -destination ./clock_mock_test.go github.com/juju/clock Clock

func TestPackage(t *T) {
	tc.TestingT(t)
}
