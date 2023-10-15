// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package jujuclient_test -destination proxy_mock_test.go github.com/juju/juju/jujuclient ProxyFactory

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}
