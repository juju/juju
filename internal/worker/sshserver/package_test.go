// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sshserver -destination facade_client_mock_test.go github.com/juju/juju/internal/worker/sshserver FacadeClient,SessionHandler
//go:generate go run go.uber.org/mock/mockgen -package sshserver -destination listener_mock_test.go net Listener
//go:generate go run go.uber.org/mock/mockgen -typed -package sshserver -destination session_mock_test.go github.com/juju/juju/internal/worker/sshserver SSHConnector

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
