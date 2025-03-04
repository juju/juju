// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser_test

import (
	. "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/controllerconfig_mock.go github.com/juju/juju/worker/jwtparser ControllerConfig
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/httpclient_mock.go github.com/juju/juju/worker/jwtparser HTTPClient

func TestPackage(t *T) {
	gc.TestingT(t)
}
