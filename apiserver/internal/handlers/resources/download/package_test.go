// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package download_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package download_test -destination filesystem_mock_test.go github.com/juju/juju/apiserver/internal/handlers/resources/download FileSystem

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
