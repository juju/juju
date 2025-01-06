// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision

import (
	"testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package charmrevision -destination package_mocks_test.go github.com/juju/juju/internal/worker/charmrevision CharmhubClient,ModelConfigService,ApplicationService,ModelService
//go:generate go run go.uber.org/mock/mockgen -typed -package charmrevision -destination clock_mocks_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package charmrevision -destination http_mocks_test.go github.com/juju/juju/core/http HTTPClientGetter,HTTPClient

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}
