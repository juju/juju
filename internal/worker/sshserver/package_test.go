// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	stdtesting "testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sshserver -destination service_mock.go github.com/juju/juju/internal/worker/sshserver ControllerConfigService
//go:generate go run go.uber.org/mock/mockgen -package sshserver_test -destination listener_mock_test.go net Listener

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}
