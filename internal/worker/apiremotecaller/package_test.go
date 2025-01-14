// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package apiremotecaller -destination package_mocks_test.go github.com/juju/juju/internal/worker/apiremotecaller RemoteServer
//go:generate go run go.uber.org/mock/mockgen -typed -package apiremotecaller -destination clock_mocks_test.go github.com/juju/clock Clock

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}
