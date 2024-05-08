// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package query -destination scope_mock_test.go github.com/juju/juju/cmd/juju/waitfor/query FuncScope,Scope

func Test(t *testing.T) {
	gc.TestingT(t)
}
