// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package common_test -destination network_mock_test.go github.com/juju/juju/core/network ConfigSource,ConfigSourceNIC,ConfigSourceAddr

func Test(t *testing.T) {
	gc.TestingT(t)
}
