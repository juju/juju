// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package jujuclient_test -destination proxy_mock_test.go github.com/juju/juju/jujuclient ProxyFactory

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
