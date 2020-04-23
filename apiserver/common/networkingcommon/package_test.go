// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/common/networkingcommon BackingSpace,BackingSubnet

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
