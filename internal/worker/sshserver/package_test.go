// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sshserver -destination service_mock_test.go github.com/juju/juju/internal/worker/sshserver FacadeClient,JWTParser,ProxyHandlers,SSHConnector,TunnelAuthenticator
//go:generate go run go.uber.org/mock/mockgen -package sshserver -destination ssh_ctx_mock_test.go github.com/gliderlabs/ssh Context
//go:generate go run go.uber.org/mock/mockgen -package sshserver -destination listener_mock_test.go net Listener
//go:generate go run go.uber.org/mock/mockgen -typed -package sshserver -destination tunneler_mock_test.go github.com/juju/juju/internal/sshtunneler State,ControllerInfo,SSHDial,Clock
//go:generate go run go.uber.org/mock/mockgen -package sshserver -destination metrics_mock_test.go github.com/prometheus/client_golang/prometheus Registerer

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
