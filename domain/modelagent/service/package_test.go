// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/modelagent/service State
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination domain_mock_test.go github.com/juju/juju/domain/modelagent Storage

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
