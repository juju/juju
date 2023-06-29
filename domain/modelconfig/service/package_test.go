// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/modelconfig/service CloudService,CloudDefaultsState,StaticConfigProvider
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
