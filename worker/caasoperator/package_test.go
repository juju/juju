// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/apibase.go github.com/juju/juju/api/base APICaller
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/uniter_mock.go github.com/juju/juju/worker/uniter/api ProviderIDGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
