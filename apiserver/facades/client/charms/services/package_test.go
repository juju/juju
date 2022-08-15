// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/interface_mocks.go github.com/juju/juju/apiserver/facades/client/charms/services StateBackend,ModelBackend,Storage,UploadedCharm

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
