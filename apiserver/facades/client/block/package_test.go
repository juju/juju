// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package block -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/block BlockCommandService,Authorizer

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}
