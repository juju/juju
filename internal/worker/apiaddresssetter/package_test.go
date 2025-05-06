// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package apiaddresssetter -destination package_mocks_test.go github.com/juju/juju/internal/worker/apiaddresssetter ControllerConfigService,ApplicationService,ControllerNodeService,NetworkService

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}
