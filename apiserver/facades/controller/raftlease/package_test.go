// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package raftlease -destination raft_mock_test.go github.com/juju/juju/apiserver/facade RaftContext,Authorizer,Context

func Test(t *testing.T) {
	gc.TestingT(t)
}
