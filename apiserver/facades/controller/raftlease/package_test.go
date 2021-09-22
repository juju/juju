// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package raftlease -destination raft_mock_test.go github.com/juju/juju/apiserver/facade RaftContext,Authorizer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
