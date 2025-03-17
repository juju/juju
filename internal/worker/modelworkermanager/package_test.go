// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelworkermanager_test -destination domainservices_mock_test.go github.com/juju/juju/internal/services DomainServices,DomainServicesGetter,ControllerDomainServices

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
