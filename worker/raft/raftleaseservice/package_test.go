// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseservice_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination worker_mock_test.go github.com/juju/worker/v2 Worker
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination auth_mock_test.go github.com/juju/juju/apiserver/httpcontext Authenticator
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination raft_mock_test.go github.com/juju/juju/worker/raft/raftleaseservice RaftApplier,State,Logger
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination target_mock_test.go github.com/juju/juju/core/raftlease NotifyTarget,FSMResponse
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination writer_mock_test.go io Writer
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination prometheus_mock_test.go github.com/prometheus/client_golang/prometheus Registerer
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination state_mock_test.go github.com/juju/juju/worker/state StateTracker
//go:generate go run github.com/golang/mock/mockgen -package raftleaseservice -destination raft_future_mock_test.go github.com/hashicorp/raft ApplyFuture

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
