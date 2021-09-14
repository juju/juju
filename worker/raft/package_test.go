// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package raft -destination raft_mock_test.go github.com/juju/juju/worker/raft Raft
//go:generate go run github.com/golang/mock/mockgen -package raft -destination raftlease_mock_test.go github.com/juju/juju/core/raftlease NotifyTarget,FSMResponse
//go:generate go run github.com/golang/mock/mockgen -package raft -destination raft_future_mock_test.go github.com/hashicorp/raft ApplyFuture,ConfigurationFuture

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
