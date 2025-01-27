// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package resource -destination resource_opener_mock_test.go github.com/juju/juju/core/resource Opener
//go:generate go run go.uber.org/mock/mockgen -typed -package resource -destination service_mock_test.go github.com/juju/juju/apiserver/internal/handlers/resource ResourceServiceGetter,ApplicationServiceGetter,ApplicationService,ResourceService,ResourceOpenerGetter,Validator

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
