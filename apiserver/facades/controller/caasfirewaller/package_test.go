// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package caasfirewaller_test -destination common_service_mocks_test.go github.com/juju/juju/apiserver/internal/charms CharmService
//go:generate go run go.uber.org/mock/mockgen -typed -package caasfirewaller_test -destination service_mocks_test.go github.com/juju/juju/apiserver/facades/controller/caasfirewaller ApplicationService

func TestAll(t *testing.T) {
	tc.TestingT(t)
}
