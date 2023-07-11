// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package syslogger -destination io_mock_test.go io WriteCloser

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
