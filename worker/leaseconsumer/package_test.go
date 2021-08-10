// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseconsumer_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package leaseconsumer -destination agent_mock_test.go github.com/juju/juju/agent Agent
//go:generate go run github.com/golang/mock/mockgen -package leaseconsumer -destination worker_mock_test.go github.com/juju/worker/v2 Worker
//go:generate go run github.com/golang/mock/mockgen -package leaseconsumer -destination auth_mock_test.go github.com/juju/juju/apiserver/httpcontext Authenticator

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
