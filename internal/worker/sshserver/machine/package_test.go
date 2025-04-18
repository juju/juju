// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package machine -destination service_mock_test.go github.com/juju/juju/internal/worker/sshserver/machine SSHConnector

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
