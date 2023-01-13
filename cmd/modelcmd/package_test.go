// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/modelconfig_mock.go github.com/juju/juju/cmd/modelcmd ModelConfigAPI

func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
