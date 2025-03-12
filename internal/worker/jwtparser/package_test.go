// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	. "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package jwtparser -destination service_mock.go github.com/juju/juju/internal/worker/jwtparser ControllerConfigGetter,HTTPClient

func TestPackage(t *T) {
	gc.TestingT(t)
}
