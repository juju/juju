// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package controlsocket -destination services_mock_test.go github.com/juju/juju/internal/worker/controlsocket AccessService

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}
