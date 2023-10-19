// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/clientfacade_mock.go github.com/juju/juju/api/base APICallCloser,ClientFacade
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/caller_mock.go github.com/juju/juju/api/base APICaller,FacadeCaller

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
