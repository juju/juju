// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go -source=./service.go
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
