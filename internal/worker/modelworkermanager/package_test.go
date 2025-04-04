// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	stdtesting "testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelworkermanager_test -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServices,DomainServicesGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package modelworkermanager_test -destination model_service_mock_test.go github.com/juju/juju/internal/worker/modelworkermanager ModelService
//go:generate go run go.uber.org/mock/mockgen -typed -package modelworkermanager_test -destination lease_mock_test.go github.com/juju/juju/core/lease Manager

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}
