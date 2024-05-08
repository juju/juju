// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package controlsocket -destination services_mock_test.go github.com/juju/juju/internal/worker/controlsocket UserService,PermissionService

func Test(t *testing.T) {
	gc.TestingT(t)
}
