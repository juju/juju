// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8s

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package k8s -destination service_mock_test.go github.com/juju/juju/internal/worker/sshserver/handlers/k8s Resolver
//go:generate go run go.uber.org/mock/mockgen -typed -package k8s -destination executor_mock_test.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
//go:generate go run go.uber.org/mock/mockgen -typed -package k8s -destination session_mock_test.go github.com/gliderlabs/ssh Session,Context

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
