// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package charmhub -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/charmhub Backend,ClientFactory,Client

func Test(t *testing.T) {
	gc.TestingT(t)
}
