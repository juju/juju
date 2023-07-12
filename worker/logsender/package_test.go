// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/logwriter_mock.go github.com/juju/juju/api/logsender LogWriter

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
