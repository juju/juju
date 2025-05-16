// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package unitassigner -destination services_mock_test.go github.com/juju/juju/apiserver/facades/agent/unitassigner StatusService

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
