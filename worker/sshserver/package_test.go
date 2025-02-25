// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_tracker_mock.go github.com/juju/juju/worker/state StateTracker
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/system_state_mock.go github.com/juju/juju/worker/sshserver SystemState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_client_mock.go github.com/juju/juju/worker/sshserver FacadeClient
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
