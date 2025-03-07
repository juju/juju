// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package fanconfigurer -destination package_mock_test.go github.com/juju/juju/internal/worker/fanconfigurer FanConfigurerFacade

func Test(t *testing.T) {
	gc.TestingT(t)
}
