// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/caller_mock.go github.com/juju/juju/api Connection
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/logger_mock.go github.com/juju/juju/worker/apicaller Logger

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
