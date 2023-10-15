// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package raft -destination raft_mock_test.go github.com/juju/juju/worker/raft Raft,ApplierMetrics
//go:generate go run go.uber.org/mock/mockgen -package raft -destination raftlease_mock_test.go github.com/juju/juju/core/raftlease NotifyTarget,FSMResponse
//go:generate go run go.uber.org/mock/mockgen -package raft -destination raft_future_mock_test.go github.com/hashicorp/raft ApplyFuture,ConfigurationFuture

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
