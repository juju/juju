// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package charmdownloader -destination service_mocks_test.go github.com/juju/juju/internal/worker/charmdownloader ApplicationService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}
