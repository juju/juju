// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/service.go github.com/juju/juju/internal/service Service

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
