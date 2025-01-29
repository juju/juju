// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisioner

import (
	"testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package charmrevisioner -destination package_mocks_test.go github.com/juju/juju/internal/worker/charmrevisioner CharmhubClient,ModelConfigService,ApplicationService,ModelService,ResourceService
//go:generate go run go.uber.org/mock/mockgen -typed -package charmrevisioner -destination clock_mocks_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package charmrevisioner -destination http_mocks_test.go github.com/juju/juju/core/http HTTPClientGetter,HTTPClient

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

func ptr[T any](v T) *T {
	return &v
}
