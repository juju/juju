// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package service -destination state_mock_test.go github.com/juju/juju/domain/modelmanager/service State
//go:generate go run go.uber.org/mock/mockgen -package service -destination deleter_mock_test.go github.com/juju/juju/core/database DBDeleter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
