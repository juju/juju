// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseconsumer_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination agent_mock_test.go github.com/juju/juju/agent Agent
//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination worker_mock_test.go github.com/juju/worker/v2 Worker
//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination auth_mock_test.go github.com/juju/juju/apiserver/httpcontext Authenticator
//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination raft_mock_test.go github.com/juju/juju/worker/raft/raftleaseconsumer RaftApplier,State,Logger
//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination target_mock_test.go github.com/juju/juju/core/raftlease NotifyTarget
//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination writer_mock_test.go io Writer
//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination prometheus_mock_test.go github.com/prometheus/client_golang/prometheus Registerer
//go:generate go run github.com/golang/mock/mockgen -package raftleaseconsumer -destination state_mock_test.go github.com/juju/juju/worker/state StateTracker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
