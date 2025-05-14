// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package apiaddresssetter -destination package_mocks_test.go github.com/juju/juju/internal/worker/apiaddresssetter ControllerConfigService,ApplicationService,ControllerNodeService,NetworkService,DomainServices

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}
