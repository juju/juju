// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package caasfirewaller_test -destination service_mocks_test.go github.com/juju/juju/apiserver/common/charms CharmService,ApplicationService

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
