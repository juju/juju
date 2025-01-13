// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/facades/agent/resourceshookcontext ApplicationService,ResourceService

func Test(t *testing.T) {
	gc.TestingT(t)
}
